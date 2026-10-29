//! Program output -> mux -> stand-in host terminal, compared after every frame.
//! The twin avoids consuming mux dirty flags.
#[path = "support/fixtures.rs"]
mod fixtures;
#[path = "support/fuzz.rs"]
mod fuzz;
#[path = "support/screen.rs"]
mod screen;
#[path = "support/terminal.rs"]
mod terminal;
use hypercube::{
    mux::{self, Mux, NoChrome, Rect},
    recording::{Reader, Record, Size},
};
use libghostty_vt::Terminal;
use terminal::{host_replies, winsize};

#[derive(Clone, Debug)]
pub enum Input {
    Output(Vec<u8>),
    Size(Size),
}
impl From<Record<'_>> for Input {
    fn from(r: Record<'_>) -> Self {
        match r {
            Record::Output(b) => Self::Output(b.to_vec()),
            Record::Size(s) => Self::Size(s),
        }
    }
}
fn term(s: Size, replies: &[u8]) -> Terminal<'static, 'static> {
    let mut t = Terminal::new(s.cols, s.rows).unwrap();
    mux::use_host_replies_for(&mut t, replies).unwrap();
    t
}
fn frame(m: &mut Mux, twin: &Terminal, host: &mut Terminal) -> Option<String> {
    m.render().unwrap();
    host.vt_write(&m.take_output());
    let want = screen::take(twin).unwrap().with_host_mouse_modes();
    let got = screen::take(host).unwrap();
    let diff = want.diff(&got, 12);
    if diff.is_empty() {
        None
    } else {
        Some(format!(
            "{diff}  program screen:\n{}  host screen:\n{}",
            want.dump(),
            got.dump()
        ))
    }
}
fn escaped(bytes: &[u8]) -> String {
    let mut out = String::from("\"");
    for &b in bytes.iter().take(160) {
        match b {
            0x1b => out.push_str("\\e"),
            b'\r' => out.push_str("\\r"),
            b'\n' => out.push_str("\\n"),
            b'"' | b'\\' => {
                out.push('\\');
                out.push(b as char);
            }
            0x20..=0x7e => out.push(b as char),
            _ => out.push_str(&format!("\\x{b:02x}")),
        }
    }
    out.push('"');
    out
}
fn round_trip(label: &str, initial: Size, records: &[Input]) -> Option<String> {
    let replies = host_replies();
    let size = winsize(initial.cols, initial.rows);
    let mut m = Mux::new(size, Box::new(NoChrome));
    m.connect_host(&replies, size).unwrap();
    let id = m.add_pane("test", false).unwrap();
    m.set_layout(&[(id, Rect::full(size))]).unwrap();
    let mut twin = term(initial, &replies);
    let mut host = term(initial, &replies);
    host.vt_write(b"\x1b[?1049h\x1b[2J");
    for (i, r) in records.iter().enumerate() {
        match r {
            Input::Output(data) => {
                m.pane_mut(id).unwrap().feed(data);
                twin.vt_write(data);
            }
            Input::Size(s) => {
                host.resize(s.cols, s.rows, 0, 0).unwrap();
                twin.resize(s.cols, s.rows, 0, 0).unwrap();
                let size = winsize(s.cols, s.rows);
                m.resize(size);
                m.set_layout(&[(id, Rect::full(size))]).unwrap();
            }
        }
        if m.pane(id).unwrap().held() {
            continue;
        }
        if let Some(diff) = frame(&mut m, &twin, &mut host) {
            let record = match r {
                Input::Output(b) => escaped(b),
                Input::Size(s) => format!("size {}x{}", s.cols, s.rows),
            };
            return Some(format!(
                "{label}: after record {} of {}: {record}\n{diff}",
                i + 1,
                records.len()
            ));
        }
    }
    m.pane(id).unwrap().set_held(false);
    frame(&mut m, &twin, &mut host).map(|diff| format!("{label}: after the final frame\n{diff}"))
}
/// Merge output between resizes, then randomly split it.
fn split(records: &[Input], seed: u64) -> Vec<Input> {
    let mut r = fuzz::Random::new(seed);
    let mut result = Vec::new();
    let mut pending = Vec::new();
    for (i, record) in records.iter().enumerate() {
        match record {
            Input::Output(b) => pending.extend_from_slice(b),
            Input::Size(s) => result.push(Input::Size(*s)),
        }
        let last = i + 1 == records.len() || matches!(records[i + 1], Input::Size(_));
        if matches!(record, Input::Output(_)) && last {
            let maxes = [1, 2, 7, 32, 256];
            let max = maxes[r.less(64, maxes.len() as u64) as usize];
            let mut rest = pending.as_slice();
            while !rest.is_empty() {
                let n = rest.len().min(r.range(64, 1, max) as usize);
                result.push(Input::Output(rest[..n].to_vec()));
                rest = &rest[n..];
            }
            pending.clear();
        }
    }
    result
}
fn round_trip_splits(name: &str, initial: Size, records: &[Input], splits: u64) -> Option<String> {
    if let Some(diff) = round_trip(name, initial, records) {
        return Some(diff);
    }
    for seed in 1..=splits {
        let label = format!("{name} (split seed {seed})");
        if let Some(diff) = round_trip(&label, initial, &split(records, seed)) {
            return Some(diff);
        }
    }
    None
}
fn fixture(name: &str) {
    let f = fixtures::ALL
        .iter()
        .find(|f| f.name == name)
        .unwrap_or_else(|| panic!("no fixture named {name:?}"));
    let records: Vec<_> = f.records.iter().copied().map(Input::from).collect();
    let result = round_trip_splits(
        f.name,
        Size {
            cols: f.cols,
            rows: f.rows,
        },
        &records,
        8,
    );
    if let Some(why) = f.known {
        assert!(
            result.is_some(),
            "{}: matches, but is marked as a known gap ({why})",
            f.name
        );
    } else if let Some(diff) = result {
        panic!("{diff}");
    }
}
fn env_u64(name: &str, default: u64) -> u64 {
    match std::env::var(name) {
        Ok(s) => s
            .parse()
            .unwrap_or_else(|_| panic!("{name} must be an unsigned integer")),
        Err(std::env::VarError::NotPresent) => default,
        Err(e) => panic!("{name}: {e}"),
    }
}
#[test]
fn fuzz() {
    let iterations = env_u64("HYPERCUBE_FUZZ_ITERATIONS", 300);
    let first = env_u64("HYPERCUBE_FUZZ_SEED", 1);
    for i in 0..iterations {
        let seed = first.wrapping_add(i);
        let mut r = fuzz::Random::new(seed);
        let initial = Size {
            cols: r.range(16, 4, 60) as u16,
            rows: r.range(16, 2, 20) as u16,
        };
        let records = fuzz::generate(&mut r);
        if let Some(diff) = round_trip_splits(&format!("fuzz seed {seed}"), initial, &records, 2) {
            panic!("{diff}");
        }
    }
}
fn recording(name: &str) {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("tests/recordings")
        .join(name);
    let data = std::fs::read(&path).unwrap();
    let mut reader = Reader::new(&data).unwrap();
    let initial = match reader.next().unwrap() {
        Some(Record::Size(s)) if s.cols > 0 && s.rows > 0 => s,
        _ => panic!("{name}: doesn't start with a usable size"),
    };
    let mut records = Vec::new();
    while let Some(r) = reader.next().unwrap() {
        records.push(Input::from(r));
    }
    if let Some(diff) = round_trip_splits(name, initial, &records, 2) {
        panic!("{diff}");
    }
}

