// The page's light or dark appearance, shown with shadcn's dark class.
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {hypercube} from './window.ts';

const systemAppearance = () => matchMedia('(prefers-color-scheme: dark)');

function show(dark: boolean) {
  document.documentElement.classList.toggle('dark', dark);
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
}

// isDark applies the Settings theme, following the Mac until the config loads.
export function isDark(state: State) {
  const theme = state.config?.theme ?? 'system';
  return theme === 'system' ? state.systemDark : theme === 'dark';
}

// startAppearance shows the page's appearance as the Mac's and the theme
// change. The desktop app takes the theme for its native appearance, which
// its other pages follow.
export function startAppearance(store: Store) {
  const appearance = systemAppearance();
  const system = () => store.update(state => state.systemDark === appearance.matches ? state : {...state, systemDark: appearance.matches});
  let theme: string | undefined;
  const update = (state: State) => {
    show(isDark(state));
    if (!state.config || state.config.theme === theme) return;
    theme = state.config.theme;
    hypercube()?.postMessage({type: 'theme', theme});
  };
  system();
  update(store.get());
  appearance.addEventListener('change', system);
  store.subscribe(update);
}

// startSystemAppearance follows the Mac's appearance, for the desktop app's
// own pages.
export function startSystemAppearance() {
  const appearance = systemAppearance();
  const update = () => show(appearance.matches);
  update();
  appearance.addEventListener('change', update);
}
