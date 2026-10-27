//! The Git checkout, worker branches and worktrees, and worker directories.

use std::error::Error;
use std::ffi::OsStr;
use std::io::Write;
use std::os::unix::ffi::OsStrExt;
use std::path::{Component, Path, PathBuf};
use std::process::{Command, Stdio};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

const EXCLUDE_RULE: &str = "/.hypercube/";

fn utf8(path: &Path) -> Result<&str> {
    path.to_str()
        .ok_or_else(|| format!("{} is not valid UTF-8", path.display()).into())
}

/// Run `git` in `dir` and return its trimmed standard output.
fn git(dir: &Path, args: &[&str]) -> Result<String> {
    let out = Command::new("git")
        .args(args)
        .current_dir(dir)
        .output()
        .map_err(|e| format!("git {}: {e}", args.join(" ")))?;
    if !out.status.success() {
        let mut message = String::from_utf8_lossy(&out.stderr).into_owned();
        if message.trim().is_empty() {
            message = String::from_utf8_lossy(&out.stdout).into_owned();
        }
        return Err(format!("git {}: {}", args.join(" "), message.trim()).into());
    }
    Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
}

fn home() -> Option<PathBuf> {
    std::env::var_os("HOME")
        .filter(|h| !h.is_empty())
        .map(PathBuf::from)
}

/// Remove `.` components and resolve `..` against the preceding component,
/// without consulting the filesystem.
fn clean(path: &Path) -> PathBuf {
    let mut out = PathBuf::new();
    for component in path.components() {
        match component {
            Component::CurDir => {}
            Component::ParentDir => match out.components().next_back() {
                Some(Component::Normal(_)) => {
                    out.pop();
                }
                Some(Component::RootDir) => {}
                _ => out.push(".."),
            },
            c => out.push(c),
        }
    }
    if out.as_os_str().is_empty() {
        out.push(".");
    }
    out
}

/// `dir` as an absolute path. A leading `~/` is the home directory, and a
/// relative path is relative to `base`. `dir` must be an existing directory.
pub fn resolve(base: &Path, dir: &str) -> Result<PathBuf> {
    resolve_in(home().as_deref(), base, dir)
}

fn resolve_in(home: Option<&Path>, base: &Path, dir: &str) -> Result<PathBuf> {
    let path = if let Some(rest) = dir.strip_prefix("~/") {
        home.ok_or("HOME is not set")?.join(rest)
    } else if Path::new(dir).is_absolute() {
        PathBuf::from(dir)
    } else {
        base.join(dir)
    };
    let path = clean(&path);
    let meta = std::fs::metadata(&path).map_err(|e| format!("{}: {e}", path.display()))?;
    if !meta.is_dir() {
        return Err(format!("{} is not a directory", path.display()).into());
    }
    Ok(path)
}

/// The top level of the checkout containing `dir`.
pub fn root(dir: &Path) -> Result<PathBuf> {
    Ok(git(dir, &["rev-parse", "--show-toplevel"])?.into())
}

/// The branch checked out in `dir`.
pub fn branch(dir: &Path) -> Result<String> {
    git(dir, &["branch", "--show-current"])
}

/// `path` with the home directory written as `~`.
pub fn short(path: &Path) -> String {
    short_in(home().as_deref(), path)
}

fn short_in(home: Option<&Path>, path: &Path) -> String {
    match home.and_then(|h| path.strip_prefix(h).ok()) {
        Some(rest) if rest.as_os_str().is_empty() => "~".to_string(),
        Some(rest) => format!("~/{}", rest.display()),
        None => path.display().to_string(),
    }
}

