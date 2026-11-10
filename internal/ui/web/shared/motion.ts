// Motion shared across the view; base.css describes the timings.

// slide opens and closes collapsible content over 200ms. Content is clipped
// only past the bottom edge, and 4px beyond it so focus rings show; tree lines
// may reach up into the row above.
export const slide = '[clip-path:inset(-1rem_-1rem_-4px_-1rem)] data-[state=open]:animate-collapsible-down data-[state=closed]:animate-collapsible-up';
