//! Helpers shared by the stand-in user terminals in the test suites.
// Each suite uses a different subset of these helpers.
#![allow(dead_code)]

use hypercube::mux::Winsize;
use libghostty_vt::render::{CellIterator, RowIterator};
use libghostty_vt::screen::CellWide;
use libghostty_vt::{RenderState, Terminal};

pub fn contains(haystack: &[u8], needle: &[u8]) -> bool {
    haystack.windows(needle.len()).any(|w| w == needle)
}

/// What the stand-in user's terminal answers to the startup queries.
pub fn host_replies() -> Vec<u8> {
    let mut s = "\x1b]10;rgb:d0d0/d1d1/d2d2\x1b\\\x1b]11;rgb:1010/1111/1212\x1b\\".to_owned();
    for i in 0..16 {
        s.push_str(&format!(
            "\x1b]4;{i};rgb:{:02x}/{:02x}/{:02x}\x1b\\",
            i * 16 + 1,
            255 - i * 15,
            i * 7 + 3
        ));
    }
    s.push_str("\x1b[?2027;1$y\x1b[?62;22c");
    s.into_bytes()
}

pub fn winsize(cols: u16, rows: u16) -> Winsize {
    Winsize {
        ws_col: cols,
        ws_row: rows,
        ws_xpixel: cols * 8,
        ws_ypixel: rows * 16,
    }
}

pub fn screen(terminal: &Terminal) -> String {
    let mut state = RenderState::new().unwrap();
    let snapshot = state.update(terminal).unwrap();
    let mut rows = RowIterator::new().unwrap();
    let mut rows = rows.update(&snapshot).unwrap();
    let mut cells = CellIterator::new().unwrap();
    let mut text = String::new();
    while let Some(row) = rows.next() {
        let mut line = String::new();
        let mut iter = cells.update(row).unwrap();
        while let Some(cell) = iter.next() {
            let raw = cell.raw_cell().unwrap();
            if raw.wide().unwrap() == CellWide::SpacerTail {
                continue;
            }
            if raw.has_text().unwrap() {
                let mut grapheme = String::new();
                cell.graphemes_utf8(&mut grapheme).unwrap();
                line.push_str(&grapheme);
            } else {
                line.push(' ');
            }
        }
        text.push_str(line.trim_end_matches(' '));
        text.push('\n');
    }
    text.trim_end_matches('\n').to_owned()
}
