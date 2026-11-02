import {afterEach, expect, spyOn, test} from 'bun:test';
import {options, render} from 'preact';
import {act} from 'preact/test-utils';
import {unmountAll} from '../../test-render.jsx';
import {receivedWorkspaces} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {App} from './app.jsx';

afterEach(unmountAll);

test('state arriving before passive effects reaches the page', async () => {
  const originalSchedule = options.requestAnimationFrame;
  const canvas = spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  options.requestAnimationFrame = () => {};
  try {
    const store = createStore({
      workspaces: [], requests: [], selected: '', pending: null, unsent: null,
      visible: true, drafts: {}, dismissed: {}, session: null,
      workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'workers'},
    });
    const root = document.createElement('div');
    document.body.append(root);
    render(<App store={store} />, root);
    store.update(state => receivedWorkspaces(state, [{id: 'w', name: 'New workspace', sessions: []}]));
    await act(async () => {});
    expect(root.querySelector('.sidebar').textContent).toContain('New workspace');
  } finally {
    options.requestAnimationFrame = originalSchedule;
    canvas.mockRestore();
  }
});
