//! The colors of everything Hypercube draws around the panes: shades of one
//! accent color, which each workspace picks from `ACCENTS`.

/// An RGB color.
pub type Rgb = (u8, u8, u8);

/// A foreground and a background color.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Colors {
    pub fg: Rgb,
    pub bg: Rgb,
}

/// The SGR sequence that resets the attributes, then sets the foreground.
pub fn fg(c: Rgb) -> String {
    format!("\x1b[0;38;2;{};{};{}m", c.0, c.1, c.2)
}

impl Colors {
    /// The SGR sequence that resets the attributes, then sets both colors.
    pub fn sgr(self) -> String {
        let (fg, bg) = (self.fg, self.bg);
        format!(
            "\x1b[0;38;2;{};{};{};48;2;{};{};{}m",
            fg.0, fg.1, fg.2, bg.0, bg.1, bg.2
        )
    }
}

const BLACK: Rgb = (0, 0, 0);
const WHITE: Rgb = (255, 255, 255);

/// The dot on the tab of a workspace with an agent waiting for a tool
/// approval.
pub const ATTENTION: Rgb = (255, 70, 70);

/// The accents, in the order Ctrl-b a cycles through them: blue, teal,
/// green, amber, orange, red, pink, and purple.
pub const ACCENTS: [Rgb; 8] = [
    (70, 160, 220),
    (40, 180, 170),
    (100, 180, 80),
    (220, 170, 50),
    (230, 120, 50),
    (220, 80, 80),
    (220, 90, 170),
    (150, 110, 220),
];

/// The colors of the chrome, all shades of one accent.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Palette {
    /// The divider and the titles' lines: the accent itself.
    pub line: Rgb,
    /// The titles' text: the accent lightened.
    pub text: Rgb,
    /// The header, the bar, and the title rows of the overlay and the list
    /// of keys: the accent lightened nearly to white on the accent darkened.
    pub bar: Colors,
    /// The workspace's entry in the bar: the bar's text on the accent
    /// darkened a little less than the bar.
    pub tab: Colors,
    /// The workspace's number on its tab: the accent lightened less than
    /// the tab's text.
    pub number: Rgb,
}

impl Palette {
    /// The palette of the accent numbered `n` in `ACCENTS`, counting around.
    pub fn of(n: usize) -> Palette {
        let accent = ACCENTS[n % ACCENTS.len()];
        Palette {
            line: accent,
            text: mix(accent, WHITE, 0.4),
            bar: Colors {
                fg: mix(accent, WHITE, 0.85),
                bg: mix(accent, BLACK, 0.78),
            },
            tab: Colors {
                fg: mix(accent, WHITE, 0.85),
                bg: mix(accent, BLACK, 0.65),
            },
            number: mix(accent, WHITE, 0.6),
        }
    }
}

/// The accent fewest of the `taken` use, the first of those in `ACCENTS`.
pub fn least_used(taken: &[usize]) -> usize {
    (0..ACCENTS.len())
        .min_by_key(|&n| taken.iter().filter(|&&t| t % ACCENTS.len() == n).count())
        .unwrap_or(0)
}

impl Default for Palette {
    fn default() -> Palette {
        Palette::of(0)
    }
}

/// `c` moved `amount`, from 0 to 1, of the way to `to`. Toward black or
/// white, the hue stays.
fn mix(c: Rgb, to: Rgb, amount: f32) -> Rgb {
    let m = |a: u8, b: u8| (f32::from(a) + (f32::from(b) - f32::from(a)) * amount).round() as u8;
    (m(c.0, to.0), m(c.1, to.1), m(c.2, to.2))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_palette_shades_its_accent() {
        let p = Palette::of(0);
        assert_eq!(p.line, (70, 160, 220));
        assert_eq!(p.text, (144, 198, 234));
        assert_eq!(p.bar.fg, (227, 241, 250));
        assert_eq!(p.bar.bg, (15, 35, 48));
        assert_eq!(p.tab.bg, (25, 56, 77));
        assert_eq!(Palette::of(ACCENTS.len() + 1), Palette::of(1));
    }

    #[test]
    fn a_new_workspace_takes_the_least_used_accent() {
        assert_eq!(least_used(&[]), 0);
        assert_eq!(least_used(&[0, 1, 3]), 2);
        let all: Vec<usize> = (0..ACCENTS.len()).collect();
        assert_eq!(least_used(&[all.clone(), vec![0, 1]].concat()), 2);
    }
}
