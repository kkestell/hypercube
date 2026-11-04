// The panel's terminal: xterm.js with its WebGL renderer, which draws box and
// block characters to fill their cells, the fit add-on, standard terminal
// colors, and the macOS text editing keys.
import {FitAddon} from '@xterm/addon-fit';
import {WebglAddon} from '@xterm/addon-webgl';
import {Terminal} from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';

// editing gives the bytes Ghostty sends for the macOS text editing shortcuts,
// which xterm.js leaves to the browser.
function editing(e) {
  const mods = (e.metaKey ? 'cmd+' : '') + (e.altKey ? 'alt+' : '') + (e.ctrlKey ? 'ctrl+' : '') + (e.shiftKey ? 'shift+' : '');
  return {
    'cmd+Backspace': '\x15',
    'cmd+ArrowLeft': '\x01',
    'cmd+ArrowRight': '\x05',
    'alt+ArrowLeft': '\x1bb',
    'alt+ArrowRight': '\x1bf',
  }[mods + e.key];
}

// createTerminal opens a terminal that fills box. Call fit.fit() after box
// changes size. The editing keys arrive through term.onData like typing.
export function createTerminal(box) {
  const term = new Terminal({
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
    fontSize: 13,
    cursorInactiveStyle: 'none',
  });
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
  const renderer = webgl._renderer;
  for (const method of ['handleResize', '_setCanvasDevicePixelDimensions']) {
    const clear = renderer[method];
    renderer[method] = function(...args) {
      clear.apply(this, args);
      this.renderRows(0, term.rows - 1);
    };
  }
  return {term, fit};
}
