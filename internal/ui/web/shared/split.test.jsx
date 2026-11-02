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
