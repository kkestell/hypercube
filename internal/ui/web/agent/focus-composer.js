// focusComposer puts the focus in the text box of a session's composer, if one
// is drawn.
export const focusComposer = session => document.querySelector(`.composer[data-session="${session}"] textarea`)?.focus();
