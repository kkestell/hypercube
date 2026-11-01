// The panel's terminal: xterm.js with its WebGL renderer and fit add-on.
import {Terminal} from '@xterm/xterm';
import {FitAddon} from '@xterm/addon-fit';
import {WebglAddon} from '@xterm/addon-webgl';

// createTerminal opens a terminal in box that fills it. Call fit.fit() after
// box changes size.
export function createTerminal(box) {
  const term = new Terminal({
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    fontSize: 13,
    theme: {background: '#1e1e1e'},
    cursorInactiveStyle: 'none',
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  term.open(box);
  // The WebGL renderer draws box and block characters to fill their cells
  // exactly, as terminals do.
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
