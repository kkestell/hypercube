//! The team list: every team of the workspace, drawn over the whole screen,
//! from which the user views a team, creates one, or deletes one.

use std::error::Error;
use std::path::{Path, PathBuf};

use crate::colors::{self, Palette, Rgb};
use crate::host::{self, fit};
use crate::icons;
use crate::mux::Winsize;
use crate::overlay::{HELP, ListRow, pad};
use crate::store::{self, State};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

const ERROR: &str = "\x1b[0;31m";

/// A team as the list shows it.
#[derive(Clone, Debug, PartialEq)]
pub struct Row {
    pub name: String,
    pub agents: usize,
    /// Whether any of its agents is busy.
    pub busy: bool,
    /// Whether its orchestrator has exited and is not running again.
    pub failed: bool,
    /// Whether any of its agents waits for a tool approval.
    pub attention: bool,
}

/// Each team of `state`, in `State::teams` order.
pub fn rows(state: &State) -> Vec<Row> {
    state
        .teams()
        .into_iter()
        .map(|team| {
            let members: Vec<_> = state.agents.iter().filter(|a| a.team == team).collect();
            Row {
                name: team.to_string(),
                agents: members.len(),
                busy: members.iter().any(|a| a.activity == store::BUSY),
                failed: state
                    .team_orchestrator(team)
                    .is_some_and(|o| o.activity == store::EXITED),
                attention: members.iter().any(|a| a.attention),
            }
        })
        .collect()
}

/// What a key in the team list asks the host to do.
#[derive(Debug, PartialEq)]
pub enum Action {
    None,
    /// The selection, the prompt, or the error changed.
    Move,
    /// View the named team.
    View(String),
    /// Create a team with this name.
    Create(String),
    /// Delete the named team.
    Delete(String),
    Close,
}

pub struct TeamList {
    rows: Vec<Row>,
    selected: usize,
    /// The workspace's state file.
    state_path: PathBuf,
    /// The viewed team, which the host keeps current.
    pub view: String,
    /// The name typed for a new team, while the prompt for it is open.
    prompt: Option<String>,
    /// Shown in the last row, until the next key.
    pub error: String,
}

impl TeamList {
    /// The list of the teams of the workspace whose state file is
    /// `state_path`, with the viewed team `view` selected.
    pub fn load(state_path: &Path, view: &str) -> Result<TeamList> {
        let mut list = TeamList {
            rows: Vec::new(),
            selected: 0,
            state_path: state_path.to_path_buf(),
            view: view.to_string(),
            prompt: None,
            error: String::new(),
        };
        list.reload()?;
        list.selected = list.rows.iter().position(|r| r.name == view).unwrap_or(0);
        Ok(list)
    }

    /// Read the list again, keeping the selected team selected.
    pub fn reload(&mut self) -> Result<()> {
        let state = store::load(&self.state_path)?;
        self.set_rows(rows(&state));
        Ok(())
    }

    fn set_rows(&mut self, rows: Vec<Row>) {
        let selected = self.rows.get(self.selected).map(|r| r.name.clone());
        self.rows = rows;
        self.selected = selected
            .and_then(|name| self.rows.iter().position(|r| r.name == name))
            .unwrap_or(0);
    }

