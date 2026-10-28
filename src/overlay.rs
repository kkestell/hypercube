//! The workspace overlay: a list of every workspace with a state file,
//! drawn over the whole screen, from which the user switches the terminal to
//! another workspace, stops one, or detaches.

use std::error::Error;
use std::path::{Path, PathBuf};

use crate::host::{self, fit};
use crate::mux::Winsize;
use crate::store;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// Black on light blue, for the title row.
const TITLE: &str = "\x1b[0;38;2;0;0;0;48;2;135;215;255m";
const SELECTED: &str = "\x1b[0;7m";
const ERROR: &str = "\x1b[0;31m";
const HELP: &str = "\x1b[0;2m";

/// A workspace as the overlay lists it.
#[derive(Clone, Debug, PartialEq)]
pub struct Row {
    /// The state file.
    pub path: PathBuf,
    /// The checkout, if the state has an orchestrator.
    pub root: Option<PathBuf>,
    pub name: String,
    pub running: bool,
    pub agents: usize,
    pub busy: usize,
    /// How long ago it was last active.
    pub active: String,
}

/// What a key in the overlay asks the host to do.
#[derive(Debug, PartialEq)]
pub enum Action {
    None,
    /// The selection moved.
    Move,
    /// Switch the client to the workspace whose checkout this is.
    Switch(PathBuf),
    /// Stop the workspace whose state file this is.
    Stop(PathBuf),
    Detach,
    Close,
}

pub struct Overlay {
    rows: Vec<Row>,
    selected: usize,
    /// The state file of the host's own workspace.
    current: PathBuf,
    /// Shown in the last row, until the next key.
    pub error: String,
}

impl Overlay {
    /// The overlay of the host whose state file is `current`, listing every
    /// workspace with a state file, most recently active first, with
    /// `current` selected.
    pub fn load(current: &Path) -> Result<Overlay> {
        let mut overlay = Overlay {
            rows: Vec::new(),
            selected: 0,
            current: current.to_path_buf(),
            error: String::new(),
        };
        overlay.reload()?;
        overlay.selected = overlay
            .rows
            .iter()
            .position(|r| r.path == current)
            .unwrap_or(0);
        Ok(overlay)
    }

    /// Read the list again, keeping the selected workspace selected.
    pub fn reload(&mut self) -> Result<()> {
        let mut listed = Vec::new();
        for w in store::workspaces()? {
            let active = w.active()?;
            let row = Row {
                root: w.root().map(Path::to_path_buf),
                name: w.name(),
                running: host::running(&w.path)?,
                agents: w.state.agents.len(),
                busy: w
                    .state
                    .agents
                    .iter()
                    .filter(|a| a.activity == store::BUSY)
                    .count(),
                active: store::ago(active),
                path: w.path,
            };
            listed.push((active, row));
        }
        listed.sort_by_key(|&(active, _)| std::cmp::Reverse(active));
        self.set_rows(listed.into_iter().map(|(_, row)| row).collect());
        Ok(())
    }

    fn set_rows(&mut self, rows: Vec<Row>) {
        let selected = self.rows.get(self.selected).map(|r| r.path.clone());
        self.rows = rows;
        self.selected = selected
            .and_then(|path| self.rows.iter().position(|r| r.path == path))
            .unwrap_or(0);
    }

    /// What a key of captured input asks for: Up and Down or k and j move,
    /// Enter switches, x stops, d detaches, and Esc closes.
    pub fn key(&mut self, key: &[u8]) -> Action {
        self.error.clear();
        match key {
            b"\x1b[A" | b"\x1bOA" | b"k" => {
                self.selected = self.selected.saturating_sub(1);
                Action::Move
            }
            b"\x1b[B" | b"\x1bOB" | b"j" => {
                self.selected = (self.selected + 1).min(self.rows.len().saturating_sub(1));
                Action::Move
            }
            b"\r" | b"\n" => match self.rows.get(self.selected) {
                Some(r) if r.path == self.current => Action::Close,
                Some(r) => match &r.root {
                    Some(root) => Action::Switch(root.clone()),
                    None => {
                        self.error = format!("{}: the state has no orchestrator", r.name);
                        Action::Move
                    }
                },
                None => Action::None,
            },
            b"x" => match self.rows.get(self.selected) {
                Some(r) if r.running => Action::Stop(r.path.clone()),
                Some(r) => {
                    self.error = format!("{} is not running", r.name);
                    Action::Move
                }
                None => Action::None,
            },
            b"d" => Action::Detach,
            b"\x1b" => Action::Close,
            _ => Action::None,
        }
    }

