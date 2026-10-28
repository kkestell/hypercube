//! Claude Code's usage limits. Claude Code gives its rate limits only to
//! status line commands, so `hypercube statusline` saves them in the state
//! directory, and the host draws them in its status bar.

use std::error::Error;
use std::fs;
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};

use crate::{store, workspace};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// The file in the state directory that holds the latest limits.
pub const FILE: &str = "claude-limits.json";

/// A usage window: the share used and when it resets, in Unix seconds.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Window {
    pub used_percentage: f64,
    pub resets_at: i64,
}

/// The 5-hour and 7-day windows, each present only if Claude Code reported it.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Limits {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub five_hour: Option<Window>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub seven_day: Option<Window>,
}

/// The path of the saved limits.
pub fn path() -> PathBuf {
    store::dir().join(FILE)
}

/// The saved limits, or none if there are none or the file is unreadable.
pub fn load(path: &Path) -> Option<Limits> {
    serde_json::from_slice(&fs::read(path).ok()?).ok()
}

/// Save the limits, through a temporary file renamed into place so a reader
/// always sees a whole file.
fn save(path: &Path, limits: &Limits) -> Result<()> {
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
    }
    let mut temp = path.as_os_str().to_owned();
    temp.push(format!(".{}.tmp", std::process::id()));
    let temp = PathBuf::from(temp);
    fs::write(&temp, serde_json::to_string(limits)?)
        .map_err(|e| format!("write {}: {e}", temp.display()))?;
    fs::rename(&temp, path).map_err(|e| {
        let _ = fs::remove_file(&temp);
        format!("rename {}: {e}", temp.display())
    })?;
    Ok(())
}

/// What Claude Code sends a status line command.
#[derive(Default, Deserialize)]
#[serde(default)]
struct Input {
    rate_limits: Option<Limits>,
    context_window: ContextWindow,
    workspace: Workspace,
}

#[derive(Default, Deserialize)]
#[serde(default)]
struct ContextWindow {
    used_percentage: f64,
}

#[derive(Default, Deserialize)]
#[serde(default)]
struct Workspace {
    current_dir: String,
}

/// The status line command of Claude Code panes. It saves the rate limits in
/// `limits_path` and writes the context use and the branch of the agent's
/// checkout, with `*` when it has uncommitted changes.
pub fn statusline(mut stdin: impl Read, mut stdout: impl Write, limits_path: &Path) -> Result<()> {
    let mut data = Vec::new();
    stdin.read_to_end(&mut data)?;
    let input: Input =
        serde_json::from_slice(&data).map_err(|e| format!("parse status line input: {e}"))?;
    if let Some(limits) = &input.rate_limits {
        save(limits_path, limits)?;
    }
    let mut line = format!("ctx {:.0}%", input.context_window.used_percentage);
    let dir = Path::new(&input.workspace.current_dir);
    if let Ok(mut branch) = workspace::branch(dir)
        && !branch.is_empty()
    {
        if workspace::dirty(dir).unwrap_or(false) {
            branch.push('*');
        }
        line.push_str(" · ");
        line.push_str(&branch);
    }
    stdout.write_all(line.as_bytes())?;
    Ok(())
}

/// The current time in Unix seconds.
pub fn now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |d| d.as_secs() as i64)
}

/// The status bar text for limits at `now`: each window that has not reset,
/// with its use and the time until it resets, such as
/// `claude 5h 22% ↻2h5m  7d 9% ↻3d4h`. Empty if no window is left.
pub fn render(limits: &Limits, now: i64) -> String {
    let windows = [("5h", &limits.five_hour), ("7d", &limits.seven_day)];
    let shown: Vec<String> = windows
        .iter()
        .filter_map(|&(label, w)| {
            let w = w.as_ref().filter(|w| w.resets_at > now)?;
            Some(format!(
                "{label} {:.0}% ↻{}",
                w.used_percentage,
                duration(w.resets_at - now)
            ))
        })
        .collect();
    if shown.is_empty() {
        return String::new();
    }
    format!("claude {}", shown.join("  "))
}

