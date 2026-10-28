//! Signals delivered through pipes, so that a signal arriving while an event
//! loop is busy still wakes its next poll. A flag checked before polling
//! would miss a signal that lands between the check and the poll.

use std::io;
use std::os::fd::{FromRawFd, IntoRawFd, OwnedFd, RawFd};
use std::sync::Mutex;
use std::sync::atomic::{AtomicI32, Ordering};

/// A pipe, read end first, with both ends close-on-exec and non-blocking.
pub fn pipe() -> io::Result<(OwnedFd, OwnedFd)> {
    let mut fds = [0; 2];
    if unsafe { libc::pipe(fds.as_mut_ptr()) } != 0 {
        return Err(io::Error::last_os_error());
    }
    for fd in fds {
        unsafe {
            libc::fcntl(
                fd,
                libc::F_SETFL,
                libc::fcntl(fd, libc::F_GETFL) | libc::O_NONBLOCK,
            );
            libc::fcntl(fd, libc::F_SETFD, libc::FD_CLOEXEC);
        }
    }
    Ok(unsafe { (OwnedFd::from_raw_fd(fds[0]), OwnedFd::from_raw_fd(fds[1])) })
}

/// Install `handler` for each of `signals`, and return the read end of a
/// pipe whose write end is stored in `write_end` for the handler to `wake`.
/// The pipe lasts as long as the process.
pub fn catch(
    signals: &[libc::c_int],
    handler: extern "C" fn(libc::c_int),
    write_end: &AtomicI32,
) -> io::Result<RawFd> {
    let (read, write) = pipe()?;
    write_end.store(write.into_raw_fd(), Ordering::Relaxed);
    for &signal in signals {
        unsafe {
            let mut action: libc::sigaction = std::mem::zeroed();
            action.sa_sigaction = handler as libc::sighandler_t;
            libc::sigemptyset(&mut action.sa_mask);
            libc::sigaction(signal, &action, std::ptr::null_mut());
        }
    }
    Ok(read.into_raw_fd())
}

/// Make the pipe whose write end is in `write_end` readable. Safe in a
/// signal handler: it only writes, and keeps `errno`.
pub fn wake(write_end: &AtomicI32) {
    let fd = write_end.load(Ordering::Relaxed);
    if fd < 0 {
        return;
    }
    unsafe {
        let saved = *libc::__error();
        let byte = 1u8;
        libc::write(fd, (&raw const byte).cast(), 1);
        *libc::__error() = saved;
    }
}

static WINCH: AtomicI32 = AtomicI32::new(-1);

extern "C" fn on_sigwinch(_: libc::c_int) {
    wake(&WINCH);
}

/// Catch SIGWINCH once per process and return the pipe's read end, which
/// becomes readable after each SIGWINCH.
pub fn winch() -> io::Result<RawFd> {
    static READ_END: Mutex<Option<RawFd>> = Mutex::new(None);
    let mut read_end = READ_END.lock().unwrap_or_else(|e| e.into_inner());
    if let Some(fd) = *read_end {
        return Ok(fd);
    }
    let fd = catch(&[libc::SIGWINCH], on_sigwinch, &WINCH)?;
    *read_end = Some(fd);
    Ok(fd)
}

/// Whether the pipe at `read_end` was woken since the last call. Empties it.
pub fn take(read_end: RawFd) -> bool {
    let mut buf = [0u8; 64];
    let mut any = false;
    while unsafe { libc::read(read_end, buf.as_mut_ptr().cast(), buf.len()) } > 0 {
        any = true;
    }
    any
}
