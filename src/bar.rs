//! The workspace bar: the bottom row of the screen, which numbers every
//! running workspace in the order its host started. Ctrl-b and a workspace's
//! number switch the terminal to it. Ctrl-b c turns the bar into a prompt
//! for the path of a workspace to switch to, starting it if need be. An amber bell
//! marks a workspace with an agent waiting for a tool approval.

use std::error::Error;
use std::fs;
use std::io::ErrorKind;
use std::path::{Path, PathBuf};
use std::time::SystemTime;

use crate::colors::{self, ATTENTION, Palette};
use crate::host::{self, fit, text_width};
use crate::icons;
use crate::mux::{Winsize, now_ms};
use crate::store;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// How long an error shows in place of the workspaces.
const ERROR_MS: i64 = 5000;
/// What the prompt for a new workspace's path shows before the path.
const PROMPT: &str = " workspace path: ";

/// A running workspace as the bar lists it.
#[derive(Clone, Debug, PartialEq)]
pub struct Entry {
    /// The state file.
    pub path: PathBuf,
    /// The checkout, if the state has an orchestrator.
    pub root: Option<PathBuf>,
    /// The checkout's directory name, or the state's name without one.
    pub name: String,
    /// Whether an agent there waits for a tool approval.
    pub attention: bool,
}

impl Entry {
    /// The bell after the name while an agent waits for an approval.
    fn bell(&self) -> String {
        if self.attention {
            format!(" {}", icons::ATTENTION)
        } else {
            String::new()
        }
    }
}

/// What a key typed at the prompt asks the host to do.
#[derive(Debug, PartialEq)]
pub enum Typed {
    None,
    /// The path changed.
    Changed,
    /// Esc closed the prompt.
    Cancel,
    /// Enter closed the prompt on this path.
    Enter(String),
}

#[derive(Default)]
pub struct Bar {
    pub entries: Vec<Entry>,
    /// The state file of the host's own workspace.
    pub current: PathBuf,
    /// The path typed so far while the bar prompts for a new workspace.
    pub prompt: Option<String>,
    error: String,
    /// When the error was shown, in `now_ms` time.
    error_at: i64,
}

impl Bar {
    pub fn new(current: PathBuf) -> Bar {
        Bar {
            current,
            ..Bar::default()
        }
    }

    /// Show `error` in place of the workspaces for `ERROR_MS`.
    pub fn fail(&mut self, error: String) {
        self.error = error;
        self.error_at = now_ms();
    }

    /// List `entries`, and drop an error shown for `ERROR_MS`. Returns
    /// whether the bar changed.
    pub fn update(&mut self, entries: Vec<Entry>) -> bool {
        let expired = !self.error.is_empty() && now_ms() - self.error_at >= ERROR_MS;
        if expired {
            self.error.clear();
        }
        let changed = self.entries != entries;
        self.entries = entries;
        changed || expired
    }

    /// Take a key typed at the prompt: text goes at the end of the path,
    /// Backspace erases its last character, and Enter and Esc close the
    /// prompt.
    pub fn key(&mut self, key: &[u8]) -> Typed {
        let Some(path) = &mut self.prompt else {
            return Typed::None;
        };
        match key {
            b"\r" | b"\n" => {
                let path = std::mem::take(path);
                self.prompt = None;
                Typed::Enter(path)
            }
            b"\x1b" => {
                self.prompt = None;
                Typed::Cancel
            }
            [0x7f] | [0x08] => {
                path.pop();
                Typed::Changed
            }
            _ => match std::str::from_utf8(key) {
                Ok(text) if !text.chars().any(char::is_control) => {
                    path.push_str(text);
                    Typed::Changed
                }
                _ => Typed::None,
            },
        }
    }

    /// The width of each tab's text, `0 name`, followed by the bell while
    /// an agent there waits.
    fn widths(&self) -> impl Iterator<Item = usize> {
        (self.entries.iter().enumerate())
            .map(|(i, e)| text_width(&format!("{i} {}{}", e.name, e.bell())))
    }

    /// How many tabs fit, whole, in `cols`.
    fn fit(&self, cols: u16) -> usize {
        let mut used = 1; // One leading space; three between tabs.
        self.widths()
            .enumerate()
            .take_while(|(i, w)| {
                used += w + if *i == 0 { 0 } else { 3 };
                used <= usize::from(cols)
            })
            .count()
    }