    /// Draw the overlay over the whole screen: a title row, a row for each
    /// workspace, and the error or the keys in the last row.
    pub fn draw(&self, out: &mut Vec<u8>, size: Winsize) {
        let (cols, rows) = (size.ws_col, usize::from(size.ws_row));
        if rows == 0 {
            return;
        }
        let mut s = String::new();
        let line = |s: &mut String, row: usize, style: &str, text: &str| {
            let text = fit(text, cols);
            s.push_str(&format!(
                "\x1b[{};1H\x1b[0m\x1b[2K{style}{text}\x1b[0m",
                row + 1
            ));
        };
        line(&mut s, 0, TITLE, &pad(" Workspaces", cols));
        let width = self.rows.iter().map(|r| host::text_width(&r.name)).max();
        let width = width.unwrap_or(0);
        let list = rows.saturating_sub(2);
        // Scroll the list to keep the selection in view.
        let first = self.selected.saturating_sub(list.saturating_sub(1));
        for (i, r) in self.rows.iter().enumerate().skip(first).take(list) {
            let mark = if r.path == self.current { '*' } else { ' ' };
            let state = if r.running { "running" } else { "stopped" };
            let agents = if r.agents == 1 { "agent" } else { "agents" };
            let gap = " ".repeat(width - host::text_width(&r.name));
            let text = format!(
                " {mark} {}{gap}  {state}  {} {agents}, {} busy  {}",
                r.name, r.agents, r.busy, r.active
            );
            let (style, text) = if i == self.selected {
                (SELECTED, pad(&text, cols))
            } else {
                ("", text)
            };
            line(&mut s, 1 + i - first, style, &text);
        }
        s.push_str(&format!("\x1b[{};1H\x1b[0J", 2 + self.rows.len().min(list)));
        if rows > 1 {
            let (style, text) = if self.error.is_empty() {
                (HELP, " ↑↓ move  enter switch  x stop  d detach  esc close")
            } else {
                (ERROR, self.error.as_str())
            };
            let text = if style == ERROR {
                format!(" {text}")
            } else {
                text.to_string()
            };
            line(&mut s, rows - 1, style, &text);
        }
        out.extend_from_slice(s.as_bytes());
    }
}

