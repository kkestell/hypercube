// The panel's terminal: xterm.js with its WebGL renderer, which draws box and
// block characters to fill their cells, the fit add-on, standard terminal
// colors, and the macOS text editing keys.
import {FitAddon} from '@xterm/addon-fit';
import {WebglAddon} from '@xterm/addon-webgl';
import {Terminal} from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';

// WebglRenderer is the part of xterm.js's WebGL renderer, which the add-on
// keeps in a private field, that the terminal repaints through.
interface WebglRenderer {
  handleResize(cols: number, rows: number): void;
  _setCanvasDevicePixelDimensions(width: number, height: number): void;
  renderRows(start: number, end: number): void;
}

declare module '@xterm/addon-webgl' {
  interface WebglAddon {
    _renderer: WebglRenderer;
  }
}

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

// createTerminal opens a terminal that fills box. Call fit.fit() after box
// changes size. The editing keys arrive through term.onData like typing.
export function createTerminal(box: HTMLElement) {
  const appearance = matchMedia('(prefers-color-scheme: dark)');
  // The backgrounds match the page's, so the panel's padding blends into the
  // terminal.
  const theme = () => appearance.matches ? {background: '#0a0a0a'} : {
    background: '#ffffff', foreground: '#252525', cursor: '#252525', cursorAccent: '#ffffff',
    selectionBackground: '#b5d5ff',
    black: '#000000', red: '#cd3131', green: '#00a000', yellow: '#795e00',
    blue: '#0451a5', magenta: '#bc05bc', cyan: '#0598bc', white: '#555555',
    brightBlack: '#666666', brightRed: '#cd3131', brightGreen: '#008000', brightYellow: '#795e00',
    brightBlue: '#0451a5', brightMagenta: '#bc05bc', brightCyan: '#0598bc', brightWhite: '#000000',
  };
  const term = new Terminal({
    theme: theme(),
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
    fontSize: 13,
    cursorInactiveStyle: 'none',
  });
  const updateTheme = () => { term.options.theme = theme(); };
  appearance.addEventListener('change', updateTheme);
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
  const webgl = new WebglAddon();
  webgl.onContextLoss(() => webgl.dispose());
  term.loadAddon(webgl);
  // The renderer clears its canvas when the terminal resizes and in its
  // device-pixel ResizeObserver, then schedules the repaint with
  // requestAnimationFrame. Resize observers run after animation callbacks, so
  // the repaint lands a frame late and the terminal flashes blank. xterm.js
  // has no public way to repaint synchronously, so repaint right after either
  // clear.
  const repainting = <Args extends unknown[]>(clear: (this: WebglRenderer, ...args: Args) => void) =>
    function(this: WebglRenderer, ...args: Args) {
      clear.apply(this, args);
      this.renderRows(0, term.rows - 1);
    };
  const renderer = webgl._renderer;
  renderer.handleResize = repainting(renderer.handleResize);
  renderer._setCanvasDevicePixelDimensions = repainting(renderer._setCanvasDevicePixelDimensions);
  return {term, fit, dispose: () => {
    appearance.removeEventListener('change', updateTheme);
    term.dispose();
  }};
}
