import {afterEach, expect, spyOn, test} from 'bun:test';
import {act, useEffect} from 'react';
import {element, find, render, unmountAll} from '../../test-render.tsx';
import {Split} from './split.tsx';

afterEach(unmountAll);

const sides: ('start' | 'end')[] = ['start', 'end'];


test('collapsing and restoring either pane keeps both views mounted and hides the separator', () => {
  const root = document.createElement('div');
  document.body.append(root);
  let mounts = 0;
  const Pane = ({name}: {name: string}) => {
    useEffect(() => { mounts++; }, []);
    return <div className={name} />;
  };
  const draw = (collapsed: 'start' | 'end' | null) => act(() => render(
    <Split fixed="start" size={260} minStart="160px" collapsed={collapsed}><Pane name="a" /><Pane name="b" /></Split>, root));
  draw(null);
  const a = find(root, '.a'), b = find(root, '.b');
  for (const side of sides) {
    draw(side);
    expect(find(root, `[data-side="${side}"]`).hasAttribute('inert')).toBe(true);
    expect(find(root, '[data-slot=resizable-handle]').hidden).toBe(true);
    draw(null);
    expect(find(root, '.a') === a).toBe(true);
    expect(find(root, '.b') === b).toBe(true);
    expect(find(root, `[data-side="${side}"]`).hasAttribute('inert')).toBe(false);
    expect(find(root, '[data-slot=resizable-handle]').hidden).toBe(false);
  }
  expect(mounts).toBe(2);
});

test('a nested split mounted in a hidden parent stays mounted across toggles', () => {
  const root = document.createElement('div');
  document.body.append(root);
  const draw = (hidden: boolean) => act(() => render(
    <Split fixed="end" size={320} collapsed={hidden ? 'end' : null}><div>Center</div>
      <Split fixed="end" size={240} collapsed="end"><div>Files</div><textarea defaultValue="Draft" /></Split>
    </Split>, root));
  draw(true);
  const field = element(root, 'textarea', HTMLTextAreaElement);
  field.value = 'Kept draft';
  draw(false);
  expect(find(root, 'textarea') === field).toBe(true);
  expect(field.value).toBe('Kept draft');
  expect(field.closest('[data-side=end]')?.hasAttribute('inert')).toBe(true);
});

test('fixed sidebars restore their actual size and separators resize with the keyboard', () => {
  // Happy DOM has no layout engine; give the library measurable panel sizes.
  const bounds = spyOn(HTMLElement.prototype, 'getBoundingClientRect');
  const width = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetWidth') ?? {value: 0};
  const height = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetHeight') ?? {value: 0};
  bounds.mockReturnValue(new DOMRect(0, 0, 1000, 600));
  Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {configurable: true, get: () => 1000});
  Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {configurable: true, get: () => 600});
  try {
    const root = document.createElement('div');
    document.body.append(root);
    for (const fixed of sides) {
      const draw = (collapsed: 'start' | 'end' | null) => act(() => render(
        <Split key={fixed} fixed={fixed} size={260} minStart="160px" minEnd="40px" collapsed={collapsed}><div>A</div><div>B</div></Split>, root));
      draw(null);
      const panel = root.querySelectorAll<HTMLElement>('[data-panel]')[fixed === 'start' ? 0 : 1];
      const before = Number(panel.style.flexGrow);
      expect(before).toBeGreaterThan(0);
      draw(fixed);
      expect(Number(panel.style.flexGrow)).toBe(0);
      draw(null);
      expect(Number(panel.style.flexGrow)).toBeCloseTo(before);
      const separator = find(root, '[data-slot=resizable-handle]');
      act(() => separator.dispatchEvent(new KeyboardEvent('keydown', {key: 'ArrowRight', bubbles: true, cancelable: true})));
      expect(Number(panel.style.flexGrow)).not.toBe(before);
    }
  } finally {
    bounds.mockRestore();
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', width);
    Object.defineProperty(HTMLElement.prototype, 'offsetHeight', height);
  }
});
