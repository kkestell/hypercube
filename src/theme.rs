//! The themes: the layout and colors of everything Hypercube draws around the
//! panes. They are constants for now; `docs/themes.md` has the direction.

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

/// How a worker's title above its pane looks.
#[derive(Clone, Copy)]
pub enum Title {
    /// A solid bar.
    Bar(Colors),
    /// A heavy horizontal line with the text in it.
    Rule { line: Rgb, text: Rgb },
}

/// A theme: the layout and the colors of the chrome.
pub struct Theme {
    /// The header across the top row.
    pub header: Colors,
    /// The footer across the last row.
    pub footer: Colors,
    pub title: Title,
    /// The gap around the panes and between them, in cells.
    pub gap: u32,
    /// The color behind the panes, in the gaps, or the terminal's own.
    pub backdrop: Option<Rgb>,
    /// The shadow under and to the right of each pane, drawn on the
    /// backdrop. It takes one more column to the right of a pane.
    pub shadow: Option<Rgb>,
    /// The line between the orchestrator and the workers, which takes a
    /// column. It branches into a title that is a rule.
    pub divider: Option<Rgb>,
}

impl Theme {
    /// The columns to the right of a pane that are not its own: the gap, and
    /// the shadow's column.
    pub fn side_gap(&self) -> u32 {
        self.gap + u32::from(self.shadow.is_some())
    }

    /// The columns between the orchestrator and the workers.
    pub fn column_gap(&self) -> u32 {
        self.side_gap() + u32::from(self.divider.is_some())
    }
}

const BLACK: Rgb = (0, 0, 0);
const LIGHT_BLUE: Rgb = (135, 215, 255);
const BLUE: Rgb = (70, 160, 220);
const BAR: Colors = Colors {
    fg: BLACK,
    bg: LIGHT_BLUE,
};

/// Panes side by side with no gaps or shadows, a blue line between the
/// orchestrator and the workers, and a blue line for each worker's title.
pub const SIMPLE: Theme = Theme {
    header: BAR,
    footer: BAR,
    title: Title::Rule {
        line: BLUE,
        text: LIGHT_BLUE,
    },
    gap: 0,
    backdrop: None,
    shadow: None,
    divider: Some(BLUE),
};

/// Panes raised over a gray backdrop, with shadows, in gaps.
pub const FANCY: Theme = Theme {
    header: BAR,
    footer: BAR,
    title: Title::Bar(BAR),
    gap: 1,
    backdrop: Some((98, 100, 108)),
    shadow: Some((74, 76, 84)),
    divider: None,
};

/// The theme in use: `fancy` in `HYPERCUBE_THEME`, or else the simple one.
pub fn active() -> &'static Theme {
    match std::env::var("HYPERCUBE_THEME").as_deref() {
        Ok("fancy") => &FANCY,
        _ => &SIMPLE,
    }
}
