//! The splash: what the client draws while no workspace is running. The
//! tesseract turns across the screen, and the last row holds a hint, or the
//! bar's prompt for the path of a workspace to start.

use crate::bar::{Bar, Typed};
use crate::colors::{self, Palette};
use crate::host::fit;
use crate::mux::{Winsize, token_end};
use crate::tesseract::{Canvas, SHADES, Tesseract};

/// The time between frames.
pub const FRAME_MS: i64 = 40;
/// The accent of the splash's colors.
const ACCENT: usize = 0;
/// What the last row says while it is not prompting.
const HINT: &str = " Ctrl-b c new workspace · Ctrl-b d exit";
const CTRL_B: u8 = 0x02;
const CTRL_C: u8 = 0x03;

/// How dark the nearest and farthest shades of the tesseract are, as shares
/// of the way from the accent to black.
const NEAREST_SHADE: f32 = 0.25;
const FARTHEST_SHADE: f32 = 0.6;

/// What the input asks the client to do.
#[derive(Debug, PartialEq)]
pub enum Outcome {
    Stay,
    Quit,
    /// Enter closed the prompt on this path.
    Open(String),
}

pub struct Splash {
    /// The working directory, which the prompt starts with.
    cwd: String,
    shape: Tesseract,
    bar: Bar,
    palette: Palette,
    /// Whether the key after Ctrl-b is next.
    prefixed: bool,
    /// The columns and rows drawn, and the tesseract's cells drawn last;
    /// `None` while the screen may hold anything else in their place.
    screen: Option<(u16, u16)>,
    drawn: Option<Canvas>,
}

impl Splash {
    pub fn new(cwd: &str) -> Splash {
        Splash {
            cwd: cwd.to_string(),
            shape: Tesseract::new(),
            bar: Bar::default(),
            palette: Palette::of(ACCENT),
            prefixed: false,
            screen: None,
            drawn: None,
        }
    }

    /// Show `error` in the last row for a while.
    pub fn fail(&mut self, error: String) {
        self.bar.fail(error);
    }

    /// Take the terminal's input. Ctrl-b c prompts for a path, starting with
    /// the working directory, Ctrl-b d and Ctrl-c quit, and while the prompt
    /// is open the other keys type into it.
    pub fn input(&mut self, data: &[u8]) -> Outcome {
        let mut i = 0;
        while i < data.len() {
            let end = token_end(data, i, true).unwrap_or(data.len());
            let token = &data[i..end];
            i = end;
            if token == [CTRL_C] {
                return Outcome::Quit;
            }
            if std::mem::take(&mut self.prefixed) {
                match token {
                    b"c" => self.bar.prompt = Some(self.cwd.clone()),
                    b"d" => return Outcome::Quit,
                    _ => {}
                }
            } else if token == [CTRL_B] {
                self.prefixed = true;
            } else if let Typed::Enter(path) = self.bar.key(token) {
                return Outcome::Open(path);
            }
        }
        Outcome::Stay
    }

    /// Write what changed on the screen of `size` at `now` ms into the
    /// turn: the tesseract's cells that differ from the last frame, and the
    /// last row.
    pub fn draw(&mut self, out: &mut Vec<u8>, size: Winsize, now: i64) {
        self.bar.update(Vec::new());
        let (cols, rows) = (size.ws_col, size.ws_row);
        if self.screen != Some((cols, rows)) {
            self.screen = Some((cols, rows));
            self.drawn = None;
            out.extend_from_slice(b"\x1b[0m\x1b[2J\x1b[?25l");
        }
        let mut s = String::new();
        if rows > 1 {
            self.draw_cube(&mut s, cols, rows - 1, now);
        }
        if self.bar.idle() {
            s.push_str(&format!(
                "\x1b[{rows};1H{}{}\x1b[{rows};1H{}",
                self.palette.bar.sgr(),
                " ".repeat(usize::from(cols)),
                fit(HINT, cols)
            ));
        } else {
            self.bar.draw(&mut s, size, self.palette);
        }
        s.push_str("\x1b[0m");
        out.extend_from_slice(s.as_bytes());
    }

