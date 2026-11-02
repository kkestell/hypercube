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
