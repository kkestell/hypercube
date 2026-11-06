import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {find, mount, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {useStore} from '../store.ts';
import {toggle} from '../app/workbench.ts';
import {Split} from './split.tsx';

afterEach(() => {
  unmountAll();
  mock.restore();
});

// Happy-dom has no layout. Give this two-panel group a fixed 1000px width,
// letting the real resizable panels apply their own percentage changes.
function geometry() {
  const observers: {callback: ResizeObserverCallback; targets: Element[]}[] = [];
  const originalObserver = window.ResizeObserver;
  class Observer {
    targets: Element[] = [];
    constructor(callback: ResizeObserverCallback) { observers.push({callback, targets: this.targets}); }
    observe(target: Element) { this.targets.push(target); }
    unobserve() {}
    disconnect() { this.targets.length = 0; }
  }
  window.ResizeObserver = Observer as unknown as typeof ResizeObserver;
  const width = (element: HTMLElement): number => {
    if (element.hasAttribute('data-group')) return 1000;
    if (!element.hasAttribute('data-panel')) return 0;
    if (parseFloat(element.style.flexBasis) > 0) return parseFloat(element.style.flexBasis);
    const sibling = [...element.parentElement!.children].find(child => child !== element && child.hasAttribute('data-panel')) as HTMLElement;
    if (parseFloat(sibling.style.flexBasis) > 0) return 1000 - parseFloat(sibling.style.flexBasis);
    return Number(element.style.flexGrow) * 10;
  };
  const descriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetWidth')!;
  const leftDescriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetLeft')!;
  Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {configurable: true, get() { return width(this); }});
  Object.defineProperty(HTMLElement.prototype, 'offsetLeft', {configurable: true, get() {
    const first = this.parentElement?.querySelector('[data-panel]') as HTMLElement | undefined;
    return first && first !== this ? width(first) : 0;
  }});
  spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) { return new DOMRect(this.offsetLeft, 0, width(this), 400); });
  return {
    resize() {
      act(() => {
        for (const {callback, targets} of observers) callback(targets.map(target => ({target, borderBoxSize: [{inlineSize: width(target as HTMLElement), blockSize: 400}]})) as unknown as ResizeObserverEntry[], {} as ResizeObserver);
      });
    },
    restore() {
      window.ResizeObserver = originalObserver;
      Object.defineProperty(HTMLElement.prototype, 'offsetWidth', descriptor);
      Object.defineProperty(HTMLElement.prototype, 'offsetLeft', leftDescriptor);
    },
  };
}

test.each([
  {fixed: 'start', size: 260},
  {fixed: 'end', size: 320},
  {fixed: 'none', size: 50},
] as const)('$fixed split reports user keyboard changes in its units and ignores mount/collapse', ({fixed, size}) => {
  const layout = geometry();
  try {
    const sizes: number[] = [];
    const store = createStore(testState());
    function View() {
      const [state] = useStore();
      return <Split fixed={fixed} size={size} collapsed={fixed !== 'none' && !state.workbench.sidebar ? fixed : null} onResize={size => sizes.push(size)}><div>First</div><div>Last</div></Split>;
    }
    const root = mount(store, <View />);
    layout.resize();
    expect(sizes).toEqual([]);
    const handle = find(root, '[role=separator]');
    act(() => handle.dispatchEvent(new KeyboardEvent('keydown', {key: 'ArrowRight', bubbles: true, cancelable: true})));
    const panels = [...root.querySelectorAll<HTMLElement>('[data-panel]')];
    expect(sizes).toHaveLength(1);
    const expected = fixed === 'none' ? panels[0].offsetWidth / 10 : panels[fixed === 'end' ? 1 : 0].offsetWidth;
    expect(sizes[0]).toBe(expected);
    expect(sizes[0]).not.toBe(size);
    layout.resize();
    act(() => store.update(state => toggle(state, 'sidebar')));
    act(() => store.update(state => toggle(state, 'sidebar')));
    expect(sizes).toHaveLength(1);
  } finally { unmountAll(); layout.restore(); }
});