mod fixtures_tests {
    use super::fixture;

    macro_rules! fixture_tests {
        ($($test:ident => $name:literal),+ $(,)?) => {
            const NAMES: &[&str] = &[$($name),+];
            $(
                #[test]
                fn $test() {
                    fixture($name);
                }
            )+
        };
    }

    fixture_tests! {
        plain_text => "plain text",
        ed_with_palette_bg => "ED with palette bg",
        ed_below_and_above_with_bg => "ED below and above with bg",
        el_with_rgb_bg => "EL with rgb bg",
        ech_with_bg => "ECH with bg",
        erase_with_fg_underline_and_inverse => "erase with fg, underline, and inverse",
        decstbm_with_su_and_sd => "DECSTBM with SU and SD",
        decstbm_with_il_and_dl => "DECSTBM with IL and DL",
        linefeed_at_bottom_margin => "linefeed at bottom margin",
        scrolling_past_the_screen => "scrolling past the screen",
        ich_and_dch => "ICH and DCH",
        insert_mode => "insert mode",
        alt_screen_1049 => "alt screen 1049",
        alt_screen_47_and_1047 => "alt screen 47 and 1047",
        cursor_save_across_alt_screen => "cursor save across alt screen",
        wide_characters => "wide characters",
        wide_character_at_the_right_margin => "wide character at the right margin",
        wide_character_without_wraparound => "wide character without wraparound",
        wide_character_overwritten => "wide character overwritten",
        combining_marks => "combining marks",
        combining_marks_without_grapheme_clustering => "combining marks without grapheme clustering",
        combining_mark_at_the_right_margin => "combining mark at the right margin",
        combining_mark_at_the_right_margin_without_grapheme_clustering => "combining mark at the right margin without grapheme clustering",
        combining_mark_after_moving_back_to_an_earlier_row => "combining mark after moving back to an earlier row",
        emoji_with_variation_selector => "emoji with variation selector",
        emoji_with_variation_selector_without_grapheme_clustering => "emoji with variation selector without grapheme clustering",
        zwj_sequence_with_grapheme_clustering => "ZWJ sequence with grapheme clustering",
        zwj_sequence_without_grapheme_clustering => "ZWJ sequence without grapheme clustering",
        cluster_ending_in_zwj_next_to_a_pictograph => "cluster ending in ZWJ next to a pictograph",
        lone_regional_indicator_next_to_a_flag => "lone regional indicator next to a flag",
        row_printed_in_both_grapheme_clustering_modes => "row printed in both grapheme clustering modes",
        soft_wrapped_lines => "soft-wrapped lines",
        pending_wrap_cursor => "pending wrap cursor",
        reflow_on_resize => "reflow on resize",
        resize_to_the_same_size => "resize to the same size",
        resize_on_the_alt_screen => "resize on the alt screen",
        ansi_colors => "ANSI colors",
        colors_256_and_rgb => "256 and RGB colors",
        attributes => "attributes",
        underline_styles_and_colors => "underline styles and colors",
        styled_blank_cells => "styled blank cells",
        cursor_shapes => "cursor shapes",
        cursor_visibility => "cursor visibility",
        cursor_movement => "cursor movement",
        origin_mode => "origin mode",
        tabs => "tabs",
        repeat_and_decaln => "repeat and DECALN",
        mirrored_input_modes => "mirrored input modes",
        other_input_modes => "other input modes",
        kitty_keyboard_flags => "kitty keyboard flags",
        reverse_video => "reverse video",
        synchronized_output => "synchronized output",
        reset => "reset",
        title_bell_and_hyperlink => "title, bell, and hyperlink",
        osc_4_palette_change => "OSC 4 palette change",
        osc_10_and_11_default_colors => "OSC 10 and 11 default colors",
    }