    /// What a key of captured input asks for: Up and Down or k and j move,
    /// Enter views, n opens the prompt for a new team's name, x deletes, and
    /// Esc closes. While the prompt is open, typed characters build the
    /// name, Backspace deletes one, Enter creates the team, and Esc closes
    /// the prompt.
    pub fn key(&mut self, key: &[u8]) -> Action {
        self.error.clear();
        if let Some(name) = &mut self.prompt {
            return match key {
                b"\r" | b"\n" => {
                    let name = std::mem::take(name);
                    self.prompt = None;
                    Action::Create(name)
                }
                b"\x1b" => {
                    self.prompt = None;
                    Action::Move
                }
                [0x7f] | [0x08] => {
                    name.pop();
                    Action::Move
                }
                _ => match std::str::from_utf8(key) {
                    Ok(text) if !text.chars().any(char::is_control) => {
                        name.push_str(text);
                        Action::Move
                    }
                    _ => Action::None,
                },
            };
        }
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
                Some(r) => Action::View(r.name.clone()),
                None => Action::None,
            },
            b"n" => {
                self.prompt = Some(String::new());
                Action::Move
            }
            b"x" => match self.rows.get(self.selected) {
                Some(r) => Action::Delete(r.name.clone()),
                None => Action::None,
            },
            b"\x1b" => Action::Close,
            _ => Action::None,
        }
    }

    /// Draw the list over the whole screen: a title row, a row for each
    /// team, and the prompt, the error, or the keys in the last row. A
    /// team's row starts with its state's icon in the state's color, the
    /// viewed team's name is bold, and a bell ends the row of a team waiting
    /// for a tool approval.
    pub fn draw(&self, out: &mut Vec<u8>, size: Winsize, palette: Palette) {
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
        line(&mut s, 0, &palette.bar.sgr(), &pad(" Teams", cols));
        let width = self.rows.iter().map(|r| host::text_width(&r.name)).max();
        let width = width.unwrap_or(0);
        let list = rows.saturating_sub(2);
        // Scroll the list to keep the selection in view.
        let first = self.selected.saturating_sub(list.saturating_sub(1));
        for (i, r) in self.rows.iter().enumerate().skip(first).take(list) {
            let agents = if r.agents == 1 { "agent" } else { "agents" };
            let gap = " ".repeat(width - host::text_width(&r.name));
            let rest = format!("{gap}  {} {agents}", r.agents);
            let row = ListRow {
                state: state(r),
                name: &r.name,
                current: r.name == self.view,
                rest: &rest,
                attention: r.attention,
                selected: i == self.selected,
            };
            row.draw(&mut s, 1 + i - first, cols, palette);
        }
        s.push_str(&format!("\x1b[{};1H\x1b[0J", 2 + self.rows.len().min(list)));
        if rows > 1 {
            let (style, text) = match &self.prompt {
                Some(name) => ("\x1b[0m", format!(" New team: {name}")),
                None if !self.error.is_empty() => (ERROR, format!(" {}", self.error)),
                None => (
                    HELP,
                    " ↑↓ move  enter view  n new  x delete  esc close".to_string(),
                ),
            };
            line(&mut s, rows - 1, style, &text);
        }
        out.extend_from_slice(s.as_bytes());
    }
}

