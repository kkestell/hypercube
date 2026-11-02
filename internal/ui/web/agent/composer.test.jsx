import {afterEach, expect, test} from 'bun:test';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {Composer, submitsOnKey} from './composer.jsx';

afterEach(unmountAll);

const agent = {id: 'a', name: 'lead', role: 'lead', activity: 'idle', harness: 'claude', model: 'opus', effort: 'high'};
const store = () => createStore({drafts: {}, unsent: null, session: null});

test('submitsOnKey sends on Enter alone', () => {
  expect(submitsOnKey({key: 'Enter'})).toBe(true);
  expect(submitsOnKey({key: 'Enter', shiftKey: true})).toBe(false);
  expect(submitsOnKey({key: 'Enter', isComposing: true})).toBe(false);
  expect(submitsOnKey({key: 'Enter', metaKey: true})).toBe(false);
  expect(submitsOnKey({key: 'a'})).toBe(false);
});

test('a composer focuses its text box when it mounts', () => {
  const root = mount(store(), <Composer agent={agent} />);
  expect(document.activeElement).toBe(root.querySelector('textarea'));
  expect(root.querySelector('.composer-config').textContent).toBe('claude · opus · high');
});

test('a composer leaves the focus in a tab strip', () => {
  const strip = document.createElement('div');
  strip.className = 'editor-tabs';
  const tab = document.createElement('button');
  strip.append(tab);
  document.body.append(strip);
  tab.focus();
  mount(store(), <Composer agent={agent} />);
  expect(document.activeElement).toBe(tab);
});

test('send is disabled without content and stop shows while the agent works', () => {
  const s = store();
  let root = mount(s, <Composer agent={agent} />);
  const send = () => root.querySelector('[title="Send"]');
  expect(send().hasAttribute('disabled')).toBe(true);
  expect(root.querySelector('[title="Stop"]')).toBeNull();
  const field = root.querySelector('textarea');
  field.value = 'hello';
  field.dispatchEvent(new Event('input'));
  expect(send().hasAttribute('disabled')).toBe(false);
  unmountAll();
  root = mount(s, <Composer agent={{...agent, activity: 'busy'}} />);
  expect(root.querySelector('[title="Stop"]')).not.toBeNull();
});