/// `text` with spaces after it to fill `cols` columns, for a row with a
/// background.
fn pad(text: &str, cols: u16) -> String {
    let fitted = fit(text, cols);
    let used = host::text_width(&fitted);
    format!(
        "{fitted}{}",
        " ".repeat(usize::from(cols).saturating_sub(used))
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use libghostty_vt::Terminal;
    use libghostty_vt::terminal::{Point, PointCoordinate};

    fn row(name: &str, running: bool) -> Row {
        Row {
            path: PathBuf::from(format!("/state/{name}.json")),
            root: Some(PathBuf::from(format!("/src/{name}"))),
            name: format!("~/src/{name}"),
            running,
            agents: 3,
            busy: 1,
            active: "2m ago".into(),
        }
    }

    fn overlay(rows: Vec<Row>, current: &str) -> Overlay {
        let current = PathBuf::from(format!("/state/{current}.json"));
        let selected = rows.iter().position(|r| r.path == current).unwrap_or(0);
        Overlay {
            rows,
            selected,
            current,
            error: String::new(),
        }
    }

    fn screen(o: &Overlay, cols: u16, rows: u16) -> (Terminal<'static, 'static>, Vec<String>) {
        let mut out = Vec::new();
        o.draw(
            &mut out,
            Winsize {
                ws_col: cols,
                ws_row: rows,
                ws_xpixel: 0,
                ws_ypixel: 0,
            },
        );
        let mut t = Terminal::new(cols, rows).unwrap();
        // Something underneath that the overlay must cover.
        t.vt_write("x".repeat(usize::from(cols) * usize::from(rows)).as_bytes());
        t.vt_write(&out);
        let lines = (0..rows)
            .map(|y| {
                (0..cols)
                    .map(|x| {
                        let point = Point::Active(PointCoordinate { x, y: u32::from(y) });
                        let mut buf = ['\0'; 4];
                        match t.grid_ref(point).unwrap().graphemes(&mut buf).unwrap() {
                            0 => " ".to_string(),
                            n => buf[..n].iter().collect(),
                        }
                    })
                    .collect::<String>()
                    .trim_end()
                    .to_string()
            })
            .collect();
        (t, lines)
    }

    #[test]
    fn the_overlay_lists_workspaces_and_marks_the_current_one() {
        let o = overlay(vec![row("foo", true), row("bar", false)], "bar");
        let (t, lines) = screen(&o, 60, 6);
        assert_eq!(
            lines,
            [
                " Workspaces",
                "   ~/src/foo  running  3 agents, 1 busy  2m ago",
                " * ~/src/bar  stopped  3 agents, 1 busy  2m ago",
                "",
                "",
                " ↑↓ move  enter switch  x stop  d detach  esc close",
            ]
        );
        let inverse = |x, y| {
            let point = Point::Active(PointCoordinate { x, y });
            t.grid_ref(point).unwrap().style().unwrap().inverse
        };
        assert!(inverse(1, 2) && inverse(59, 2));
        assert!(!inverse(1, 1));
    }

    #[test]
    fn names_are_clipped_to_whole_grapheme_clusters() {
        let mut wide = row("foo", true);
        wide.name = "界界界e\u{301}xyz".into();
        let o = overlay(vec![wide], "foo");
        let (_, lines) = screen(&o, 11, 3);
        // Spacer cells read as blanks.
        assert_eq!(lines[1], " * 界 界 界 e\u{301}x");
        let (_, lines) = screen(&o, 8, 3);
        assert_eq!(lines[1], " * 界 界");
    }

    #[test]
    fn keys_move_the_selection_and_stop_at_the_ends() {
        let mut o = overlay(vec![row("a", true), row("b", true), row("c", true)], "a");
        assert_eq!(o.key(b"k"), Action::Move);
        assert_eq!(o.selected, 0);
        for (key, selected) in [
            (&b"j"[..], 1),
            (b"\x1b[B", 2),
            (b"\x1bOB", 2),
            (b"\x1b[A", 1),
            (b"\x1bOA", 0),
        ] {
            assert_eq!(o.key(key), Action::Move);
            assert_eq!(o.selected, selected, "{key:?}");
        }
    }

    #[test]
    fn keys_switch_stop_detach_and_close() {
        let mut o = overlay(vec![row("a", true), row("b", false)], "a");
        assert_eq!(o.key(b"\r"), Action::Close);
        assert_eq!(o.key(b"x"), Action::Stop(PathBuf::from("/state/a.json")));
        o.key(b"j");
        assert_eq!(o.key(b"\r"), Action::Switch(PathBuf::from("/src/b")));
        // A stopped workspace can't be stopped.
        assert_eq!(o.key(b"x"), Action::Move);
        assert_eq!(o.error, "~/src/b is not running");
        assert_eq!(o.key(b"d"), Action::Detach);
        assert!(o.error.is_empty());
        assert_eq!(o.key(b"\x1b"), Action::Close);
        assert_eq!(o.key(b"z"), Action::None);
    }

    #[test]
    fn an_error_shows_in_the_last_row() {
        let mut o = overlay(vec![row("a", true)], "a");
        o.error = "the host did not start listening".into();
        let (_, lines) = screen(&o, 50, 5);
        assert_eq!(lines[4], " the host did not start listening");
    }

    #[test]
    fn the_selection_follows_its_workspace_across_reloads() {
        let mut o = overlay(vec![row("a", true), row("b", true)], "a");
        o.key(b"j");
        o.set_rows(vec![row("c", true), row("a", true), row("b", true)]);
        assert_eq!(o.selected, 2);
        o.set_rows(vec![row("c", true)]);
        assert_eq!(o.selected, 0);
    }
}
