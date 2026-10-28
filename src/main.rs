//! Hypercube runs a team of local coding agents, each a Claude Code or Pi
//! session in its own pane.

use std::error::Error;
use std::fs;
use std::io::{BufRead, ErrorKind, Write};
use std::path::{Path, PathBuf};
use std::process::ExitCode;
use std::time::{Duration, SystemTime};

use hypercube::store::{Agent, State};
use hypercube::{control, hook, host, mcp, statusline, store, workspace};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

const USAGE: &str = "usage: hypercube [stop [dir | --all] | delete [dir | --all] | status | mcp | hook <event> | statusline [--limits]]";

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let args: Vec<&str> = args.iter().map(String::as_str).collect();
    let result = match args.as_slice() {
        [] => start(),
        ["stop"] => stop("."),
        ["stop", "--all"] => stop_all(),
        ["stop", dir] => stop(dir),
        ["delete"] => delete("."),
        ["delete", "--all"] => delete_all(),
        ["delete", dir] => delete(dir),
        ["status"] => status(),
        ["mcp"] => mcp::serve(std::io::stdin().lock(), std::io::stdout().lock()),
        ["hook", event] => hook::run(event, std::io::stdin().lock(), std::io::stdout().lock()),
        ["statusline"] => statusline::run(false, std::io::stdin().lock(), std::io::stdout().lock()),
        ["statusline", "--limits"] => {
            statusline::run(true, std::io::stdin().lock(), std::io::stdout().lock())
        }
        _ => {
            eprintln!("{USAGE}");
            return ExitCode::from(2);
        }
    };
    match result {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("hypercube: {e}");
            ExitCode::FAILURE
        }
    }
}

/// The root of the workspace containing `dir` and its state file. The root is
/// the top of the checkout, or the directory itself outside Git.
fn workspace_at(dir: &str) -> Result<(PathBuf, PathBuf)> {
    let wd = std::env::current_dir()?;
    let mut root = workspace::resolve(&wd, dir)?;
    if let Ok(top) = workspace::root(&root) {
        root = top;
    }
    let path = store::path(&workspace::name(&root));
    Ok((root, path))
}

/// Host the workspace containing the working directory in this terminal.
fn start() -> Result<()> {
    let (root, _) = workspace_at(".")?;
    host::run(&root)
}

/// End the session of the workspace containing `dir`.
fn stop(dir: &str) -> Result<()> {
    let (root, path) = workspace_at(dir)?;
    stop_workspace(&root, &path)
}

/// End the session of the workspace whose checkout is `root`.
fn stop_workspace(root: &Path, path: &Path) -> Result<()> {
    if host::running(path)?.is_none() {
        return Err(format!("hypercube is not running for {}", workspace::short(root)).into());
    }
    host::stop(path)
}

/// A workspace with a state file.
struct Known {
    path: PathBuf,
    state: State,
}

impl Known {
    /// The workspace's checkout: the orchestrator's directory.
    fn root(&self) -> Option<&Path> {
        let o = self
            .state
            .agents
            .iter()
            .find(|a| a.role == store::ORCHESTRATOR)?;
        Some(Path::new(&o.dir))
    }

    /// The text that names the workspace: its directory, or its state's name
    /// when the state has no orchestrator.
    fn name(&self) -> String {
        self.root()
            .map_or_else(|| store::workspace(&self.path), workspace::short)
    }

    /// Run `run` on the workspace.
    fn apply(&self, mut run: impl FnMut(&Path, &Path) -> Result<()>) -> Result<()> {
        let root = self.root().ok_or("the state has no orchestrator")?;
        run(root, &self.path)
    }
}

/// Every workspace with a state file.
fn workspaces() -> Result<Vec<Known>> {
    let entries = match fs::read_dir(store::dir()) {
        Ok(entries) => entries.collect::<std::io::Result<Vec<_>>>()?,
        Err(e) if e.kind() == ErrorKind::NotFound => Vec::new(),
        Err(e) => return Err(format!("read {}: {e}", store::dir().display()).into()),
    };
    let mut list = Vec::new();
    for entry in entries {
        let path = entry.path();
        if path.extension().is_none_or(|e| e != "json") {
            continue;
        }
        let state = store::load(&path)?;
        list.push(Known { path, state });
    }
    Ok(list)
}

/// End the session of every running workspace, after the user confirms.
fn stop_all() -> Result<()> {
    let mut running = Vec::new();
    for k in workspaces()? {
        if host::running(&k.path)?.is_some() {
            running.push(k);
        }
    }
    all(
        "stop",
        running,
        std::io::stdin().lock(),
        std::io::stdout().lock(),
        stop_workspace,
    )
}

