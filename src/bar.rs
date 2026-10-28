//! The workspace bar: the bottom row of the screen, which numbers every
//! running workspace in the order its host started. Ctrl-b and a workspace's
//! number switch the terminal to it.

use std::error::Error;
use std::fs;
use std::io::ErrorKind;
use std::path::PathBuf;
use std::time::SystemTime;

use crate::host::{self, fit, text_width};
use crate::mux::{Winsize, now_ms};
use crate::store;
use crate::theme;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// How long an error shows in place of the workspaces.
const ERROR_MS: i64 = 5000;

/// A running workspace as the bar lists it.
#[derive(Clone, Debug, PartialEq)]
pub struct Entry {
    /// The state file.
    pub path: PathBuf,
    /// The checkout, if the state has an orchestrator.
    pub root: Option<PathBuf>,
    /// The checkout's directory name, or the state's name without one.
    pub name: String,
}

#[derive(Default)]
pub struct Bar {
    pub entries: Vec<Entry>,
    /// The state file of the host's own workspace.
    pub current: PathBuf,
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

    /// Draw the bar across the last row in the footer's colors, with each
    /// workspace that fits after its number, and the host's own marked `*`
    /// and bold.
    pub fn draw(&self, s: &mut String, size: Winsize) {
        let (cols, row) = (size.ws_col, size.ws_row);
        if row == 0 {
            return;
        }
        s.push_str(&format!(
            "\x1b[{row};1H{}{}\x1b[{row};1H",
            theme::FOOTER.sgr(),
            " ".repeat(usize::from(cols))
        ));
        if !self.error.is_empty() {
            s.push_str(&fit(&format!(" {}", self.error), cols));
            return;
        }
        let mut used = 0;
        for (i, e) in self.entries.iter().enumerate() {
            let current = e.path == self.current;
            let gap = if i == 0 { " " } else { "  " };
            let mark = if current { "*" } else { "" };
            let text = format!("{i} {mark}{}", e.name);
            used += text_width(gap) + text_width(&text);
            if used > usize::from(cols) {
                break;
            }
            if current {
                s.push_str(&format!("{gap}\x1b[1m{text}\x1b[22m"));
            } else {
                s.push_str(&format!("{gap}{text}"));
            }
        }
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
        let root = store::load(&path).ok().and_then(|state| {
            let o = state
                .agents
                .iter()
                .find(|a| a.role == store::ORCHESTRATOR)?;
            Some(PathBuf::from(&o.dir))
        });
        let name = root
            .as_ref()
            .and_then(|r| r.file_name())
            .map_or_else(|| workspace.to_string(), |n| n.to_string_lossy().into());
        listed.push((started, Entry { path, root, name }));
    }
    listed.sort_by(|(a, x), (b, y)| a.cmp(b).then_with(|| x.path.cmp(&y.path)));
    Ok(listed.into_iter().map(|(_, entry)| entry).collect())
}

#[cfg(test)]
mod tests {
    use super::*;
    use libghostty_vt::Terminal;
    use libghostty_vt::terminal::{Point, PointCoordinate};

    fn entry(name: &str) -> Entry {
        Entry {
            path: PathBuf::from(format!("/state/{name}.json")),
            root: Some(PathBuf::from(format!("/src/{name}"))),
            name: name.into(),
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
        bar.draw(&mut s, size);
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

    fn bold(t: &Terminal, x: u16) -> bool {
        let point = Point::Active(PointCoordinate { x, y: 1 });
        t.grid_ref(point).unwrap().style().unwrap().bold
    }

    #[test]
    fn the_bar_numbers_the_workspaces_and_marks_the_current_one() {
        let t = screen(&bar(&["foo", "bar", "glarpy"], "bar"), 30);
        assert_eq!(row(&t), " 0 foo  1 *bar  2 glarpy      ");
        assert!(!bold(&t, 1) && !bold(&t, 7));
        assert!((8..14).all(|x| bold(&t, x)));
        assert!(!bold(&t, 14) && !bold(&t, 18));
    }

    #[test]
    fn only_the_workspaces_that_fit_are_shown() {
        let b = bar(&["foo", "bar", "glarpy"], "foo");
        assert_eq!(row(&screen(&b, 24)), " 0 *foo  1 bar  2 glarpy");
        assert_eq!(row(&screen(&b, 23)), " 0 *foo  1 bar         ");
        assert_eq!(row(&screen(&b, 6)), "      ");
    }

    #[test]
    fn an_error_shows_in_place_of_the_workspaces_until_it_expires() {
        let mut b = bar(&["foo"], "foo");
        b.fail("no such workspace".into());
        assert_eq!(row(&screen(&b, 20)), " no such workspace  ");
        assert!(!b.update(b.entries.clone()));
        b.error_at -= ERROR_MS;
        assert!(b.update(b.entries.clone()));
        assert_eq!(row(&screen(&b, 20)), " 0 *foo             ");
        assert!(b.update(vec![entry("foo"), entry("bar")]));
    }
}
