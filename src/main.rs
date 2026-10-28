//! Hypercube runs a team of local coding agents, each a Claude Code or Pi
//! session in its own pane.

use std::error::Error;
use std::fs;
use std::io::ErrorKind;
use std::path::{Path, PathBuf};
use std::process::ExitCode;
use std::time::{Duration, SystemTime};

use hypercube::{control, hook, host, mcp, store, workspace};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

const USAGE: &str = "usage: hypercube [stop [dir] | delete [dir] | status | mcp | hook <event>]";

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let args: Vec<&str> = args.iter().map(String::as_str).collect();
    let result = match args.as_slice() {
        [] => start(),
        ["stop"] => stop("."),
        ["stop", dir] => stop(dir),
        ["delete"] => delete("."),
        ["delete", dir] => delete(dir),
        ["status"] => status(),
        ["mcp"] => mcp::serve(std::io::stdin().lock(), std::io::stdout().lock()),
        ["hook", event] => hook::run(event, std::io::stdin().lock(), std::io::stdout().lock()),
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
    if host::running(&path)?.is_none() {
        return Err(format!("hypercube is not running for {}", workspace::short(&root)).into());
    }
    host::stop(&path)
}

/// Stop the workspace containing `dir` and delete its state, worker
/// worktrees, and worker branches. Refuses, changing nothing, if a worker has
/// uncommitted changes in its worktree or an unmerged branch. Workers without
/// worktrees have their directories left alone.
fn delete(dir: &str) -> Result<()> {
    let (root, path) = workspace_at(dir)?;
    let state = store::load(&path)?;
    let workers: Vec<_> = state
        .agents
        .iter()
        .filter(|a| a.role == store::WORKER && !a.branch.is_empty())
        .collect();
    let problems: Vec<String> = workers
        .iter()
        .filter_map(|a| {
            let err = workspace::removable(&root, Path::new(&a.dir), &a.branch).err()?;
            Some(format!("{}: {err}", a.name))
        })
        .collect();
    if !problems.is_empty() {
        return Err(format!("not deleting the workspace:\n  {}", problems.join("\n  ")).into());
    }

    if host::running(&path)?.is_some() {
        host::stop(&path)?;
    }
    // Holding the host lock keeps a host from starting until the workspace is
    // gone.
    let Some(_lock) = host::try_lock(&host::lock_path(&path))? else {
        return Err(format!(
            "hypercube started again for {}; stop it and delete again",
            workspace::short(&root)
        )
        .into());
    };
    // A worker can change its worktree before the session ends. Such a worker
    // stays in the state, so delete can run again after it is fixed.
    for a in workers {
        workspace::remove_worktree(&root, Path::new(&a.dir), &a.branch)
            .map_err(|e| format!("{}: {e}", a.name))?;
        store::update(&path, |s| {
            s.remove(&a.id);
            Ok(())
        })?;
    }
    store::delete(&path)?;
    for file in [control::socket_path(&path)?, host::lock_path(&path)] {
        match fs::remove_file(&file) {
            Err(e) if e.kind() != ErrorKind::NotFound => {
                return Err(format!("remove {}: {e}", file.display()).into());
            }
            _ => {}
        }
    }
    Ok(())
}

/// Print every workspace with a state file: its directory, whether it is
/// running, its number of agents, when it was created, and when it was last
/// active, most recently active first. The state's lock file is created with
/// the state and never written, so its modification time is the creation
/// time. Every hook writes the state, so the state file's modification time is
/// the last activity.
fn status() -> Result<()> {
    let mut rows = Vec::new();
    let entries = match fs::read_dir(store::dir()) {
        Ok(entries) => entries.collect::<std::io::Result<Vec<_>>>()?,
        Err(e) if e.kind() == ErrorKind::NotFound => Vec::new(),
        Err(e) => return Err(format!("read {}: {e}", store::dir().display()).into()),
    };
    for entry in entries {
        let path = entry.path();
        if path.extension().is_none_or(|e| e != "json") {
            continue;
        }
        let state = store::load(&path)?;
        let active = fs::metadata(&path)
            .and_then(|m| m.modified())
            .map_err(|e| format!("stat {}: {e}", path.display()))?;
        let created = fs::metadata(path.with_extension("lock"))
            .and_then(|m| m.modified())
            .unwrap_or(active);
        let dir = match state.agents.iter().find(|a| a.role == store::ORCHESTRATOR) {
            Some(o) => workspace::short(Path::new(&o.dir)),
            None => store::workspace(&path),
        };
        let session = match host::running(&path)? {
            Some(_) => "running",
            None => "stopped",
        };
        let row = [
            dir,
            session.to_string(),
            state.agents.len().to_string(),
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
