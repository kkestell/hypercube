//! The theme: the colors and spacing of everything Hypercube draws around the
//! panes. They are constants for now; `docs/themes.md` has the direction.

/// An RGB color.
pub type Rgb = (u8, u8, u8);

/// A foreground and a background color.
#[derive(Clone, Copy)]
pub struct Colors {
    pub fg: Rgb,
    pub bg: Rgb,
}

impl Colors {
    /// The SGR sequence that resets the attributes, then sets both colors.
    pub fn sgr(self) -> String {
        let ((fr, fg, fb), (br, bg, bb)) = (self.fg, self.bg);
        format!("\x1b[0;38;2;{fr};{fg};{fb};48;2;{br};{bg};{bb}m")
    }
}

const BLACK: Rgb = (0, 0, 0);
const LIGHT_BLUE: Rgb = (135, 215, 255);

/// The header across the top row.
pub const HEADER: Colors = Colors {
    fg: BLACK,
    bg: LIGHT_BLUE,
};
/// The footer across the last row.
pub const FOOTER: Colors = Colors {
    fg: BLACK,
    bg: LIGHT_BLUE,
};
/// A worker's title above its pane.
pub const TITLE: Colors = Colors {
    fg: BLACK,
    bg: LIGHT_BLUE,
};
/// Behind the panes, in the gaps.
pub const BACKDROP: Rgb = (98, 100, 108);
/// The shadow under and to the right of each pane.
pub const SHADOW: Rgb = (74, 76, 84);

/// The gap around the panes and between them, in cells. Each pane's shadow
/// takes one more column to its right, so the gaps there are a column wider.
pub const GAP: u32 = 1;
