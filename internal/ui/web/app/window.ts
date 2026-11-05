// The desktop app's bridge, absent in a browser.
export const hypercube = () => globalThis.hypercube;

// startWindowEvents re-dispatches the app's reports as window events. The
// bridge lives in an isolated world, where an event's detail would be lost.
export function startWindowEvents() {
  hypercube()?.onEvent((name, detail) => dispatchEvent(new CustomEvent(name, {detail})));
}
