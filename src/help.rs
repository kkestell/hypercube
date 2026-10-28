//! The list of keys, which Ctrl-b ? draws over the whole screen.

use crate::colors::Palette;
use crate::host::fit;
use crate::mux::Winsize;
use crate::overlay::{HELP, pad};

/// Each key and what it does.
const KEYS: &[(&str, &str)] = &[
    ("Ctrl-b o", "focus the next pane"),
    ("Ctrl-b z", "zoom the focused pane, or restore the layout"),
    ("Ctrl-b [", "scroll the focused pane up a page"),
    ("Ctrl-b s", "open the workspace overlay"),
    ("Ctrl-b 0-9", "switch to the workspace with that number"),
    ("Ctrl-b ( )", "switch to the previous or next workspace"),
    ("Ctrl-b c", "switch to the workspace at a path"),
    ("Ctrl-b d", "detach"),
    ("Ctrl-b a", "change this workspace's color"),
    ("Ctrl-b Ctrl-b", "send Ctrl-b to the focused pane"),
    ("Ctrl-b ?", "show these keys"),
    ("Shift-PageUp", "scroll the focused pane up a page"),
    ("", ""),
    ("While scrolled", ""),
    ("PageUp PageDown ↑ ↓", "scroll"),
    ("Esc q", "return to the bottom"),
];

/// Draw the list over the whole screen: a title row, a row for each key,
/// and how to close it in the last row.
pub fn draw(out: &mut Vec<u8>, size: Winsize, palette: Palette) {
    let (cols, rows) = (size.ws_col, usize::from(size.ws_row));
    if rows == 0 {
        return;
    }
    let mut s = format!(
        "\x1b[0m\x1b[2J\x1b[1;1H{}{}\x1b[0m",
        palette.bar.sgr(),
        pad(" Keys", cols)
    );
    let width = KEYS.iter().map(|(key, _)| key.chars().count()).max();
    let width = width.unwrap_or(0);
    for (i, (key, does)) in KEYS.iter().enumerate().take(rows.saturating_sub(2)) {
        let gap = " ".repeat(width - key.chars().count());
        let text = fit(&format!(" {key}{gap}  {does}"), cols);
        s.push_str(&format!("\x1b[{};1H{text}", i + 2));
    }
    if rows > 1 {
        let text = fit(" any key closes", cols);
        s.push_str(&format!("\x1b[{rows};1H{HELP}{text}\x1b[0m"));
    }
    out.extend_from_slice(s.as_bytes());
}
