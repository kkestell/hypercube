//! The workspace bar: the bottom row of the screen, which numbers every
//! running workspace in the order its host started. Ctrl-b and a workspace's
//! number switch the terminal to it. Ctrl-b c turns the bar into a prompt
//! for the path of a workspace to switch to, starting it if need be.

use std::error::Error;
use std::fs;
use std::io::ErrorKind;
use std::path::PathBuf;
use std::time::SystemTime;

use crate::colors::Palette;
use crate::host::{self, fit, text_width};
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
    /// The workspace's accent color, numbered in `colors::ACCENTS`.
    pub accent: usize,
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

    /// The width of each tab, ` 0 name `.
    fn widths(&self) -> impl Iterator<Item = usize> {
        (self.entries.iter().enumerate()).map(|(i, e)| text_width(&format!(" {i} {} ", e.name)))
    }

    /// How many tabs fit, whole, in `cols`.
    fn fit(&self, cols: u16) -> usize {
        let mut used = 0;
        self.widths()
            .take_while(|w| {
                used += w;
                used <= usize::from(cols)
            })
            .count()
    }

    /// The number of the workspace whose tab is at column `col` of a bar
    /// `cols` wide, while the bar lists the workspaces.
    pub fn tab_at(&self, col: u16, cols: u16) -> Option<usize> {
        if self.prompt.is_some() || !self.error.is_empty() {
            return None;
        }
        let mut end = 0;
        self.widths().take(self.fit(cols)).position(|w| {
            end += w;
            usize::from(col) < end
        })
    }

    /// Draw the bar across the last row in the bar colors: the prompt while
    /// it is open, or each workspace that fits after its number, on its own
    /// accent's tab colors with the number dimmer, with the host's own
    /// bold.
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
        for (i, e) in self.entries.iter().take(self.fit(cols)).enumerate() {
            let current = e.path == self.current;
            // The host's own palette has its accent before the list does.
            let tab = if current {
                palette
            } else {
                Palette::of(e.accent)
            };
            let bold = if current { "\x1b[1m" } else { "" };
            let (number, text) = (tab.number, tab.tab.fg);
            s.push_str(&format!(
                "{}{bold} \x1b[38;2;{};{};{}m{i}\x1b[38;2;{};{};{}m {} ",
                tab.tab.sgr(),
                number.0,
                number.1,
                number.2,
                text.0,
                text.1,
                text.2,
                e.name,
            ));
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
        let root = state.as_ref().and_then(|state| {
            let o = state
                .agents
                .iter()
                .find(|a| a.role == store::ORCHESTRATOR)?;
            Some(PathBuf::from(&o.dir))
        });
        let accent = state.map_or(0, |state| state.accent as usize);
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
                accent,
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
            accent: name.len(),
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
        assert_eq!(row(&t), " 0 foo  1 bar  2 glarpy       ");
        assert!(!bold(&t, 1) && !bold(&t, 6));
        assert!((7..14).all(|x| bold(&t, x)));
        assert!(!bold(&t, 14) && !bold(&t, 15));
    }

    #[test]
    fn each_workspace_shows_on_its_own_accent() {
        let t = screen(&bar(&["foo", "glarpy"], "glarpy"), 24);
        let (bar, current) = (Palette::default().bar.bg, Palette::default().tab.bg);
        let point = |x| Point::Active(PointCoordinate { x, y: 1 });
        let fg = |x| match t.grid_ref(point(x)).unwrap().style().unwrap().fg_color {
            StyleColor::Rgb(c) => (c.r, c.g, c.b),
            other => panic!("foreground {other:?}"),
        };
        assert_eq!(
            [fg(1), fg(3)],
            [Palette::of(3).number, Palette::of(3).tab.fg]
        );
        assert_eq!(
            [fg(8), fg(10)],
            [Palette::default().number, Palette::default().tab.fg]
        );
        assert!((0..7).all(|x| bg(&t, x) == Palette::of(3).tab.bg));
        assert!((7..17).all(|x| bg(&t, x) == current));
        assert!((17..24).all(|x| bg(&t, x) == bar));
    }

    #[test]
    fn a_click_finds_the_tab_under_it() {
        let mut b = bar(&["foo", "bar", "glarpy"], "foo");
        let at = |b: &Bar, cols: &[u16]| cols.iter().map(|&c| b.tab_at(c, 23)).collect::<Vec<_>>();
        assert_eq!(
            at(&b, &[0, 6, 7, 13, 14, 22]),
            [Some(0), Some(0), Some(1), Some(1), None, None]
        );
        assert_eq!(b.tab_at(23, 24), Some(2));
        b.prompt = Some(String::new());
        assert_eq!(at(&b, &[0]), [None]);
        b.prompt = None;
        b.fail("no".into());
        assert_eq!(at(&b, &[0]), [None]);
    }

    #[test]
    fn only_the_workspaces_that_fit_are_shown() {
        let b = bar(&["foo", "bar", "glarpy"], "foo");
        assert_eq!(row(&screen(&b, 24)), " 0 foo  1 bar  2 glarpy ");
        assert_eq!(row(&screen(&b, 23)), " 0 foo  1 bar          ");
        assert_eq!(row(&screen(&b, 6)), "      ");
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
