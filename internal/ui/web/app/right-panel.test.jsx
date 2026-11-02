import {afterEach, expect, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {RightPanel} from './right-panel.jsx';

afterEach(unmountAll);

test('the right sidebar offers Files and Changes, initially Files', () => {
  const store = createStore({workbench: {rightTab: 'files'}, session: null});
  const root = mount(store, <RightPanel />);
  expect([...root.querySelectorAll('vscode-tab-header')].map(tab => tab.title)).toEqual(['Files', 'Changes']);
  const tabs = root.querySelector('vscode-tabs');
  expect(tabs.getAttribute('selected-index')).toBe('0');
  act(() => tabs.dispatchEvent(new CustomEvent('vsc-tabs-select', {detail: {selectedIndex: 1}})));
  expect(store.get().workbench.rightTab).toBe('changes');
});