/// A workspace's name from its checkout path: the directory name and the
/// start of a hash of the path. It names the state file and the host's files.
pub fn name(root: &Path) -> String {
    let base = root
        .file_name()
        .unwrap_or(OsStr::new("/"))
        .to_string_lossy();
    let mut name = String::new();
    let mut in_run = false;
    for c in base.chars() {
        if c.is_ascii_alphanumeric() || c == '_' || c == '-' {
            name.push(c);
            in_run = false;
        } else if !in_run {
            name.push('-');
            in_run = true;
        }
    }
    let hash = sha256(root.as_os_str().as_bytes());
    format!("{name}-{:02x}{:02x}{:02x}", hash[0], hash[1], hash[2])
}

fn sha256(data: &[u8]) -> [u8; 32] {
    const K: [u32; 64] = [
        0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4,
        0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe,
        0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f,
        0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
        0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc,
        0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
        0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116,
        0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
        0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7,
        0xc67178f2,
    ];
    let mut h: [u32; 8] = [
        0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab,
        0x5be0cd19,
    ];
    let mut message = data.to_vec();
    message.push(0x80);
    while message.len() % 64 != 56 {
        message.push(0);
    }
    message.extend_from_slice(&(data.len() as u64 * 8).to_be_bytes());
    for block in message.chunks(64) {
        let mut w = [0u32; 64];
        for (i, word) in block.chunks(4).enumerate() {
            w[i] = u32::from_be_bytes([word[0], word[1], word[2], word[3]]);
        }
        for i in 16..64 {
            let s0 = w[i - 15].rotate_right(7) ^ w[i - 15].rotate_right(18) ^ (w[i - 15] >> 3);
            let s1 = w[i - 2].rotate_right(17) ^ w[i - 2].rotate_right(19) ^ (w[i - 2] >> 10);
            w[i] = w[i - 16]
                .wrapping_add(s0)
                .wrapping_add(w[i - 7])
                .wrapping_add(s1);
        }
        let [mut a, mut b, mut c, mut d, mut e, mut f, mut g, mut hh] = h;
        for i in 0..64 {
            let s1 = e.rotate_right(6) ^ e.rotate_right(11) ^ e.rotate_right(25);
            let ch = (e & f) ^ (!e & g);
            let t1 = hh
                .wrapping_add(s1)
                .wrapping_add(ch)
                .wrapping_add(K[i])
                .wrapping_add(w[i]);
            let s0 = a.rotate_right(2) ^ a.rotate_right(13) ^ a.rotate_right(22);
            let maj = (a & b) ^ (a & c) ^ (b & c);
            let t2 = s0.wrapping_add(maj);
            hh = g;
            g = f;
            f = e;
            e = d.wrapping_add(t1);
            d = c;
            c = b;
            b = a;
            a = t1.wrapping_add(t2);
        }
        for (state, value) in h.iter_mut().zip([a, b, c, d, e, f, g, hh]) {
            *state = state.wrapping_add(value);
        }
    }
    let mut out = [0u8; 32];
    for (bytes, word) in out.chunks_mut(4).zip(h) {
        bytes.copy_from_slice(&word.to_be_bytes());
    }
    out
}

/// A worker's branch.
pub fn worker_branch(name: &str) -> String {
    format!("hypercube/{name}")
}

/// Where a worker's worktree lives. Worktrees sit inside the checkout so that
/// harnesses extend the checkout's folder trust to them.
pub fn worktree_dir(root: &Path, name: &str) -> PathBuf {
    root.join(".hypercube").join(name)
}

