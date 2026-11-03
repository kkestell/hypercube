import {afterEach, expect, spyOn, test} from 'bun:test';
import {options, render} from 'preact';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
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
      visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
      workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files'},
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

test('Escape interrupts only the shown busy session and leaves other Escape actions alone', () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  try {
    const sessions = ['a', 'b'].map(id => ({id, title: id, running: true, activity: 'busy', children: []}));
    const store = createStore(receivedWorkspaces({
      workspaces: [], requests: [], selected: 'a', pending: null, unsent: null,
      visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
      workbench: {sidebar: true, rightPanel: false, panel: false, rightTab: 'changes'},
    }, [{id: 'w', name: 'repo', path: '/repo', sessions}]));
    const root = mount(store, <App store={store} />);
    fetches.mockClear();
    const escape = (target, options = {}) => {
      const event = new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true, ...options});
      act(() => target.dispatchEvent(event));
      return event;
    };
    const field = root.querySelector('.composer textarea');
    expect(escape(field).defaultPrevented).toBe(true);
    expect(fetches).toHaveBeenCalledTimes(1);
    expect(Object.fromEntries(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams)).toEqual({action: 'interrupt', id: 'a'});

    escape(field, {repeat: true});
    escape(field, {isComposing: true});
    escape(field, {metaKey: true});
    escape(root.querySelector('.terminal-panel'));
    const handled = new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true});
    handled.preventDefault();
    act(() => field.dispatchEvent(handled));
    expect(fetches).toHaveBeenCalledTimes(1);

    act(() => root.querySelector('.session-item-menu').click());
    escape(root.querySelector('.context-menu'));
    expect(root.querySelector('.context-menu')).toBeNull();
    expect(fetches).toHaveBeenCalledTimes(1);

    store.update(state => ({...state, confirmation: {message: 'Confirm?', primary: 'OK', accept() {}}}));
    escape(root.querySelector('dialog'));
    expect(store.get().confirmation).toBeNull();
    expect(fetches).toHaveBeenCalledTimes(1);

    store.update(state => receivedWorkspaces(state, [{id: 'w', name: 'repo', path: '/repo', sessions: [{...sessions[0], activity: 'idle'}, sessions[1]]}]));
    escape(field);
    expect(fetches).toHaveBeenCalledTimes(1);
  } finally {
    unmountAll();
    fetches.mockRestore();
  }
});
