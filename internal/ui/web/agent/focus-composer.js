// focusComposer puts the focus in the text box of an agent's composer, if one
// is drawn.
export const focusComposer = agent => document.querySelector(`.composer[data-agent="${agent}"] textarea`)?.focus();
