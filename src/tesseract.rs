//! The tesseract the splash draws: a wireframe four-dimensional
//! cube turning in two planes, projected down to the screen with perspective
//! and drawn with braille dots, two across and four down in each cell.

use std::f64::consts::TAU;

/// How many levels of depth a cell is shaded in, from 0, the nearest.
pub const SHADES: u8 = 12;

/// Dots across and down a cell.
const DOTS_WIDE: i32 = 2;
const DOTS_HIGH: i32 = 4;

/// The bit of the dot at each row and column of a cell in a braille character.
const DOT_BITS: [[u8; 2]; 4] = [[0x01, 0x08], [0x02, 0x10], [0x04, 0x20], [0x40, 0x80]];

/// How far the camera is from the cube along the fourth axis, then along the
/// third, with the cube's corners at one from the center.
const CAMERA_4D: f32 = 4.5;
const CAMERA_3D: f32 = 6.0;
/// The turns of the cube in the XW and YZ planes, in radians per second.
const XW_RATE: f64 = 0.5;
const YZ_RATE: f64 = 0.31;
/// The share of the smaller side the cube spans when it is at its widest.
const FILL: f32 = 0.95;
/// The steps in each plane of the sweep that measures the cube's reach.
const SWEEP: u32 = 48;

/// A braille cell: the dots that are set, and the shade of the nearest.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Cell {
    pub dots: u8,
    /// From 0, the nearest, to `SHADES - 1`. 0 in a blank cell.
    pub shade: u8,
}

impl Cell {
    /// The braille character of the dots, or a space if none are set.
    pub fn glyph(self) -> char {
        match self.dots {
            0 => ' ',
            dots => char::from_u32(0x2800 + u32::from(dots)).unwrap_or(' '),
        }
    }
}

/// A point on the dot grid, from the top left, with its depth from 0, the
/// nearest, to 1.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Point {
    pub x: f32,
    pub y: f32,
    pub depth: f32,
}

/// A grid of braille cells that lines are drawn on.
pub struct Canvas {
    cols: u16,
    rows: u16,
    cells: Vec<Cell>,
}

impl Canvas {
    /// A blank canvas of `cols` by `rows` cells.
    pub fn new(cols: u16, rows: u16) -> Canvas {
        Canvas {
            cols,
            rows,
            cells: vec![Cell::default(); usize::from(cols) * usize::from(rows)],
        }
    }

    pub fn cols(&self) -> u16 {
        self.cols
    }

    pub fn rows(&self) -> u16 {
        self.rows
    }

    /// The cell at `col` and `row`, from 0.
    pub fn cell(&self, col: u16, row: u16) -> Cell {
        self.cells[usize::from(row) * usize::from(self.cols) + usize::from(col)]
    }

    /// Set the dot at `x` and `y`, which does nothing off the canvas. A cell
    /// takes the shade of its nearest dot.
    pub fn set(&mut self, x: i32, y: i32, depth: f32) {
        if x < 0 || y < 0 || x >= i32::from(self.cols) * DOTS_WIDE {
            return;
        }
        if y >= i32::from(self.rows) * DOTS_HIGH {
            return;
        }
        let shade = (depth.clamp(0.0, 1.0) * f32::from(SHADES - 1)).round() as u8;
        let index = (y / DOTS_HIGH) as usize * usize::from(self.cols) + (x / DOTS_WIDE) as usize;
        let cell = &mut self.cells[index];
        cell.shade = if cell.dots == 0 {
            shade
        } else {
            cell.shade.min(shade)
        };
        cell.dots |= DOT_BITS[(y % DOTS_HIGH) as usize][(x % DOTS_WIDE) as usize];
    }