/// Delete every workspace, after the user confirms.
fn delete_all() -> Result<()> {
    all(
        "delete",
        workspaces()?,
        std::io::stdin().lock(),
        std::io::stdout().lock(),
        delete_workspace,
    )
}

/// List `targets`, ask for confirmation, and run `command` on each. A
/// workspace that fails is reported at the end, and the rest still run.
fn all(
    command: &str,
    targets: Vec<Known>,
    mut input: impl BufRead,
    mut out: impl Write,
    mut run: impl FnMut(&Path, &Path) -> Result<()>,
) -> Result<()> {
    if targets.is_empty() {
        writeln!(out, "No workspaces to {command}.")?;
        return Ok(());
    }
    for k in &targets {
        writeln!(out, "{}", k.name())?;
    }
    let capitalized = command[..1].to_uppercase() + &command[1..];
    write!(out, "{capitalized} all of these? [y/N] ")?;
    out.flush()?;
    let mut answer = String::new();
    input.read_line(&mut answer)?;
    if !matches!(answer.trim().to_lowercase().as_str(), "y" | "yes") {
        return Err(format!("{command} cancelled").into());
    }
    let failed: Vec<String> = targets
        .iter()
        .filter_map(|k| Some(format!("{}: {}", k.name(), k.apply(&mut run).err()?)))
        .collect();
    if !failed.is_empty() {
        return Err(format!("{command} failed for:\n{}", failed.join("\n")).into());
    }
    Ok(())
}

/// Stop the workspace containing `dir` and delete its state, worker
/// worktrees, and worker branches. Refuses, changing nothing, if a worker has
/// uncommitted changes in its worktree or an unmerged branch. Workers without
/// worktrees have their directories left alone.
fn delete(dir: &str) -> Result<()> {
    let (root, path) = workspace_at(dir)?;
    delete_workspace(&root, &path)
}

/// `delete` for the workspace whose checkout is `root`.
fn delete_workspace(root: &Path, path: &Path) -> Result<()> {
    // Refuse before stopping the session.
    removable_workers(root, path)?;
    if host::running(path)?.is_some() {
        host::stop(path)?;
    }
    // Holding the host lock keeps a host from starting until the workspace is
    // gone.
    let Some(_lock) = host::try_lock(&host::lock_path(path))? else {
        return Err(format!(
            "hypercube started again for {}; stop it and delete again",
            workspace::short(root)
        )
        .into());
    };
    // Workers may have come or changed while the session ran.
    let workers = removable_workers(root, path)?;
    // Each worker leaves the state once its worktree is gone, so after a
    // failed removal delete can run again.
    for a in workers {
        workspace::remove_worktree(root, Path::new(&a.dir), &a.branch)
            .map_err(|e| format!("{}: {e}", a.name))?;
        store::update(path, |s| {
            s.remove(&a.id);
            Ok(())
        })?;
    }
    store::delete(path)?;
    for file in [control::socket_path(path)?, host::lock_path(path)] {
        match fs::remove_file(&file) {
            Err(e) if e.kind() != ErrorKind::NotFound => {
                return Err(format!("remove {}: {e}", file.display()).into());
            }
            _ => {}
        }
    }
    Ok(())
}

/// The workspace's workers with worktrees, or an error naming each one whose
/// worktree is dirty or whose branch is not merged.
fn removable_workers(root: &Path, path: &Path) -> Result<Vec<Agent>> {
    let workers: Vec<Agent> = store::load(path)?
        .agents
        .into_iter()
        .filter(|a| a.role == store::WORKER && !a.branch.is_empty())
        .collect();
    let problems: Vec<String> = workers
        .iter()
        .filter_map(|a| {
            let err = workspace::removable(root, Path::new(&a.dir), &a.branch).err()?;
            Some(format!("{}: {err}", a.name))
        })
        .collect();
    if !problems.is_empty() {
        return Err(format!("not deleting the workspace:\n  {}", problems.join("\n  ")).into());
    }
    Ok(workers)
}

