//! The colors of everything Hypercube draws around the panes.

/// An RGB color.
pub type Rgb = (u8, u8, u8);

/// A foreground and a background color.
#[derive(Clone, Copy)]
pub struct Colors {
    pub fg: Rgb,
    pub bg: Rgb,
}

/// The SGR sequence that resets the attributes, then sets the foreground.
pub fn fg(c: Rgb) -> String {
    format!("\x1b[0;38;2;{};{};{}m", c.0, c.1, c.2)
}

/// The SGR sequence that resets the attributes, then sets the background.
pub fn bg(c: Rgb) -> String {
    format!("\x1b[0;48;2;{};{};{}m", c.0, c.1, c.2)
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

/// The light blue of the header and the footer, and of the titles' text.
pub const LIGHT_BLUE: Rgb = (135, 215, 255);

/// The blue of the divider and the titles' lines.
pub const BLUE: Rgb = (70, 160, 220);

/// Black on light blue: the header and the footer.
pub const BAR: Colors = Colors {
    fg: BLACK,
    bg: LIGHT_BLUE,
};