/// A team's state icon and its color: running while an agent is busy,
/// failed while its orchestrator has exited, and idle otherwise.
fn state(r: &Row) -> (char, Rgb) {
    if r.busy {
        (icons::RUNNING, colors::RUNNING)
    } else if r.failed {
        (icons::FAILED, colors::ERROR)
    } else {
        (icons::IDLE, colors::QUIET)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::Agent;
    use libghostty_vt::Terminal;
    use libghostty_vt::style::StyleColor;
    use libghostty_vt::terminal::{Point, PointCoordinate};

    fn row(name: &str, agents: usize) -> Row {
        Row {
            name: name.into(),
            agents,
            busy: false,
            failed: false,
            attention: false,
        }
    }

    fn list(rows: Vec<Row>, view: &str) -> TeamList {
        let selected = rows.iter().position(|r| r.name == view).unwrap_or(0);
        TeamList {
            rows,
            selected,
            state_path: PathBuf::new(),
            view: view.into(),
            prompt: None,
            error: String::new(),
        }
    }

    fn screen(l: &TeamList, cols: u16, rows: u16) -> (Terminal<'static, 'static>, Vec<String>) {
        let mut out = Vec::new();
        l.draw(
            &mut out,
            Winsize {
                ws_col: cols,
                ws_row: rows,
                ws_xpixel: 0,
                ws_ypixel: 0,
            },
            Palette::default(),
        );
        let mut t = Terminal::new(cols, rows).unwrap();
        // Something underneath that the list must cover.
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
    fn rows_count_each_teams_agents_and_their_states() {
        let agent = |id: &str, role: &str, team: &str, activity: &str| Agent {
            id: id.into(),
            role: role.into(),
            team: team.into(),
            activity: activity.into(),
            ..Agent::default()
        };
        let mut state = State {
            agents: vec![
                agent("o", store::ORCHESTRATOR, store::DEFAULT_TEAM, store::IDLE),
                agent("d", store::ORCHESTRATOR, "docs", store::EXITED),
                agent("w", store::WORKER, store::DEFAULT_TEAM, store::BUSY),
                agent("r", store::ORCHESTRATOR, "review", store::IDLE),
            ],
            ..State::default()
        };
        state.agents[3].attention = true;
        assert_eq!(
            rows(&state),
            [
                Row {
                    busy: true,
                    ..row(store::DEFAULT_TEAM, 2)
                },
                Row {
                    failed: true,
                    ..row("docs", 1)
                },
                Row {
                    attention: true,
                    ..row("review", 1)
                },
            ]
        );
    }

    #[test]
    fn the_list_shows_each_teams_state_count_and_bell() {
        let rows = vec![
            Row {
                busy: true,
                ..row("default", 3)
            },
            Row {
                failed: true,
                attention: true,
                ..row("docs", 1)
            },
            row("review", 1),
        ];
        let l = list(rows, "docs");
        let (t, lines) = screen(&l, 50, 6);
        assert_eq!(
            lines,
            [
                " Teams",
                " \u{f04b} default  3 agents",
                " \u{f071} docs     1 agent \u{f0f3}",
                " \u{f04c} review   1 agent",
                "",
                " ↑↓ move  enter view  n new  x delete  esc close",
            ]
        );
        let style = |x, y| {
            let point = Point::Active(PointCoordinate { x, y });
            t.grid_ref(point).unwrap().style().unwrap()
        };
        let rgb = |c| match c {
            StyleColor::Rgb(c) => (c.r, c.g, c.b),
            other => panic!("color {other:?}"),
        };
        let selected = Palette::default().selected;
        assert_eq!(rgb(style(1, 1).fg_color), colors::RUNNING);
        assert_eq!(rgb(style(1, 2).fg_color), colors::ERROR);
        assert_eq!(rgb(style(1, 3).fg_color), colors::QUIET);
        // The viewed team is selected and bold.
        assert_eq!(rgb(style(3, 2).bg_color), selected.bg);
        assert_eq!(rgb(style(49, 2).bg_color), selected.bg);
        assert!(style(3, 2).bold && style(6, 2).bold && !style(7, 2).bold);
        assert!(!style(3, 1).bold && !style(3, 3).bold);
        assert_eq!(rgb(style(20, 2).fg_color), colors::ATTENTION);
        assert_eq!(rgb(style(20, 2).bg_color), selected.bg);
    }

    #[test]
    fn the_prompt_and_an_error_show_in_the_last_row() {
        let mut l = list(vec![row("default", 1)], "default");
        l.key(b"n");
        l.key(b"q");
        l.key(b"a");
        let (_, lines) = screen(&l, 50, 4);
        assert_eq!(lines[3], " New team: qa");
        l.key(b"\r");
        l.error = "a team named qa already exists".into();
        let (_, lines) = screen(&l, 50, 4);
        assert_eq!(lines[3], " a team named qa already exists");
    }

    #[test]
    fn keys_move_view_delete_and_close() {
        let mut l = list(vec![row("default", 2), row("docs", 1)], "docs");
        assert_eq!(l.selected, 1);
        assert_eq!(l.key(b"j"), Action::Move);
        assert_eq!(l.selected, 1);
        for (key, selected) in [(&b"k"[..], 0), (b"\x1b[B", 1), (b"\x1bOA", 0)] {
            assert_eq!(l.key(key), Action::Move);
            assert_eq!(l.selected, selected, "{key:?}");
        }
        assert_eq!(l.key(b"\r"), Action::View("default".into()));
        l.key(b"j");
        assert_eq!(l.key(b"x"), Action::Delete("docs".into()));
        l.error = "no".into();
        assert_eq!(l.key(b"\x1b"), Action::Close);
        assert!(l.error.is_empty());
        assert_eq!(l.key(b"z"), Action::None);
    }

    #[test]
    fn keys_edit_submit_and_cancel_the_prompt() {
        let mut l = list(vec![row("default", 1)], "default");
        assert_eq!(l.key(b"n"), Action::Move);
        for key in [&b"q"[..], b"x", b"\x7f", b"a", b"\x1b[A"] {
            l.key(key);
        }
        assert_eq!(l.prompt.as_deref(), Some("qa"));
        // Keys that move or delete type into the prompt.
        assert_eq!(l.key(b"j"), Action::Move);
        assert_eq!(l.key(b"\r"), Action::Create("qaj".into()));
        assert_eq!(l.prompt, None);
        l.key(b"n");
        l.key(b"a");
        assert_eq!(l.key(b"\x1b"), Action::Move);
        assert_eq!(l.prompt, None);
        assert_eq!(l.key(b"\x1b"), Action::Close);
    }

    #[test]
    fn the_selection_follows_its_team_across_reloads() {
        let mut l = list(vec![row("default", 1), row("docs", 1)], "docs");
        l.set_rows(vec![row("default", 1), row("api", 1), row("docs", 1)]);
        assert_eq!(l.selected, 2);
        l.set_rows(vec![row("default", 1)]);
        assert_eq!(l.selected, 0);
    }
}