/// Create the worker's branch from HEAD and check it out in `dir`. Adds
/// `.hypercube/` to the checkout's local excludes.
pub fn add_worktree(root: &Path, dir: &Path, branch: &str) -> Result<()> {
    let exclude = PathBuf::from(git(root, &["rev-parse", "--git-path", "info/exclude"])?);
    let exclude = root.join(exclude);
    let data = std::fs::read_to_string(&exclude).unwrap_or_default();
    if !data.lines().any(|line| line == EXCLUDE_RULE) {
        let separator = if data.is_empty() || data.ends_with('\n') {
            ""
        } else {
            "\n"
        };
        if let Some(parent) = exclude.parent() {
            std::fs::create_dir_all(parent)
                .map_err(|e| format!("create {}: {e}", parent.display()))?;
        }
        let mut file = std::fs::OpenOptions::new()
            .append(true)
            .create(true)
            .open(&exclude)
            .map_err(|e| format!("open {}: {e}", exclude.display()))?;
        writeln!(file, "{separator}{EXCLUDE_RULE}")
            .map_err(|e| format!("write {}: {e}", exclude.display()))?;
    }
    git(root, &["worktree", "add", "-b", branch, utf8(dir)?, "HEAD"])?;
    if let Err(e) = set_up(root, dir) {
        let _ = git(root, &["worktree", "remove", "--force", utf8(dir)?]);
        let _ = git(root, &["branch", "-D", branch]);
        return Err(e);
    }
    Ok(())
}

/// Give a new worktree what the checkout has but Git does not check out: the
/// gitignored paths that `.worktreeinclude` selects, and submodules.
fn set_up(root: &Path, dir: &Path) -> Result<()> {
    copy_included(root, dir)?;
    if !dir.join(".gitmodules").exists() {
        return Ok(());
    }
    let out = Command::new("git")
        .args(["submodule", "update", "--init", "--recursive"])
        .current_dir(dir)
        .env("GIT_TERMINAL_PROMPT", "0")
        .output()
        .map_err(|e| format!("git submodule update: {e}"))?;
    if !out.status.success() {
        let message = String::from_utf8_lossy(&out.stderr);
        return Err(format!("git submodule update: {}", message.trim()).into());
    }
    Ok(())
}

/// Copy the paths in `root` that match a pattern in its `.worktreeinclude`, in
/// `.gitignore` syntax, and that Git ignores. The copies are APFS clones, so
/// even large directories copy quickly.
fn copy_included(root: &Path, dir: &Path) -> Result<()> {
    if !root.join(".worktreeinclude").exists() {
        return Ok(());
    }
    let matched = git(
        root,
        &[
            "ls-files",
            "--others",
            "--ignored",
            "--exclude-from=.worktreeinclude",
            "--directory",
        ],
    )?;
    if matched.is_empty() {
        return Ok(());
    }
    let mut child = Command::new("git")
        .args(["check-ignore", "--stdin"])
        .current_dir(root)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|e| format!("git check-ignore: {e}"))?;
    let mut stdin = child.stdin.take().ok_or("git check-ignore: no stdin")?;
    let writer = std::thread::spawn(move || stdin.write_all(matched.as_bytes()));
    let out = child
        .wait_with_output()
        .map_err(|e| format!("git check-ignore: {e}"))?;
    let _ = writer.join();
    // check-ignore exits 1 when it matches nothing.
    if out.status.code() == Some(1) {
        return Ok(());
    }
    if !out.status.success() {
        let message = String::from_utf8_lossy(&out.stderr);
        return Err(format!("git check-ignore: {}", message.trim()).into());
    }
    // With --directory, Git lists a directory and also the files in it.
    let mut copied: Vec<&str> = Vec::new();
    let listing = String::from_utf8_lossy(&out.stdout);
    for p in listing.trim().lines() {
        if copied.iter().any(|d| p.starts_with(d)) {
            continue;
        }
        if p.ends_with('/') {
            copied.push(p);
        }
        let p = p.trim_end_matches('/');
        let target = dir.join(p);
        if let Some(parent) = target.parent() {
            std::fs::create_dir_all(parent)
                .map_err(|e| format!("create {}: {e}", parent.display()))?;
        }
        let out = Command::new("cp")
            .args(["-c", "-R"])
            .arg(root.join(p))
            .arg(&target)
            .output()
            .map_err(|e| format!("copying {p}: {e}"))?;
        if !out.status.success() {
            let message = String::from_utf8_lossy(&out.stderr);
            return Err(format!("copying {p}: {}", message.trim()).into());
        }
    }
    Ok(())
}