    /// Draw a line between two points, its depth running from one end's to
    /// the other's.
    pub fn line(&mut self, from: Point, to: Point) {
        let (dx, dy) = (to.x - from.x, to.y - from.y);
        let steps = dx.abs().max(dy.abs()).ceil() as i32;
        for i in 0..=steps {
            let t = if steps == 0 {
                0.0
            } else {
                i as f32 / steps as f32
            };
            self.set(
                (from.x + dx * t).round() as i32,
                (from.y + dy * t).round() as i32,
                from.depth + (to.depth - from.depth) * t,
            );
        }
    }
}

/// A tesseract and how far its projection reaches, so that it is drawn at a
/// size that doesn't change as it turns.
pub struct Tesseract {
    /// The farthest the projection gets from the center along either screen
    /// axis, and from the camera plane along the third.
    reach: f32,
    depth: f32,
}

impl Default for Tesseract {
    fn default() -> Tesseract {
        Tesseract::new()
    }
}

impl Tesseract {
    /// A tesseract, measured over every angle of its two planes.
    pub fn new() -> Tesseract {
        let mut tesseract = Tesseract {
            reach: 0.0,
            depth: 0.0,
        };
        for a in 0..SWEEP {
            for b in 0..SWEEP {
                let xw = (TAU * f64::from(a) / f64::from(SWEEP)) as f32;
                let yz = (TAU * f64::from(b) / f64::from(SWEEP)) as f32;
                for corner in 0..16 {
                    let (x, y, z) = project(corner, xw, yz);
                    tesseract.reach = tesseract.reach.max(x.abs()).max(y.abs());
                    tesseract.depth = tesseract.depth.max(z.abs());
                }
            }
        }
        tesseract
    }

    /// The corners on a dot grid `width` by `height`, `seconds` into the
    /// turn, the cube centered and at most `FILL` of the smaller side, with
    /// the first dot at 0 and the last at one less than the size. Dots
    /// are square on a terminal, whose cells are twice as tall as wide.
    pub fn corners(&self, seconds: f64, width: f32, height: f32) -> [Point; 16] {
        let xw = (seconds * XW_RATE).rem_euclid(TAU) as f32;
        let yz = (seconds * YZ_RATE).rem_euclid(TAU) as f32;
        // Dots are drawn at their centers, from 0 to one less than the size.
        let (right, bottom) = (width - 1.0, height - 1.0);
        let scale = FILL * right.min(bottom) / 2.0 / self.reach;
        std::array::from_fn(|corner| {
            let (x, y, z) = project(corner, xw, yz);
            Point {
                x: right / 2.0 + x * scale,
                y: bottom / 2.0 - y * scale,
                depth: 0.5 - z / (2.0 * self.depth),
            }
        })
    }

    /// The tesseract's 32 edges on a canvas of `cols` by `rows` cells,
    /// `seconds` into the turn.
    pub fn draw(&self, cols: u16, rows: u16, seconds: f64) -> Canvas {
        let mut canvas = Canvas::new(cols, rows);
        let width = f32::from(cols) * DOTS_WIDE as f32;
        let height = f32::from(rows) * DOTS_HIGH as f32;
        let corners = self.corners(seconds, width, height);
        for a in 0..16 {
            for axis in 0..4 {
                let b = a ^ (1 << axis);
                if a < b {
                    canvas.line(corners[a], corners[b]);
                }
            }
        }
        canvas
    }
}

/// Corner `corner` of the tesseract, whose bits are the signs of its four
/// coordinates, turned `xw` in the XW plane and `yz` in the YZ plane, then
/// projected with perspective along W and along Z onto the screen plane. The
/// result is the screen x and y, and the depth along Z, larger when nearer.
fn project(corner: usize, xw: f32, yz: f32) -> (f32, f32, f32) {
    let sign = |bit: usize| if corner >> bit & 1 == 1 { 1.0 } else { -1.0 };
    let (mut x, mut y, mut z, mut w) = (sign(0), sign(1), sign(2), sign(3));
    (x, w) = turn(x, w, xw);
    (y, z) = turn(y, z, yz);
    let near = CAMERA_4D / (CAMERA_4D - w);
    let (x, y, z) = (x * near, y * near, z * near);
    let near = CAMERA_3D / (CAMERA_3D - z);
    (x * near, y * near, z)
}