    /// Draw the cells of the tesseract on `cols` by `rows` from the top left
    /// that differ from the last frame.
    fn draw_cube(&mut self, s: &mut String, cols: u16, rows: u16, now: i64) {
        let canvas = self.shape.draw(cols, rows, now as f64 / 1000.0);
        let last = self.drawn.take();
        let mut shade = None;
        for y in 0..rows {
            // The cursor is already at the cell after one just drawn.
            let mut next = None;
            for x in 0..cols {
                let cell = canvas.cell(x, y);
                if last.as_ref().is_some_and(|last| last.cell(x, y) == cell) {
                    continue;
                }
                if next != Some(x) {
                    s.push_str(&format!("\x1b[{};{}H", y + 1, x + 1));
                }
                next = Some(x + 1);
                if cell.dots != 0 && shade != Some(cell.shade) {
                    shade = Some(cell.shade);
                    let depth = f32::from(cell.shade) / f32::from(SHADES - 1);
                    let rgb = colors::mix(
                        self.palette.line,
                        colors::BLACK,
                        NEAREST_SHADE + depth * (FARTHEST_SHADE - NEAREST_SHADE),
                    );
                    s.push_str(&colors::fg(rgb));
                }
                s.push(cell.glyph());
            }
        }
        self.drawn = Some(canvas);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use libghostty_vt::Terminal;
    use libghostty_vt::terminal::{Point, PointCoordinate};

    fn size(cols: u16, rows: u16) -> Winsize {
        Winsize {
            ws_col: cols,
            ws_row: rows,
            ws_xpixel: 0,
            ws_ypixel: 0,
        }
    }

    /// The screen of `cols` by `rows` after `frames` drawn in order.
    fn rows_of(frames: &[Vec<u8>], cols: u16, rows: u16) -> Vec<String> {
        let mut terminal = Terminal::new(cols, rows).unwrap();
        for frame in frames {
            terminal.vt_write(frame);
        }
        (0..rows)
            .map(|y| {
                (0..cols)
                    .map(|x| {
                        let point = Point::Active(PointCoordinate { x, y: y.into() });
                        let mut buf = ['\0'; 4];
                        match terminal
                            .grid_ref(point)
                            .unwrap()
                            .graphemes(&mut buf)
                            .unwrap()
                        {
                            0 => ' ',
                            _ => buf[0],
                        }
                    })
                    .collect()
            })
            .collect()
    }

    fn frame(splash: &mut Splash, cols: u16, rows: u16, now: i64) -> Vec<u8> {
        let mut out = Vec::new();
        splash.draw(&mut out, size(cols, rows), now);
        out
    }

    fn braille(c: char) -> bool {
        ('\u{2800}'..='\u{28ff}').contains(&c)
    }

    #[test]
    fn the_tesseract_fills_the_screen_above_the_hint() {
        let mut splash = Splash::new("/work");
        let rows = rows_of(&[frame(&mut splash, 60, 20, 3000)], 60, 20);
        let drawn = |row: &String| row.chars().filter(|&c| braille(c)).count();
        assert!(rows[..19].iter().map(drawn).sum::<usize>() > 40);
        assert_eq!(drawn(&rows[19]), 0);
        assert!(rows[19].starts_with(" Ctrl-b c new workspace"));
    }

    #[test]
    fn a_frame_draws_only_the_cells_that_changed() {
        let mut splash = Splash::new("/work");
        let first = frame(&mut splash, 60, 20, 3000);
        let again = frame(&mut splash, 60, 20, 3000);
        let next = frame(&mut splash, 60, 20, 3040);
        assert!(next.len() < first.len());
        assert!(again.len() < next.len());
        // The turn of the tesseract goes on over the first frame.
        let rows = rows_of(&[first, again, next], 60, 20);
        assert!(rows[0].chars().chain(rows[9].chars()).any(braille));
    }

    #[test]
    fn a_new_size_draws_everything_again() {
        let mut splash = Splash::new("/work");
        frame(&mut splash, 60, 20, 3000);
        let resized = frame(&mut splash, 40, 12, 3040);
        assert!(resized.starts_with(b"\x1b[0m\x1b[2J"));
        let rows = rows_of(&[resized], 40, 12);
        assert!(rows[..11].iter().any(|r| r.chars().any(braille)));
    }

    #[test]
    fn ctrl_b_c_opens_the_prompt_with_the_working_directory() {
        let mut splash = Splash::new("/work");
        assert_eq!(splash.input(b"\x02c"), Outcome::Stay);
        let rows = rows_of(&[frame(&mut splash, 60, 20, 3000)], 60, 20);
        assert!(
            rows[19].starts_with(" workspace path: /work"),
            "{}",
            rows[19]
        );
    }

    #[test]
    fn the_prompt_edits_the_path_and_enter_opens_it() {
        let mut splash = Splash::new("/work");
        // Keys typed one at a time, or in one read.
        assert_eq!(splash.input(b"\x02"), Outcome::Stay);
        assert_eq!(splash.input(b"c"), Outcome::Stay);
        assert_eq!(splash.input(b"\x7f\x7frx"), Outcome::Stay);
        assert_eq!(splash.input(b"\r"), Outcome::Open("/worx".into()));
        // Enter closed the prompt.
        assert_eq!(splash.input(b"\r"), Outcome::Stay);
        let rows = rows_of(&[frame(&mut splash, 60, 20, 3000)], 60, 20);
        assert!(rows[19].starts_with(" Ctrl-b c new workspace"));
    }

    #[test]
    fn escape_closes_the_prompt() {
        let mut splash = Splash::new("/work");
        splash.input(b"\x02c");
        assert_eq!(splash.input(b"\x1b"), Outcome::Stay);
        let rows = rows_of(&[frame(&mut splash, 60, 20, 3000)], 60, 20);
        assert!(rows[19].starts_with(" Ctrl-b c new workspace"));
        // Typing without a prompt does nothing.
        assert_eq!(splash.input(b"x\r"), Outcome::Stay);
    }

    #[test]
    fn ctrl_b_d_and_ctrl_c_quit() {
        let mut splash = Splash::new("/work");
        assert_eq!(splash.input(b"\x02d"), Outcome::Quit);
        assert_eq!(splash.input(b"\x03"), Outcome::Quit);
        splash.input(b"\x02c");
        assert_eq!(splash.input(b"\x03"), Outcome::Quit);
        let mut splash = Splash::new("/work");
        splash.input(b"\x02c");
        assert_eq!(splash.input(b"\x02d"), Outcome::Quit);
    }

    #[test]
    fn an_error_replaces_the_hint() {
        let mut splash = Splash::new("/work");
        splash.fail("no such directory".into());
        let rows = rows_of(&[frame(&mut splash, 60, 20, 3000)], 60, 20);
        assert!(rows[19].starts_with(" no such directory"), "{}", rows[19]);
    }
}