    /// The number of the workspace whose tab is at column `col` of a bar
    /// `cols` wide, while the bar lists the workspaces. The leading space and
    /// separators do not belong to a tab.
    pub fn tab_at(&self, col: u16, cols: u16) -> Option<usize> {
        if self.prompt.is_some() || !self.error.is_empty() {
            return None;
        }
        let mut start = 1;
        for (i, width) in self.widths().take(self.fit(cols)).enumerate() {
            if (start..start + width).contains(&usize::from(col)) {
                return Some(i);
            }
            start += width + 3;
        }
        None
    }

    /// Draw the bar across the last row in the bar colors: the prompt while
    /// it is open, or numbered workspace tabs joined by dim ` · ` separators.
    /// The host's own tab is bold, other tabs are dimmed, and an amber bell
    /// follows the name of each workspace with an agent waiting for an approval.
    pub fn draw(&self, s: &mut String, size: Winsize, palette: Palette) {
        let (cols, row) = (size.ws_col, size.ws_row);
        if row == 0 {
            return;
        }
        s.push_str(&format!(
            "\x1b[{row};1H{}{}\x1b[{row};1H",
            palette.bar.sgr(),
            " ".repeat(usize::from(cols))
        ));
        if let Some(path) = &self.prompt {
            // The end of a path too long to show whole, and a cursor after it.
            let mut path = path.as_str();
            let width = |path: &str| text_width(PROMPT) + text_width(path) + 1;
            while !path.is_empty() && width(path) > usize::from(cols) {
                let mut chars = path.chars();
                chars.next();
                path = chars.as_str();
            }
            s.push_str(&fit(&format!("{PROMPT}{path}"), cols));
            if width(path) <= usize::from(cols) {
                s.push_str("\x1b[7m \x1b[27m");
            }
            return;
        }
        if !self.error.is_empty() {
            s.push_str(&fit(&format!(" {}", self.error), cols));
            return;
        }
        s.push_str(&fit(" ", cols));
        for (i, e) in self.entries.iter().take(self.fit(cols)).enumerate() {
            if i > 0 {
                s.push_str(
                    &colors::Colors {
                        fg: colors::mix(palette.bar.fg, palette.bar.bg, 0.55),
                        bg: palette.bar.bg,
                    }
                    .sgr(),
                );
                s.push_str(" · ");
            }
            let current = e.path == self.current;
            let text = if current {
                palette.bar.fg
            } else {
                colors::mix(palette.bar.fg, palette.bar.bg, 0.4)
            };
            // Foreground-only changes keep the bar's background throughout.
            // Reset attributes explicitly so separators and other tabs aren't bold.
            let (r, g, b) = text;
            s.push_str(&format!("\x1b[22;38;2;{r};{g};{b}m"));
            if current {
                s.push_str("\x1b[1m");
            }
            s.push_str(&format!("{i} {}", e.name));
            if e.attention {
                let (r, g, b) = ATTENTION;
                s.push_str(&format!("\x1b[38;2;{r};{g};{b}m{}", e.bell()));
            }
        }
        s.push_str(&palette.bar.sgr());
    }
}

