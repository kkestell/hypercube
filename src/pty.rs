//! Programs running in pseudo-terminals.

use std::ffi::OsStr;
use std::io;
use std::os::fd::{FromRawFd as _, OwnedFd};
use std::os::unix::process::CommandExt as _;
use std::path::Path;
use std::process::{Child, Command, Stdio};
use std::sync::{Mutex, MutexGuard};

use crate::mux::Winsize;

/// openpty names the pty with ptsname, which returns a shared static buffer,
/// so ptys are opened one at a time. Holding the lock until the child has
/// started also keeps a child spawned on another thread from inheriting a
/// pty before it is marked close-on-exec.
static LOCK: Mutex<()> = Mutex::new(());

fn lock() -> MutexGuard<'static, ()> {
    LOCK.lock().unwrap_or_else(|e| e.into_inner())
}

/// A program in its own session with a pty as its controlling terminal.
pub struct Pty {
    pub master: OwnedFd,
    pub child: Child,
}

impl Pty {
    /// Start `argv` in `cwd` with `env` added to the environment, on a pty of
    /// `size`. `TERM` is `xterm-256color` and `COLORTERM` is `truecolor`. The
    /// master is close-on-exec, so later children don't inherit it.
    pub fn spawn(
        argv: &[impl AsRef<OsStr>],
        cwd: &Path,
        env: &[(impl AsRef<OsStr>, impl AsRef<OsStr>)],
        size: Winsize,
    ) -> io::Result<Pty> {
        let Some(program) = argv.first() else {
            return Err(io::Error::new(io::ErrorKind::InvalidInput, "empty command"));
        };
        let _guard = lock();
        let (master, slave) = open(size)?;
        let mut command = Command::new(program);
        command
            .args(&argv[1..])
            .current_dir(cwd)
            .envs(env.iter().map(|(k, v)| (k, v)))
            .env("TERM", "xterm-256color")
            .env("COLORTERM", "truecolor")
            .stdin(Stdio::from(slave.try_clone()?))
            .stdout(Stdio::from(slave.try_clone()?))
            .stderr(Stdio::from(slave));
        unsafe {
            command.pre_exec(|| {
                if libc::setsid() < 0 || libc::ioctl(0, libc::TIOCSCTTY.into(), 0) < 0 {
                    return Err(io::Error::last_os_error());
                }
                Ok(())
            });
        }
        let child = command.spawn().map_err(|e| {
            let program = Path::new(program).display();
            io::Error::new(e.kind(), format!("{program}: {e}"))
        });
        // The command holds the slave; the parent keeps only the master, so
        // reading it ends once the program's side closes.
        drop(command);
        Ok(Pty {
            master,
            child: child?,
        })
    }
}

/// Open a pty of `size`, returning its master and slave, both close-on-exec.
pub fn openpty(size: Winsize) -> io::Result<(OwnedFd, OwnedFd)> {
    let _guard = lock();
    open(size)
}

fn open(size: Winsize) -> io::Result<(OwnedFd, OwnedFd)> {
    let (mut master, mut slave) = (-1, -1);
    let mut ws = size;
    let result = unsafe {
        libc::openpty(
            &mut master,
            &mut slave,
            std::ptr::null_mut(),
            std::ptr::null_mut(),
            &mut ws,
        )
    };
    if result != 0 {
        return Err(io::Error::last_os_error());
    }
    let fds = unsafe { (OwnedFd::from_raw_fd(master), OwnedFd::from_raw_fd(slave)) };
    for fd in [master, slave] {
        unsafe { libc::fcntl(fd, libc::F_SETFD, libc::FD_CLOEXEC) };
    }
    Ok(fds)
}
