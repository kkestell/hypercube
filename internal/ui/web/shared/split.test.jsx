import {afterEach, expect, test} from 'bun:test';
import {render} from 'preact';
import {useEffect} from 'preact/hooks';
import {act} from 'preact/test-utils';
import {Split} from './split.jsx';

const root = document.createElement('div');
afterEach(() => render(null, root));

test('collapsing a pane keeps both panes mounted', () => {
  let mounts = 0;
  const Pane = ({name}) => {
    useEffect(() => { mounts++; }, []);
    return <div class={name} />;
  };
  const draw = collapsed => act(() => render(
    <Split fixed="start" size={260} minStart="160px" collapsed={collapsed}><Pane name="a" /><Pane name="b" /></Split>, root));
  draw(null);
  const [a, b] = [root.querySelector('.a'), root.querySelector('.b')];
  draw('start');
  expect(root.querySelector('vscode-split-layout').getAttribute('handle-size')).toBe('0');
  expect(root.querySelector('vscode-split-layout').hasAttribute('min-start')).toBe(false);
  draw(null);
  expect(root.querySelector('.a')).toBe(a);
  expect(root.querySelector('.b')).toBe(b);
  expect(mounts).toBe(2);
});

test('collapsed panes are inert', () => {
  const draw = collapsed => act(() => render(
    <Split size={50} collapsed={collapsed}><button>Start</button><button>End</button></Split>, root));
  for (const side of ['start', 'end']) {
    draw(null);
    const pane = root.querySelector(`[slot="${side}"]`);
    draw(side);
    expect(pane.hasAttribute('inert')).toBe(true);
    expect(root.querySelector(`[slot="${side === 'start' ? 'end' : 'start'}"]`).hasAttribute('inert')).toBe(false);
    draw(null);
    expect(pane.hasAttribute('inert')).toBe(false);
  }
});

test('real split panes stay collapsed after mounting inside a hidden parent and restore their sizes', () => {
  // Isolate custom-element registration from tests that use plain elements.
  const result = Bun.spawnSync([process.execPath, '-e', `
    import './test-dom.js';
    import {h, render} from 'preact';
    import {act} from 'preact/test-utils';
    import {expect} from 'bun:test';
    import {Split} from './web/shared/split.jsx';
    await import('@vscode-elements/elements/dist/vscode-split-layout/index.js');
    let width = 0;
    HTMLElement.prototype.getBoundingClientRect = () => ({width, height: width, left: 0, top: 0});
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {get: () => width});
    Object.defineProperty(HTMLElement.prototype, 'clientHeight', {get: () => width});
    const root = document.createElement('div');
    document.body.append(root);
    const settle = async () => {
      for (const element of root.querySelectorAll('vscode-split-layout')) {
        await element.updateComplete;
        await element.updateComplete;
      }
    };
    for (const fixed of ['none', 'start', 'end']) {
      for (const collapsed of ['start', 'end']) {
        width = 0;
        const draw = (parentHidden, childHidden) => act(() => render(
          h(Split, {size: 50, collapsed: parentHidden ? 'end' : null},
            h('div'), h(Split, {size: fixed === 'none' ? 40 : 240, fixed, collapsed: childHidden ? collapsed : null}, h('div'), h('div'))), root));
        draw(true, true);
        await settle();
        width = 1000;
        draw(false, true);
        await settle();
        const child = root.querySelectorAll('vscode-split-layout')[1];
        expect(child.shadowRoot.querySelector('.' + collapsed).style.flex).toBe('0 0 0px');
        draw(false, false);
        await settle();
        expect(child.fixedPane).toBe(fixed);
        expect(child.handlePosition).toBe(fixed === 'none' ? '40%' : fixed === 'start' ? '240px' : '760px');
        if (fixed !== 'none') expect(child.shadowRoot.querySelector('.' + fixed).style.flex).toBe('0 0 240px');
        act(() => render(null, root));
      }
    }
  `], {cwd: new URL('../..', import.meta.url).pathname});
  expect(result.stderr.toString()).toBe('');
  expect(result.exitCode).toBe(0);
});

test('the first grab after a parent resize preserves the divider and captures the drag', () => {
  const result = Bun.spawnSync([process.execPath, '-e', `
    import './test-dom.js';
    import {h, render} from 'preact';
    import {act} from 'preact/test-utils';
    import {expect} from 'bun:test';
    import {Split} from './web/shared/split.jsx';
    await import('@vscode-elements/elements/dist/vscode-split-layout/index.js');
    let length = 600;
    let vertical = true;
    // Model a parent resizing while the percentage divider stays in place.
    HTMLElement.prototype.getBoundingClientRect = function() {
      let left = 100, top = 60, width = length, height = length;
      if (this.classList.contains('handle')) {
        const raw = this.style[vertical ? 'left' : 'top'];
        const position = parseFloat(raw) * (raw.endsWith('%') ? length / 100 : 1);
        if (vertical) { left += position - 2; width = 4; }
        else { top += position - 2; height = 4; }
      }
      return {left, top, width, height, right: left + width, bottom: top + height};
    };
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {get: () => length});
    Object.defineProperty(HTMLElement.prototype, 'clientHeight', {get: () => length});
    const root = document.createElement('div');
    document.body.append(root);
    for (const split of ['vertical', 'horizontal']) {
      vertical = split === 'vertical';
      length = 600;
      let resized, captured;
      act(() => render(h(Split, {split, size: 40, onResize: value => resized = value},
        h('div', {class: 'text'}, 'Pane'), h('div')), root));
      const element = root.querySelector('vscode-split-layout');
      await element.updateComplete;
      await element.updateComplete;
      length = 900;
      const handle = element.shadowRoot.querySelector('.handle');
      handle.setPointerCapture = id => captured = id;
      const bounds = handle.getBoundingClientRect();
      const center = () => {
        const r = handle.getBoundingClientRect();
        return vertical ? r.left + r.width / 2 : r.top + r.height / 2;
      };
      const before = center();
      const coordinates = {clientX: bounds.left + bounds.width / 2, clientY: bounds.top + bounds.height / 2};
      const options = {...coordinates, button: 0, buttons: 1, bubbles: true, composed: true, cancelable: true};
      window.getSelection().selectAllChildren(root.querySelector('.text'));
      handle.dispatchEvent(new PointerEvent('pointerdown', {...options, pointerId: 7}));
      const down = new MouseEvent('mousedown', options);
      handle.dispatchEvent(down);
      await element.updateComplete;
      expect(center()).toBeCloseTo(before);
      expect(captured).toBe(7);
      expect(down.defaultPrevented).toBe(true);
      expect(window.getSelection().rangeCount).toBe(0);
      const moved = {...coordinates};
      moved[vertical ? 'clientX' : 'clientY'] += 90;
      window.dispatchEvent(new MouseEvent('mousemove', {...moved, buttons: 1}));
      await element.updateComplete;
      expect(center()).toBeCloseTo(before + 90);
      window.dispatchEvent(new MouseEvent('mouseup', moved));
      expect(resized).toBeCloseTo(50);
      act(() => render(null, root));
    }
  `], {cwd: new URL('../..', import.meta.url).pathname});
  expect(result.stderr.toString()).toBe('');
  expect(result.exitCode).toBe(0);
});