/// Seconds as `3d4h`, `2h13m`, or `47m`.
fn duration(seconds: i64) -> String {
    let (d, h, m) = (seconds / 86400, seconds % 86400 / 3600, seconds % 3600 / 60);
    match (d, h, m) {
        (1.., 1.., _) => format!("{d}d{h}h"),
        (1.., 0, _) => format!("{d}d"),
        (0, 1.., 1..) => format!("{h}h{m}m"),
        (0, 1.., 0) => format!("{h}h"),
        _ => format!("{m}m"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::TempDir;

    fn window(used: f64, resets_at: i64) -> Option<Window> {
        Some(Window {
            used_percentage: used,
            resets_at,
        })
    }

    #[test]
    fn durations() {
        for (seconds, want) in [
            (59, "0m"),
            (47 * 60, "47m"),
            (2 * 3600 + 13 * 60, "2h13m"),
            (3 * 3600, "3h"),
            (3 * 86400 + 4 * 3600, "3d4h"),
            (2 * 86400, "2d"),
            (86400 + 59 * 60, "1d"),
        ] {
            assert_eq!(duration(seconds), want, "{seconds}");
        }
    }

    #[test]
    fn render_shows_the_windows_that_have_not_reset() {
        let limits = Limits {
            five_hour: window(21.6, 1000 + 2 * 3600 + 5 * 60),
            seven_day: window(9.0, 1000 + 3 * 86400 + 4 * 3600),
        };
        assert_eq!(render(&limits, 1000), "claude 5h 22% ↻2h5m  7d 9% ↻3d4h");

        let reset = Limits {
            five_hour: window(80.0, 999),
            seven_day: window(9.0, 1000 + 86400),
        };
        assert_eq!(render(&reset, 1000), "claude 7d 9% ↻1d");
        // A window that resets this second has reset.
        assert_eq!(render(&Limits::default(), 1000), "");
        let only_reset = Limits {
            five_hour: window(1.0, 1000),
            ..Limits::default()
        };
        assert_eq!(render(&only_reset, 1000), "");
    }

    #[test]
    fn limits_parse_from_claude_codes_json_and_round_trip() {
        let json = r#"{"five_hour":{"used_percentage":12.5,"resets_at":1700},
            "seven_day":{"used_percentage":3,"resets_at":2900,"other":1},"extra":true}"#;
        let limits: Limits = serde_json::from_str(json).unwrap();
        assert_eq!(limits.five_hour, window(12.5, 1700));
        assert_eq!(limits.seven_day, window(3.0, 2900));

        let partial: Limits =
            serde_json::from_str(r#"{"five_hour":{"used_percentage":1,"resets_at":5}}"#).unwrap();
        assert_eq!(partial.seven_day, None);
        let text = serde_json::to_string(&partial).unwrap();
        assert!(!text.contains("seven_day"), "{text}");
        assert_eq!(serde_json::from_str::<Limits>(&text).unwrap(), partial);
    }

    #[test]
    fn load_ignores_missing_and_malformed_files() {
        let dir = TempDir::new();
        let path = dir.path().join(FILE);
        assert_eq!(load(&path), None);
        fs::write(&path, "{not json").unwrap();
        assert_eq!(load(&path), None);
        let limits = Limits {
            seven_day: window(4.0, 77),
            ..Limits::default()
        };
        save(&path, &limits).unwrap();
        assert_eq!(load(&path), Some(limits));
        let names: Vec<_> = fs::read_dir(dir.path())
            .unwrap()
            .map(|e| e.unwrap().file_name())
            .collect();
        assert_eq!(names, [FILE], "a temporary file remains");
    }

    fn run(input: &str, path: &Path) -> Result<String> {
        let mut out = Vec::new();
        statusline(input.as_bytes(), &mut out, path)?;
        Ok(String::from_utf8(out).unwrap())
    }

    #[test]
    fn statusline_saves_limits_and_prints_the_context_use() {
        let dir = TempDir::new();
        let path = dir.path().join("state").join(FILE);
        let input = r#"{"rate_limits":{"five_hour":{"used_percentage":40,"resets_at":900},
            "seven_day":{"used_percentage":10.4,"resets_at":9000}},
            "context_window":{"used_percentage":37.6},
            "workspace":{"current_dir":"/does/not/exist"}}"#;
        assert_eq!(run(input, &path).unwrap(), "ctx 38%");
        assert_eq!(
            load(&path),
            Some(Limits {
                five_hour: window(40.0, 900),
                seven_day: window(10.4, 9000),
            })
        );

        // Input without limits leaves the saved ones alone.
        assert_eq!(run(r#"{"context_window":{}}"#, &path).unwrap(), "ctx 0%");
        assert_eq!(load(&path).unwrap().five_hour, window(40.0, 900));

        assert!(run("not json", &path).is_err());
    }

    #[test]
    fn statusline_prints_the_branch_with_a_star_when_dirty() {
        let dir = TempDir::new();
        let repo = dir.path().join("repo");
        fs::create_dir(&repo).unwrap();
        let git = |args: &[&str]| {
            let out = std::process::Command::new("git")
                .arg("-C")
                .arg(&repo)
                .args(["-c", "user.name=t", "-c", "user.email=t@t"])
                .args(args)
                .output()
                .unwrap();
            assert!(out.status.success(), "git {args:?}");
        };
        git(&["init", "-q", "-b", "topic"]);
        git(&["commit", "-q", "--allow-empty", "-m", "first"]);
        let input = format!(
            r#"{{"context_window":{{"used_percentage":5}},"workspace":{{"current_dir":{:?}}}}}"#,
            repo.to_str().unwrap()
        );
        let path = dir.path().join(FILE);
        assert_eq!(run(&input, &path).unwrap(), "ctx 5% · topic");
        fs::write(repo.join("f"), "x").unwrap();
        assert_eq!(run(&input, &path).unwrap(), "ctx 5% · topic*");
    }
}
