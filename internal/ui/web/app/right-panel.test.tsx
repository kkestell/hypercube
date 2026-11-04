import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, findAll, mount, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {RightPanel} from './right-panel.tsx';

afterEach(() => { mock.restore(); unmountAll(); });

test('the right sidebar offers Files and Search, initially Files', () => {
  const store = createStore(testState({workbench: {...testState().workbench, rightPanel: false}}));
  const root = mount(store, <RightPanel />);
  const tabs = findAll(root, '[role="tab"]');
  expect(tabs.map(tab => tab.title)).toEqual(['Files', 'Search']);
  expect(tabs.map(tab => tab.getAttribute('aria-selected'))).toEqual(['true', 'false']);
  act(() => clickControl(tabs[1]));
  expect(store.get().workbench.rightTab).toBe('search');
});

test('side-panel tabs support arrow keys, Home, and End, retaining both views', async () => {
  const store = createStore(testState({workbench: {...testState().workbench, rightPanel: false}}));
  const root = mount(store, <RightPanel />);
  const tabs = findAll(root, '[role="tab"]');
  const panels = findAll(root, '[role="tabpanel"]');
  act(() => tabs[0].focus());
  const steps: [string, number][] = [['ArrowRight', 1], ['ArrowRight', 0], ['ArrowLeft', 1], ['Home', 0], ['End', 1]];
  for (const [key, index] of steps) {
    act(() => document.activeElement?.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));
    await act(() => new Promise(resolve => setTimeout(resolve)));
    expect(document.activeElement === tabs[index]).toBe(true);
    expect(tabs[index].getAttribute('aria-selected')).toBe('true');
    expect(panels[index].hidden).toBe(false);
    expect(panels.filter((_, i) => i !== index).every(panel => panel.hidden)).toBe(true);
    expect(tabs[index].getAttribute('aria-controls')).toBe(panels[index].id);
    expect(panels[index].getAttribute('aria-labelledby')).toBe(tabs[index].id);
  }
  expect(findAll(root, '[role="tabpanel"]').every((panel, i) => panel === panels[i])).toBe(true);
});