/// An error if the worktree has uncommitted changes, or its branch or
/// checked-out commit is not merged into HEAD. A worktree or branch that no
/// longer exists passes.
pub fn removable(root: &Path, dir: &Path, branch: &str) -> Result<()> {
    if exists(root, branch) && git(root, &["merge-base", "--is-ancestor", branch, "HEAD"]).is_err()
    {
        return Err(format!("branch {branch} is not merged into HEAD").into());
    }
    if !is_worktree(dir) {
        return Ok(());
    }
    let head = git(dir, &["rev-parse", "HEAD"])?;
    if git(root, &["merge-base", "--is-ancestor", &head, "HEAD"]).is_err() {
        return Err(format!(
            "worktree {} has commits not merged into HEAD",
            dir.display()
        )
        .into());
    }
    if dirty(dir)? {
        return Err(format!("worktree {} has uncommitted changes", dir.display()).into());
    }
    Ok(())
}

/// Whether the checkout in `dir` has uncommitted changes or untracked files.
pub fn dirty(dir: &Path) -> Result<bool> {
    Ok(!git(dir, &["status", "--porcelain"])?.is_empty())
}

/// Remove the worktree and delete its branch if `removable` allows it.
/// `removable` has checked the branch against HEAD, so the deletion ignores
/// the branch's upstream, and has checked the worktree, so `--force` only lets
/// Git remove a worktree with submodules. A worktree or branch that is
/// already gone is skipped.
pub fn remove_worktree(root: &Path, dir: &Path, branch: &str) -> Result<()> {
    removable(root, dir, branch)?;
    if is_worktree(dir) {
        git(root, &["worktree", "remove", "--force", utf8(dir)?])?;
    }
    // Forget a worktree whose directory was deleted, so its branch can go.
    git(root, &["worktree", "prune"])?;
    if !exists(root, branch) {
        return Ok(());
    }
    git(root, &["branch", "-D", branch])?;
    Ok(())
}

fn exists(root: &Path, branch: &str) -> bool {
    let reference = format!("refs/heads/{branch}");
    git(root, &["rev-parse", "--verify", "--quiet", &reference]).is_ok()
}