    #[test]
    fn every_fixture_has_a_test() {
        let mut fixtures: Vec<_> = super::fixtures::ALL.iter().map(|f| f.name).collect();
        let mut tests = NAMES.to_vec();
        fixtures.sort_unstable();
        tests.sort_unstable();
        assert_eq!(tests, fixtures);
    }
}

#[test]
fn recordings() {
    // Discover inputs rather than maintaining a list, so new captures are replayed too.
    let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("tests/recordings");
    let mut names = Vec::new();
    for entry in std::fs::read_dir(dir).unwrap() {
        let entry = entry.unwrap();
        if entry.file_type().unwrap().is_file()
            && entry.path().extension().is_some_and(|ext| ext == "rec")
        {
            names.push(entry.file_name().into_string().unwrap());
        }
    }
    names.sort();
    for name in names {
        recording(&name);
    }
}

// Additional regression identified during port review: a same-size resize
// redraws the stale render-state cache instead of the live grapheme cell.
#[test]
fn combining_mark_survives_same_size_resize() {
    let initial = Size { cols: 20, rows: 8 };
    let records = [
        Input::Output(b"\x1b[?2027le".to_vec()),
        Input::Output(b"\xcc\x81".to_vec()),
        Input::Size(initial),
    ];
    if let Some(diff) = round_trip("combining mark then same-size resize", initial, &records) {
        panic!("{diff}");
    }
}
