// The platform's reading of the key notation that shortcuts are written in:
// ⌘ and ⌥ on macOS, Ctrl and Alt on Linux, where the Super key belongs to the
// desktop.
import {hypercube} from './window.ts';

// linux is true in the desktop app on Linux. Without the app, the page uses the
// macOS rule.
export const linux = () => hypercube()?.platform === 'linux';

const arrows: Record<string, string> = {'↓': 'ArrowDown', '↑': 'ArrowUp'};
const modifiers = /[⌘⌥⌃⇧]/g;

// binding translates notation into keyboard event fields. On Linux ⌘ and ⌃ both
// mean Ctrl.
export function binding(text: string) {
  const key = text.replace(modifiers, '');
  const mac = !linux();
  const ctrl = text.includes('⌃') || (!mac && text.includes('⌘'));
  return {key: arrows[key] ?? key.toLowerCase(),
    metaKey: mac && text.includes('⌘'), altKey: text.includes('⌥'), ctrlKey: ctrl, shiftKey: text.includes('⇧') || key === '+'};
}

const linuxKeys: Record<string, string> = {'↓': 'Down', '↑': 'Up'};

// keyNames gives the parts of a key's label: glyphs on macOS, and on Linux
// Alt, Ctrl, and Shift before the key, the order in which Electron's menus
// label accelerators.
export function keyNames(text: string) {
  if (!linux()) return text.match(/[⌘⌥⌃⇧]|[^⌘⌥⌃⇧]+/g) ?? [];
  const key = text.replace(modifiers, '');
  return [
    ...(text.includes('⌥') ? ['Alt'] : []),
    ...(/[⌘⌃]/.test(text) ? ['Ctrl'] : []),
    ...(text.includes('⇧') ? ['Shift'] : []),
    linuxKeys[key] ?? (key.length === 1 ? key.toUpperCase() : key),
  ];
}

// keySeparator goes between the parts of a label.
export const keySeparator = () => linux() ? '+' : '';

// keysLabel is the label as one string, for tooltips and menu text.
export const keysLabel = (text: string) => keyNames(text).join(keySeparator());