/// Whether `dir` is the top level of a checkout. A deleted worktree is not,
/// and neither is a plain directory left in its place.
fn is_worktree(dir: &Path) -> bool {
    let Ok(top) = root(dir) else {
        return false;
    };
    let real = |p: &Path| p.canonicalize().unwrap_or_else(|_| p.to_path_buf());
    real(&top) == real(dir)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::TempDir;
    use std::fs;

    /// Run git in `dir` as a fixed author.
    fn run(dir: &Path, args: &[&str]) {
        let out = Command::new("git")
            .args(["-c", "user.name=t", "-c", "user.email=t@t"])
            .args(args)
            .current_dir(dir)
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "git {args:?}: {}{}",
            String::from_utf8_lossy(&out.stdout),
            String::from_utf8_lossy(&out.stderr)
        );
    }

    fn repo() -> (TempDir, PathBuf) {
        let tmp = TempDir::new();
        let root = tmp.path().to_path_buf();
        run(&root, &["init", "-q", "-b", "main"]);
        run(&root, &["commit", "-q", "--allow-empty", "-m", "init"]);
        (tmp, root)
    }

    /// Git refuses local submodule URLs unless file transport is allowed. The
    /// setting reaches the clones that `git submodule` starts only through the
    /// environment. It is set once, and std orders it against process spawns.
    fn allow_file_transport() {
        static ONCE: std::sync::Once = std::sync::Once::new();
        ONCE.call_once(|| unsafe {
            std::env::set_var("GIT_CONFIG_COUNT", "1");
            std::env::set_var("GIT_CONFIG_KEY_0", "protocol.file.allow");
            std::env::set_var("GIT_CONFIG_VALUE_0", "always");
        });
    }

    fn write(path: &Path, data: &str) {
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(path, data).unwrap();
    }

    fn branch_exists(root: &Path, branch: &str) -> bool {
        git(root, &["rev-parse", "--verify", branch]).is_ok()
    }

    #[test]
    fn root_requires_a_checkout() {
        let tmp = TempDir::new();
        assert!(root(tmp.path()).is_err());
        run(tmp.path(), &["init", "-q"]);
        let sub = tmp.path().join("sub");
        fs::create_dir(&sub).unwrap();
        assert_eq!(root(tmp.path()).unwrap(), tmp.path());
        assert_eq!(root(&sub).unwrap(), tmp.path());
    }

    #[test]
    fn branch_is_the_checked_out_branch() {
        let (_tmp, root) = repo();
        assert_eq!(branch(&root).unwrap(), "main");
    }

    #[test]
    fn resolve_directories() {
        let base = TempDir::new();
        let home = TempDir::new();
        let (base, home) = (base.path(), home.path());
        fs::create_dir(base.join("rel")).unwrap();
        fs::create_dir(home.join("src")).unwrap();
        fs::write(base.join("file"), "").unwrap();

        let cases = [
            ("rel", base.join("rel")),
            ("./rel", base.join("rel")),
            (".", base.to_path_buf()),
            ("~/src", home.join("src")),
            (&home.join("src").display().to_string(), home.join("src")),
            (
                &base.join("rel/..").display().to_string(),
                base.to_path_buf(),
            ),
        ];
        for (dir, want) in cases {
            assert_eq!(resolve_in(Some(home), base, dir).unwrap(), want, "{dir}");
        }
        for dir in ["missing", "file", "~/missing"] {
            assert!(resolve_in(Some(home), base, dir).is_err(), "{dir}");
        }
        assert!(resolve_in(None, base, "~/src").is_err());
    }

    #[test]
    fn clean_paths() {
        for (path, want) in [
            ("/a/b/../c", "/a/c"),
            ("/a/./b//c/", "/a/b/c"),
            ("/../a", "/a"),
            ("a/../..", ".."),
            ("a/..", "."),
        ] {
            assert_eq!(clean(Path::new(path)), Path::new(want), "{path}");
        }
    }

    #[test]
    fn short_writes_the_home_directory_as_a_tilde() {
        let home = Path::new("/Users/k");
        for (path, want) in [
            ("/Users/k", "~"),
            ("/Users/k/src/x", "~/src/x"),
            ("/Users/kk/src", "/Users/kk/src"),
            ("/tmp/x", "/tmp/x"),
        ] {
            assert_eq!(short_in(Some(home), Path::new(path)), want, "{path}");
        }
        assert_eq!(short_in(None, Path::new("/Users/k")), "/Users/k");
    }

    #[test]
    fn workspace_names() {
        // The suffix is the first three bytes of the SHA-256 of the path.
        assert_eq!(
            name(Path::new("/Users/kyle/src/hypercube")),
            "hypercube-b046a8"
        );
        assert_eq!(name(Path::new("/tmp/my.repo/x")), "x-537735");
        assert_eq!(name(Path::new("/tmp/my.repo")), "my-repo-1f08f2");
        assert_eq!(name(Path::new("/tmp/a  b!!c")), "a-b-c-db52bd");
        assert_eq!(name(Path::new("/")), "--8a5eda");
    }

    #[test]
    fn sha256_known_answers() {
        let hex =
            |data: &[u8]| -> String { sha256(data).iter().map(|b| format!("{b:02x}")).collect() };
        assert_eq!(
            hex(b""),
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        );
        assert_eq!(
            hex(b"abc"),
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
        );
        assert_eq!(
            hex(b"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"),
            "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"
        );
    }

    #[test]
    fn worker_names() {
        assert_eq!(worker_branch("w1"), "hypercube/w1");
        assert_eq!(
            worktree_dir(Path::new("/r"), "w1"),
            Path::new("/r/.hypercube/w1")
        );
    }

    #[test]
    fn worktree_lifecycle() {
        let (_tmp, root) = repo();
        run(&root, &["branch", "other"]);
        let exclude = root.join(".git/info/exclude");
        fs::write(&exclude, "local-only").unwrap();
        let (dir, branch) = (worktree_dir(&root, "w1"), worker_branch("w1"));
        add_worktree(&root, &dir, &branch).unwrap();
        assert_eq!(
            fs::read_to_string(&exclude).unwrap(),
            "local-only\n/.hypercube/\n"
        );
        assert!(
            !dirty(&root).unwrap(),
            "the worktree makes the checkout dirty"
        );

        run(&dir, &["commit", "-q", "--allow-empty", "-m", "work"]);
        assert!(
            remove_worktree(&root, &dir, &branch).is_err(),
            "unmerged branch"
        );
        run(&root, &["merge", "-q", &branch]);
        fs::write(dir.join("scratch"), "").unwrap();
        assert!(
            remove_worktree(&root, &dir, &branch).is_err(),
            "dirty worktree"
        );
        fs::remove_file(dir.join("scratch")).unwrap();
        run(&dir, &["checkout", "-q", "--detach"]);
        run(&dir, &["commit", "-q", "--allow-empty", "-m", "detached"]);
        assert!(
            remove_worktree(&root, &dir, &branch).is_err(),
            "unmerged detached commit"
        );
        run(&dir, &["checkout", "-q", &branch]);
        // The branch is merged into HEAD but not into its upstream.
        run(&dir, &["branch", "-q", "--set-upstream-to=other"]);
        remove_worktree(&root, &dir, &branch).unwrap();
        assert!(!dir.exists(), "worktree directory remains");
        assert!(!branch_exists(&root, &branch), "branch remains");
    }

    #[test]
    fn removal_errors_say_what_blocks_it() {
        let (_tmp, root) = repo();
        let (dir, branch) = (worktree_dir(&root, "w1"), worker_branch("w1"));
        add_worktree(&root, &dir, &branch).unwrap();
        fs::write(dir.join("scratch"), "").unwrap();
        let err = removable(&root, &dir, &branch).unwrap_err().to_string();
        assert!(err.contains("has uncommitted changes"), "{err}");
        fs::remove_file(dir.join("scratch")).unwrap();
        run(&dir, &["commit", "-q", "--allow-empty", "-m", "work"]);
        let err = removable(&root, &dir, &branch).unwrap_err().to_string();
        assert_eq!(err, "branch hypercube/w1 is not merged into HEAD");
    }

    #[test]
    fn exclude_rule_is_added_once() {
        let (_tmp, root) = repo();
        for name in ["w1", "w2"] {
            add_worktree(&root, &worktree_dir(&root, name), &worker_branch(name)).unwrap();
        }
        let exclude = fs::read_to_string(root.join(".git/info/exclude")).unwrap();
        assert_eq!(exclude.matches("/.hypercube/").count(), 1, "{exclude}");
    }

    #[test]
    fn worktree_set_up_copies_included_paths_and_checks_out_submodules() {
        let (_sub_tmp, sub) = repo();
        write(&sub.join("lib"), "lib");
        run(&sub, &["add", "lib"]);
        run(&sub, &["commit", "-q", "-m", "lib"]);

        let (_tmp, root) = repo();
        allow_file_transport();
        run(
            &root,
            &[
                "submodule",
                "-q",
                "add",
                sub.to_str().unwrap(),
                "vendor/sub",
            ],
        );
        let files = [
            (".gitignore", ".toolchain/\n.env\ncache/\nbuild/\n"),
            (".worktreeinclude", ".toolchain/\n.env\ncache/keep\nsrc/\n"),
            (".toolchain/bin/zig", "zig"),
            (".env", "env"),
            ("cache/keep", "keep"),
            ("cache/drop", "drop"),
            ("build/out", "out"),
            ("src/untracked.go", "not ignored"),
        ];
        for (name, data) in files {
            write(&root.join(name), data);
        }
        run(&root, &["add", ".gitignore", ".worktreeinclude"]);
        run(&root, &["commit", "-q", "-m", "setup"]);

        let (dir, branch) = (worktree_dir(&root, "w1"), worker_branch("w1"));
        add_worktree(&root, &dir, &branch).unwrap();
        for name in [".toolchain/bin/zig", ".env", "cache/keep", "vendor/sub/lib"] {
            let data = fs::read_to_string(dir.join(name))
                .unwrap_or_else(|e| panic!("{name} missing from the worktree: {e}"));
            if let Some((_, want)) = files.iter().find(|(n, _)| *n == name) {
                assert_eq!(&data, want, "{name}");
            }
        }
        for name in ["cache/drop", "build/out", "src/untracked.go"] {
            assert!(
                !dir.join(name).exists(),
                "{name} was copied into the worktree"
            );
        }
        assert!(!dirty(&dir).unwrap(), "new worktree is dirty");

        write(&dir.join("vendor/sub/lib"), "changed");
        assert!(
            remove_worktree(&root, &dir, &branch).is_err(),
            "removed a worktree with a changed submodule"
        );
        run(&dir.join("vendor/sub"), &["checkout", "-q", "lib"]);
        remove_worktree(&root, &dir, &branch).unwrap();
        assert!(!dir.exists(), "worktree directory remains");
    }

    #[test]
    fn failed_set_up_removes_the_worktree_and_branch() {
        let (_tmp, root) = repo();
        // A submodule whose URL cannot be fetched fails the set up.
        write(
            &root.join(".gitmodules"),
            "[submodule \"x\"]\n\tpath = x\n\turl = /nonexistent/hypercube-test\n",
        );
        run(&root, &["add", ".gitmodules"]);
        run(
            &root,
            &[
                "update-index",
                "--add",
                "--cacheinfo",
                "160000,0123456789012345678901234567890123456789,x",
            ],
        );
        run(&root, &["commit", "-q", "-m", "submodule"]);
        let (dir, branch) = (worktree_dir(&root, "w1"), worker_branch("w1"));
        let err = add_worktree(&root, &dir, &branch).unwrap_err().to_string();
        assert!(err.contains("git submodule update"), "{err}");
        assert!(!dir.exists(), "worktree directory remains");
        assert!(!branch_exists(&root, &branch), "branch remains");
    }

    #[test]
    fn removing_missing_worktrees_and_branches() {
        let (_tmp, root) = repo();
        let (dir, branch) = (worktree_dir(&root, "w1"), worker_branch("w1"));
        add_worktree(&root, &dir, &branch).unwrap();
        fs::remove_dir_all(&dir).unwrap();
        remove_worktree(&root, &dir, &branch).unwrap();
        assert!(!branch_exists(&root, &branch), "branch remains");

        add_worktree(&root, &dir, &branch).unwrap();
        run(&root, &["worktree", "remove", dir.to_str().unwrap()]);
        run(&root, &["branch", "-D", &branch]);
        remove_worktree(&root, &dir, &branch).unwrap();

        run(&root, &["branch", &branch]);
        run(&root, &["checkout", "-q", &branch]);
        run(&root, &["commit", "-q", "--allow-empty", "-m", "unmerged"]);
        run(&root, &["checkout", "-q", "main"]);
        assert!(
            remove_worktree(&root, &dir, &branch).is_err(),
            "removed an unmerged branch without a worktree"
        );
    }

    #[test]
    fn a_plain_directory_in_a_worktrees_place_is_left_alone() {
        let (_tmp, root) = repo();
        let (dir, branch) = (worktree_dir(&root, "w1"), worker_branch("w1"));
        write(&dir.join("keep"), "keep");
        remove_worktree(&root, &dir, &branch).unwrap();
        assert!(dir.join("keep").exists());
    }
}