/// Print every workspace with a state file: its directory, whether it is
/// running, its number of agents, when it was created, and when it was last
/// active, most recently active first. The state's lock file is created with
/// the state and never written, so its modification time is the creation
/// time. Every hook writes the state, so the state file's modification time is
/// the last activity.
fn status() -> Result<()> {
    let mut rows = Vec::new();
    for k in workspaces()? {
        let path = &k.path;
        let active = fs::metadata(path)
            .and_then(|m| m.modified())
            .map_err(|e| format!("stat {}: {e}", path.display()))?;
        let created = fs::metadata(path.with_extension("lock"))
            .and_then(|m| m.modified())
            .unwrap_or(active);
        let session = match host::running(path)? {
            Some(_) => "running",
            None => "stopped",
        };
        let row = [
            k.name(),
            session.to_string(),
            k.state.agents.len().to_string(),
            ago(created),
            ago(active),
        ];
        rows.push((active, row));
    }
    rows.sort_by_key(|&(active, _)| std::cmp::Reverse(active));
    let header = ["WORKSPACE", "SESSION", "AGENTS", "CREATED", "ACTIVE"].map(String::from);
    let table: Vec<[String; 5]> = std::iter::once(header)
        .chain(rows.into_iter().map(|(_, row)| row))
        .collect();
    let widths: Vec<usize> = (0..5)
        .map(|i| {
            table
                .iter()
                .map(|r| r[i].chars().count())
                .max()
                .unwrap_or(0)
        })
        .collect();
    for row in &table {
        let line: String = row
            .iter()
            .zip(&widths)
            .map(|(cell, &width)| format!("{cell:<width$}  "))
            .collect();
        println!("{}", line.trim_end());
    }
    Ok(())
}

/// How long before now `time` was.
fn ago(time: SystemTime) -> String {
    let d = SystemTime::now()
        .duration_since(time)
        .unwrap_or(Duration::ZERO)
        .as_secs();
    match d {
        0..60 => "just now".to_string(),
        60..3600 => format!("{}m ago", d / 60),
        3600..86400 => format!("{}h ago", d / 3600),
        _ => format!("{}d ago", d / 86400),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn known(name: &str, dir: Option<&str>) -> Known {
        let agents = dir.map(|dir| Agent {
            role: store::ORCHESTRATOR.into(),
            dir: dir.into(),
            ..Agent::default()
        });
        Known {
            path: PathBuf::from(format!("/state/{name}.json")),
            state: State {
                agents: agents.into_iter().collect(),
                ..State::default()
            },
        }
    }

    /// Run `all` with `answer` typed, returning what it printed, the roots it
    /// ran on, and its result.
    fn run_all(
        command: &str,
        targets: Vec<Known>,
        answer: &str,
        fail: &[&str],
    ) -> (String, Vec<String>, Result<()>) {
        let mut out = Vec::new();
        let mut ran = Vec::new();
        let result = all(
            command,
            targets,
            answer.as_bytes(),
            &mut out,
            |root, path| {
                ran.push(format!("{} {}", root.display(), path.display()));
                match fail.contains(&root.to_str().unwrap()) {
                    true => Err("refused".into()),
                    false => Ok(()),
                }
            },
        );
        (String::from_utf8(out).unwrap(), ran, result)
    }

    #[test]
    fn all_without_workspaces_does_nothing() {
        let (out, ran, result) = run_all("stop", Vec::new(), "", &[]);
        assert_eq!(out, "No workspaces to stop.\n");
        assert!(ran.is_empty());
        assert!(result.is_ok());
    }

    #[test]
    fn all_lists_the_workspaces_and_runs_on_each_after_yes() {
        for answer in ["y\n", "Yes\n", " YES \n"] {
            let targets = vec![known("a", Some("/r/a")), known("b", Some("/r/b"))];
            let (out, ran, result) = run_all("stop", targets, answer, &[]);
            assert_eq!(out, "/r/a\n/r/b\nStop all of these? [y/N] ");
            assert_eq!(ran, ["/r/a /state/a.json", "/r/b /state/b.json"]);
            assert!(result.is_ok(), "{answer:?}");
        }
    }

    #[test]
    fn all_runs_on_nothing_unless_confirmed() {
        for answer in ["", "\n", "n\n", "no\n", "yy\n"] {
            let (out, ran, result) = run_all("delete", vec![known("a", Some("/r/a"))], answer, &[]);
            assert_eq!(out, "/r/a\nDelete all of these? [y/N] ");
            assert!(ran.is_empty(), "{answer:?}");
            assert_eq!(result.unwrap_err().to_string(), "delete cancelled");
        }
    }

    #[test]
    fn all_reports_refusals_and_continues() {
        let targets = vec![
            known("a", Some("/r/a")),
            known("b", Some("/r/b")),
            known("c", None),
            known("d", Some("/r/d")),
        ];
        let (_, ran, result) = run_all("delete", targets, "y\n", &["/r/a", "/r/d"]);
        // The workspace without an orchestrator is reported by its state's
        // name and never run.
        assert_eq!(
            ran,
            [
                "/r/a /state/a.json",
                "/r/b /state/b.json",
                "/r/d /state/d.json"
            ]
        );
        assert_eq!(
            result.unwrap_err().to_string(),
            "delete failed for:\n/r/a: refused\nc: the state has no orchestrator\n/r/d: refused"
        );
    }
}
