// The panel's terminal: stock xterm.js, the fit add-on, standard terminal
// colors, and macOS text editing keys.
import {FitAddon} from '@xterm/addon-fit';
import {Terminal} from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';

// editing gives the bytes Ghostty sends for the macOS text editing shortcuts,
// which xterm.js leaves to the browser.
const editingKeys = new Map([
  ['cmd+Backspace', '\x15'],
  ['cmd+ArrowLeft', '\x01'],
  ['cmd+ArrowRight', '\x05'],
  ['alt+ArrowLeft', '\x1bb'],
  ['alt+ArrowRight', '\x1bf'],
]);

function editing(e: KeyboardEvent): string | undefined {
  const mods = (e.metaKey ? 'cmd+' : '') + (e.altKey ? 'alt+' : '') + (e.ctrlKey ? 'ctrl+' : '') + (e.shiftKey ? 'shift+' : '');
  return editingKeys.get(mods + e.key);
}

// The backgrounds match the page's, so the panel's padding blends into the
// terminal.
const theme = (dark: boolean) => dark ? {background: '#0a0a0a'} : {
  background: '#ffffff', foreground: '#252525', cursor: '#252525', cursorAccent: '#ffffff',
  selectionBackground: '#b5d5ff',
  black: '#000000', red: '#cd3131', green: '#00a000', yellow: '#795e00',
  blue: '#0451a5', magenta: '#bc05bc', cyan: '#0598bc', white: '#555555',
  brightBlack: '#666666', brightRed: '#cd3131', brightGreen: '#008000', brightYellow: '#795e00',
  brightBlue: '#0451a5', brightMagenta: '#bc05bc', brightCyan: '#0598bc', brightWhite: '#000000',
};

// TerminalStyle is the page's appearance and the Settings text size.
export interface TerminalStyle {
  dark: boolean;
  fontSize: number;
}

// createTerminal opens a terminal that fills box. Call fit.fit() after box
// changes size or after restyle. The editing keys arrive through term.onData
// like typing.
export function createTerminal(box: HTMLElement, style: TerminalStyle) {
  const term = new Terminal({
    theme: theme(style.dark),
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
    fontSize: style.fontSize,
    cursorInactiveStyle: 'none',
  });
  const restyle = ({dark, fontSize}: TerminalStyle) => {
    term.options.theme = theme(dark);
    term.options.fontSize = fontSize;
  };
  const fit = new FitAddon();
  term.loadAddon(fit);
  term.open(box);
  term.attachCustomKeyEventHandler(e => {
    const text = e.type === 'keydown' && editing(e);
    if (!text) return true;
    e.preventDefault();
    term.input(text);
    return false;
  });
  return {term, fit, restyle, dispose: () => term.dispose()};
}
