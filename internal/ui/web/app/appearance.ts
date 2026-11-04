// Follow the system appearance using shadcn's dark class.
export function startAppearance() {
  const appearance = matchMedia('(prefers-color-scheme: dark)');
  const update = () => document.documentElement.classList.toggle('dark', appearance.matches);
  update();
  appearance.addEventListener('change', update);
}