/// Every running workspace, in the order its host started. A host writes its
/// process ID into its host lock when it starts, so the lock file's
/// modification time is when it started.
pub fn running() -> Result<Vec<Entry>> {
    let dir = store::dir();
    let files = match fs::read_dir(&dir) {
        Ok(files) => files.collect::<std::io::Result<Vec<_>>>()?,
        Err(e) if e.kind() == ErrorKind::NotFound => Vec::new(),
        Err(e) => return Err(format!("read {}: {e}", dir.display()).into()),
    };
    let mut listed: Vec<(SystemTime, Entry)> = Vec::new();
    for file in files {
        let file_name = file.file_name();
        let Some(workspace) = file_name
            .to_str()
            .and_then(|n| n.strip_suffix(".host.lock"))
        else {
            continue;
        };
        let path = dir.join(format!("{workspace}.json"));
        if !host::running(&path)? {
            continue;
        }
        // A workspace that stops or is deleted while it is read is left out.
        let Ok(started) = file.metadata().and_then(|m| m.modified()) else {
            continue;
        };
        let state = store::load(&path).ok();
        let root = state.as_ref().and_then(|s| s.root()).map(Path::to_path_buf);
        let attention = state
            .as_ref()
            .is_some_and(|state| state.agents.iter().any(|a| a.attention));
        let name = root
            .as_ref()
            .and_then(|r| r.file_name())
            .map_or_else(|| workspace.to_string(), |n| n.to_string_lossy().into());
        listed.push((
            started,
            Entry {
                path,
                root,
                name,
                attention,
            },
        ));
    }
    listed.sort_by(|(a, x), (b, y)| a.cmp(b).then_with(|| x.path.cmp(&y.path)));
    Ok(listed.into_iter().map(|(_, entry)| entry).collect())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::colors::Rgb;
    use libghostty_vt::Terminal;
    use libghostty_vt::style::StyleColor;
    use libghostty_vt::terminal::{Point, PointCoordinate};

    fn entry(name: &str) -> Entry {
        Entry {
            path: PathBuf::from(format!("/state/{name}.json")),
            root: Some(PathBuf::from(format!("/src/{name}"))),
            name: name.into(),
            attention: false,
        }
    }

    fn bar(names: &[&str], current: &str) -> Bar {
        let mut bar = Bar::new(PathBuf::from(format!("/state/{current}.json")));
        bar.entries = names.iter().map(|n| entry(n)).collect();
        bar
    }

    fn screen(bar: &Bar, cols: u16) -> Terminal<'static, 'static> {
        let mut s = String::new();
        let size = Winsize {
            ws_col: cols,
            ws_row: 2,
            ..Winsize::default()
        };
        bar.draw(&mut s, size, Palette::default());
        let mut t = Terminal::new(cols, 2).unwrap();
        t.vt_write(s.as_bytes());
        t
    }

    fn row(t: &Terminal) -> String {
        let cols = t.cols().unwrap();
        (0..cols)
            .map(|x| {
                let point = Point::Active(PointCoordinate { x, y: 1 });
                let mut buf = ['\0'; 4];
                match t.grid_ref(point).unwrap().graphemes(&mut buf).unwrap() {
                    0 => ' ',
                    _ => buf[0],
                }
            })
            .collect()
    }

    fn is_cursor(t: &Terminal, x: u16) -> bool {
        let point = Point::Active(PointCoordinate { x, y: 1 });
        t.grid_ref(point).unwrap().style().unwrap().inverse
    }

    fn bold(t: &Terminal, x: u16) -> bool {
        let point = Point::Active(PointCoordinate { x, y: 1 });
        t.grid_ref(point).unwrap().style().unwrap().bold
    }

    fn bg(t: &Terminal, x: u16) -> Rgb {
        let point = Point::Active(PointCoordinate { x, y: 1 });
        match t.grid_ref(point).unwrap().style().unwrap().bg_color {
            StyleColor::Rgb(c) => (c.r, c.g, c.b),
            other => panic!("background {other:?}"),
        }
    }

    #[test]
    fn the_bar_numbers_the_workspaces_and_marks_the_current_one() {
        let t = screen(&bar(&["foo", "bar", "glarpy"], "bar"), 30);
        assert_eq!(row(&t), " 0 foo · 1 bar · 2 glarpy     ");
        assert!((0..9).all(|x| !bold(&t, x)));
        assert!((9..14).all(|x| bold(&t, x)));
        assert!((14..30).all(|x| !bold(&t, x)));
    }

    #[test]
    fn tabs_share_the_bar_colors_with_dim_inactive_text_and_separators() {
        let t = screen(&bar(&["foo", "glarpy"], "glarpy"), 24);
        let palette = Palette::default();
        let point = |x| Point::Active(PointCoordinate { x, y: 1 });
        let fg = |x| match t.grid_ref(point(x)).unwrap().style().unwrap().fg_color {
            StyleColor::Rgb(c) => (c.r, c.g, c.b),
            other => panic!("foreground {other:?}"),
        };
        assert!((1..6).all(|x| fg(x) == colors::mix(palette.bar.fg, palette.bar.bg, 0.4)));
        assert!((6..9).all(|x| fg(x) == colors::mix(palette.bar.fg, palette.bar.bg, 0.55)));
        assert!((9..17).all(|x| fg(x) == palette.bar.fg));
        assert!((0..24).all(|x| bg(&t, x) == palette.bar.bg));
    }

    #[test]
    fn a_bell_marks_a_workspace_waiting_for_an_approval() {
        let mut b = bar(&["foo", "bar"], "bar");
        b.entries[0].attention = true;
        b.entries[1].attention = true;
        let t = screen(&b, 20);
        assert_eq!(row(&t), " 0 foo \u{f0f3} · 1 bar \u{f0f3}  ");
        let point = |x| Point::Active(PointCoordinate { x, y: 1 });
        let fg = |x| match t.grid_ref(point(x)).unwrap().style().unwrap().fg_color {
            StyleColor::Rgb(c) => (c.r, c.g, c.b),
            other => panic!("foreground {other:?}"),
        };
        assert_eq!([fg(7), fg(17)], [ATTENTION, ATTENTION]);
        assert_eq!(
            fg(5),
            colors::mix(Palette::default().bar.fg, Palette::default().bar.bg, 0.4)
        );
        assert!((0..20).all(|x| bg(&t, x) == Palette::default().bar.bg));
        assert!(!bold(&t, 7));
        assert!(bold(&t, 17));
        // The dot widens the tab; separator clicks do nothing.
        assert_eq!(b.tab_at(7, 20), Some(0));
        assert_eq!(b.tab_at(8, 20), None);
        assert_eq!(b.tab_at(10, 20), None);
        assert_eq!(b.tab_at(11, 20), Some(1));
        assert_eq!(row(&screen(&b, 17)), " 0 foo \u{f0f3}         ");
    }

    #[test]
    fn a_click_finds_the_tab_under_it() {
        let mut b = bar(&["foo", "bar", "glarpy"], "foo");
        let at = |b: &Bar, cols: &[u16]| cols.iter().map(|&c| b.tab_at(c, 23)).collect::<Vec<_>>();
        assert_eq!(
            at(&b, &[0, 1, 5, 6, 8, 9, 13, 14, 22]),
            [
                None,
                Some(0),
                Some(0),
                None,
                None,
                Some(1),
                Some(1),
                None,
                None
            ]
        );
        assert_eq!(b.tab_at(24, 25), Some(2));
        b.prompt = Some(String::new());
        assert_eq!(at(&b, &[0]), [None]);
        b.prompt = None;
        b.fail("no".into());
        assert_eq!(at(&b, &[0]), [None]);
    }

    #[test]
    fn only_the_workspaces_that_fit_are_shown() {
        let b = bar(&["foo", "bar", "glarpy"], "foo");
        assert_eq!(row(&screen(&b, 25)), " 0 foo · 1 bar · 2 glarpy");
        assert_eq!(row(&screen(&b, 24)), " 0 foo · 1 bar          ");
        assert_eq!(row(&screen(&b, 6)), " 0 foo");
        assert_eq!(row(&screen(&b, 5)), "     ");
    }

    #[test]
    fn the_prompt_takes_a_path_and_shows_its_end() {
        let mut b = bar(&["foo"], "foo");
        assert_eq!(b.key(b"x"), Typed::None);
        b.prompt = Some(String::new());
        for key in [b"~".as_slice(), b"/", "é".as_bytes(), b"x", b"\x7f", b"y"] {
            assert_eq!(b.key(key), Typed::Changed);
        }
        assert_eq!(b.key(b"\x1b[A"), Typed::None);
        let t = screen(&b, 24);
        assert_eq!(row(&t), " workspace path: ~/éy   ");
        assert!(is_cursor(&t, 21) && !is_cursor(&t, 20) && !is_cursor(&t, 22));
        assert_eq!(row(&screen(&b, 20)), " workspace path: éy ");
        assert_eq!(b.key(b"\r"), Typed::Enter("~/éy".into()));
        assert_eq!(b.prompt, None);
        assert_eq!(row(&screen(&b, 20)), " 0 foo              ");

        b.prompt = Some("x".into());
        assert_eq!(b.key(b"\x1b"), Typed::Cancel);
        assert_eq!(b.prompt, None);
    }

    #[test]
    fn an_error_shows_in_place_of_the_workspaces_until_it_expires() {
        let mut b = bar(&["foo"], "foo");
        b.fail("no such workspace".into());
        assert_eq!(row(&screen(&b, 20)), " no such workspace  ");
        assert!(!b.update(b.entries.clone()));
        b.error_at -= ERROR_MS;
        assert!(b.update(b.entries.clone()));
        assert_eq!(row(&screen(&b, 20)), " 0 foo              ");
        assert!(b.update(vec![entry("foo"), entry("bar")]));
    }
}
