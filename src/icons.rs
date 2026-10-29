//! The Nerd Font icons the chrome, the bar, the overlay, and the team list
//! draw.

/// Before a worker's name while it is expanded (nf-oct-chevron_down).
pub const EXPANDED: char = '\u{f47c}';
/// Before a worker's name while it is collapsed (nf-oct-chevron_right).
pub const COLLAPSED: char = '\u{f460}';
/// Before a branch (nf-dev-git_branch).
pub const BRANCH: char = '\u{e725}';
/// Before a directory (nf-fa-folder).
pub const DIRECTORY: char = '\u{f07b}';
/// Before the workspace's path in the header (nf-fa-folder_open).
pub const HEADER: char = '\u{f07c}';
/// After an agent or workspace waiting for a tool approval (nf-fa-bell).
pub const ATTENTION: char = '\u{f0f3}';
/// The note on a zoomed pane (nf-fa-expand).
pub const ZOOMED: char = '\u{f065}';
/// Before the exit status of a pane's program (nf-fa-times_circle).
pub const EXITED: char = '\u{f057}';
/// Before the error of an agent whose harness failed to start
/// (nf-fa-warning).
pub const FAILED: char = '\u{f071}';
/// Starts a running workspace's row in the overlay, or a running team's in
/// the team list (nf-fa-play).
pub const RUNNING: char = '\u{f04b}';
/// Starts a stopped workspace's row in the overlay (nf-fa-stop).
pub const STOPPED: char = '\u{f04d}';
/// Starts an idle team's row in the team list (nf-fa-pause).
pub const IDLE: char = '\u{f04c}';
/// Before the scheduled tasks' counts in the header and the overlay
/// (nf-fa-clock_o).
pub const TASKS: char = '\u{f017}';
