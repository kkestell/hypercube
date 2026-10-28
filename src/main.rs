//! Hypercube runs a team of local coding agents, each a Claude Code or OpenCode
//! session in its own pane.

use std::error::Error;
use std::fs;
use std::io::{BufRead, ErrorKind, Write};
use std::path::{Path, PathBuf};
use std::process::ExitCode;

use hypercube::store::{Agent, Workspace};
use hypercube::{bar, client, control, hook, host, mcp, statusline, store, wait, workspace};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

const USAGE: &str = "usage: hypercube [start dir | stop [dir | --all] | delete [dir | --all] | status | running | mcp | hook <event> | wait | statusline [--limits]]";

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let args: Vec<&str> = args.iter().map(String::as_str).collect();
    let result = match args.as_slice() {
        [] => start(),
        ["host", dir] => host::serve_workspace(Path::new(dir)),
        ["start", dir] => workspace_at(dir).and_then(|(root, _)| client::start(&root)),
        ["stop"] => stop("."),
        ["stop", "--all"] => stop_all(),
        ["stop", dir] => stop(dir),
        ["delete"] => delete("."),
        ["delete", "--all"] => delete_all(),
        ["delete", dir] => delete(dir),
        ["status"] => status(),
        ["running"] => running(),
        ["mcp"] => mcp::serve(std::io::stdin().lock(), std::io::stdout().lock()),
        ["hook", event] => hook::run(event, std::io::stdin().lock(), std::io::stdout().lock()),
        // Status 2 tells the harness that the waiter wrote messages.
        ["wait"] => match wait::run(std::io::stdin().lock(), std::io::stderr().lock()) {
            Ok(true) => return ExitCode::from(2),
            result => result.map(|_| ()),
        },
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
    let root = workspace::locate(&std::env::current_dir()?, dir)?;
    let path = store::path(&workspace::name(&root));
    Ok((root, path))
}

/// Attach this terminal to the workspace containing the working directory,
/// starting its host if none is running.
fn start() -> Result<()> {
    let (root, _) = workspace_at(".")?;
    client::run(&root)
}

/// End the session of the workspace containing `dir`.
fn stop(dir: &str) -> Result<()> {
    let (root, path) = existing_workspace_at(dir)?;
    stop_workspace(&root, &path)
}

/// `workspace_at`, or for a checkout that was moved or deleted, the
/// workspace whose checkout was at `dir`, if it has a state file.
fn existing_workspace_at(dir: &str) -> Result<(PathBuf, PathBuf)> {
    workspace_at(dir).or_else(|e| {
        let root = workspace::spelled(&std::env::current_dir()?, dir)?;
        let path = store::path(&workspace::name(&root));
        if root.exists() || !path.exists() {
            return Err(e);
        }
        Ok((root, path))
    })
}

/// End the session of the workspace whose checkout is `root`.
fn stop_workspace(root: &Path, path: &Path) -> Result<()> {
    if !host::running(path)? {
        return Err(format!("hypercube is not running for {}", workspace::short(root)).into());
    }
    host::stop(path)
}

/// Run `run` on the workspace.
fn apply(w: &Workspace, mut run: impl FnMut(&Path, &Path) -> Result<()>) -> Result<()> {
    let root = w.root().ok_or("the state has no orchestrator")?;
    run(root, &w.path)
}

/// End the session of every running workspace, after the user confirms.
fn stop_all() -> Result<()> {
    let mut running = Vec::new();
    for w in store::workspaces()? {
        if host::running(&w.path)? {
            running.push(w);
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
        store::workspaces()?,
        std::io::stdin().lock(),
        std::io::stdout().lock(),
        delete_workspace,
    )
}

/// List `targets`, ask for confirmation, and run `command` on each. A
/// workspace that fails is reported at the end, and the rest still run.
fn all(
    command: &str,
    targets: Vec<Workspace>,
    mut input: impl BufRead,
    mut out: impl Write,
    mut run: impl FnMut(&Path, &Path) -> Result<()>,
) -> Result<()> {
    if targets.is_empty() {
        writeln!(out, "No workspaces to {command}.")?;
        return Ok(());
    }
    for w in &targets {
        writeln!(out, "{}", w.name())?;
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
        .filter_map(|w| Some(format!("{}: {}", w.name(), apply(w, &mut run).err()?)))
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
    let (root, path) = existing_workspace_at(dir)?;
    delete_workspace(&root, &path)
}

/// `delete` for the workspace whose checkout is `root`.
fn delete_workspace(root: &Path, path: &Path) -> Result<()> {
    // Refuse before stopping the session.
    removable_workers(root, path)?;
    if host::running(path)? {
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
        // Worktrees and branches live in the checkout, so a checkout that
        // was moved or deleted took them with it.
        if root.exists() {
            workspace::remove_worktree(root, Path::new(&a.dir), &a.branch)
                .map_err(|e| format!("{}: {e}", a.name))?;
        }
        store::update(path, |s| {
            s.remove(&a.id);
            Ok(())
        })?;
    }
    store::delete(path)?;
    for file in [
        control::socket_path(path)?,
        host::log_path(path),
        host::lock_path(path),
    ] {
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

/// Print the directory of every running workspace, one per line, in the order
/// the bar lists them.
fn running() -> Result<()> {
    for entry in bar::running()? {
        if let Some(root) = entry.root {
            println!("{}", root.display());
        }
    }
    Ok(())
}

/// Print every workspace with a state file: its directory, whether it is
/// running, its number of agents, when it was created, and when it was last
/// active, most recently active first. The state's lock file is created with
/// the state and never written, so its modification time is the creation
/// time.
fn status() -> Result<()> {
    let mut rows = Vec::new();
    for w in store::workspaces()? {
        let active = w.active()?;
        let created = fs::metadata(w.path.with_extension("lock"))
            .and_then(|m| m.modified())
            .unwrap_or(active);
        let session = match host::running(&w.path)? {
            true => "running",
            false => "stopped",
        };
        let row = [
            w.name(),
            session.to_string(),
            w.state.agents.len().to_string(),
            store::ago(created),
            store::ago(active),
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

#[cfg(test)]
mod tests {
    use super::*;
    use hypercube::store::State;

    fn known(name: &str, dir: Option<&str>) -> Workspace {
        let agents = dir.map(|dir| Agent {
            role: store::ORCHESTRATOR.into(),
            dir: dir.into(),
            ..Agent::default()
        });
        Workspace {
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
        targets: Vec<Workspace>,
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