/// The point (`a`, `b`) turned `angle` radians.
fn turn(a: f32, b: f32, angle: f32) -> (f32, f32) {
    let (sin, cos) = angle.sin_cos();
    (a * cos - b * sin, a * sin + b * cos)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn point(x: f32, y: f32, depth: f32) -> Point {
        Point { x, y, depth }
    }

    #[test]
    fn dots_set_the_bits_of_their_braille_character() {
        let mut canvas = Canvas::new(2, 1);
        canvas.set(0, 0, 0.0);
        assert_eq!(canvas.cell(0, 0).glyph(), '\u{2801}');
        canvas.set(1, 3, 0.0);
        assert_eq!(canvas.cell(0, 0).glyph(), '\u{2881}');
        for y in 0..4 {
            for x in 2..4 {
                canvas.set(x, y, 0.0);
            }
        }
        assert_eq!(canvas.cell(1, 0).glyph(), '\u{28ff}');
        assert_eq!(Cell::default().glyph(), ' ');
    }

    #[test]
    fn dots_off_the_canvas_are_dropped() {
        let mut canvas = Canvas::new(2, 2);
        for (x, y) in [(-1, 0), (0, -1), (4, 0), (0, 8), (i32::MAX, i32::MIN)] {
            canvas.set(x, y, 0.0);
        }
        assert!((0..2).all(|row| (0..2).all(|col| canvas.cell(col, row) == Cell::default())));
    }

    #[test]
    fn a_cell_takes_the_shade_of_its_nearest_dot() {
        let mut canvas = Canvas::new(1, 1);
        canvas.set(0, 0, 1.0);
        assert_eq!(canvas.cell(0, 0).shade, SHADES - 1);
        canvas.set(1, 0, 0.0);
        canvas.set(0, 1, 0.5);
        assert_eq!(canvas.cell(0, 0).shade, 0);
        assert_eq!((canvas.cell(0, 0).dots).count_ones(), 3);
    }

    #[test]
    fn a_line_covers_every_cell_between_its_ends_and_runs_in_depth() {
        let mut canvas = Canvas::new(4, 1);
        canvas.line(point(0.0, 0.0, 0.0), point(7.0, 0.0, 1.0));
        let cells: Vec<Cell> = (0..4).map(|col| canvas.cell(col, 0)).collect();
        assert!(cells.iter().all(|c| c.dots == 0x09));
        let shades: Vec<u8> = cells.iter().map(|c| c.shade).collect();
        assert_eq!(shades[0], 0);
        assert!(shades[3] >= SHADES - 3);
        assert!(shades.windows(2).all(|w| w[0] <= w[1]));
    }

    #[test]
    fn a_line_between_a_point_and_itself_is_one_dot_and_a_steep_line_is_whole() {
        let mut canvas = Canvas::new(1, 2);
        canvas.line(point(1.0, 2.0, 0.0), point(1.0, 2.0, 0.0));
        assert_eq!(canvas.cell(0, 0).dots, 0x20);
        let mut canvas = Canvas::new(1, 2);
        canvas.line(point(0.0, 0.0, 0.0), point(0.0, 7.0, 0.0));
        assert_eq!(canvas.cell(0, 0).dots, 0x47);
        assert_eq!(canvas.cell(0, 1).dots, 0x47);
    }

    #[test]
    fn a_line_with_fractional_ends_skips_no_dot() {
        let mut canvas = Canvas::new(2, 1);
        canvas.line(point(0.49, 0.0, 0.0), point(2.98, 0.0, 0.0));
        let dot = |x: u16| canvas.cell(x / 2, 0).dots & DOT_BITS[0][usize::from(x % 2)] != 0;
        assert!((0..4).all(dot));
        let mut canvas = Canvas::new(1, 2);
        canvas.line(point(0.0, 0.4, 0.0), point(0.0, 5.7, 0.0));
        let dots: u32 = (0..2)
            .map(|row| canvas.cell(0, row).dots.count_ones())
            .sum();
        assert_eq!(dots, 7);
    }

    #[test]
    fn a_line_leaving_the_canvas_keeps_the_part_inside() {
        let mut canvas = Canvas::new(1, 1);
        canvas.line(point(-50.0, 0.0, 0.0), point(50.0, 0.0, 0.0));
        assert_eq!(canvas.cell(0, 0).dots, 0x09);
    }

    #[test]
    fn the_corners_fit_the_smaller_side_and_nearly_fill_it_at_the_widest() {
        let tesseract = Tesseract::new();
        for (width, height) in [(40.0f32, 40.0f32), (200.0, 80.0), (60.0, 300.0)] {
            let half = (width.min(height) - 1.0) / 2.0;
            let (mut widest, mut tallest) = (0f32, 0f32);
            for step in 0..4000 {
                let seconds = f64::from(step) * 0.05;
                for p in tesseract.corners(seconds, width, height) {
                    let center = ((width - 1.0) / 2.0, (height - 1.0) / 2.0);
                    let (dx, dy) = ((p.x - center.0).abs(), (p.y - center.1).abs());
                    assert!(dx <= half && dy <= half, "{dx} {dy} outside {half}");
                    assert!((0.0..=1.0).contains(&p.depth));
                    widest = widest.max(dx);
                    tallest = tallest.max(dy);
                }
            }
            let reach = widest.max(tallest);
            assert!(reach > 0.9 * half, "{reach} of {half}");
        }
    }

    #[test]
    fn the_corners_round_to_dots_on_the_canvas() {
        let tesseract = Tesseract::new();
        for (cols, rows) in [(30u16, 2u16), (1, 1), (2, 40), (3, 3), (60, 20)] {
            let (width, height) = (f32::from(cols) * 2.0, f32::from(rows) * 4.0);
            for step in 0..2000 {
                for p in tesseract.corners(f64::from(step) * 0.0361, width, height) {
                    let (x, y) = (p.x.round(), p.y.round());
                    assert!((0.0..width).contains(&x), "{x} of {width}");
                    assert!((0.0..height).contains(&y), "{y} of {height}");
                }
            }
        }
    }

    #[test]
    fn the_corners_move_a_little_each_frame() {
        let tesseract = Tesseract::new();
        let at = |seconds| tesseract.corners(seconds, 160.0, 80.0);
        assert_ne!(at(0.0), at(1.0));
        for (a, b) in at(5.0).iter().zip(at(5.04)) {
            assert!((a.x - b.x).hypot(a.y - b.y) < 3.0);
        }
    }

    #[test]
    fn a_drawing_is_inside_its_canvas_and_shades_by_depth() {
        let tesseract = Tesseract::new();
        let canvas = tesseract.draw(30, 12, 3.0);
        let cells: Vec<Cell> = (0..12)
            .flat_map(|row| (0..30).map(move |col| (col, row)))
            .map(|(col, row)| canvas.cell(col, row))
            .collect();
        let drawn: Vec<&Cell> = cells.iter().filter(|c| c.dots != 0).collect();
        assert!(drawn.len() > 30);
        assert!(drawn.iter().any(|c| c.shade < 4));
        assert!(drawn.iter().any(|c| c.shade > 6));
        // Every edge has its 32 ends on the canvas, so nothing is cut off.
        let (width, height) = (60.0, 48.0);
        assert!(
            tesseract
                .corners(3.0, width, height)
                .iter()
                .all(|p| (0.0..width).contains(&p.x) && (0.0..height).contains(&p.y))
        );
    }
}
