import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {useModels} from './catalogs.ts';

// Several independent label sites share the catalog without opening a picker.
function Label({harness = 'claude'}: {harness?: string}) {
  const models = useModels('w', harness);
  return <span>{models[0]?.label}</span>;
}

afterEach(() => { mock.restore(); unmountAll(); });

test('label mounts share one request per key, including after remounting', async () => {
  let finish!: (response: Response) => void;
  const fetches = stubFetch(() => new Promise<Response>(resolve => { finish = resolve; }));
  const store = createStore(testState());
  const root = mount(store, <><Label /><Label /><Label /></>);
  expect(fetches).toHaveBeenCalledTimes(1);
  await act(async () => finish(Response.json({models: [{id: 'opus', label: 'Opus 5.5'}]})));
  expect(root.textContent).toBe('Opus 5.5Opus 5.5Opus 5.5');
  unmountAll();
  const remounted = mount(store, <Label />);
  expect(remounted.textContent).toBe('Opus 5.5');
  expect(fetches).toHaveBeenCalledTimes(1);
  mount(store, <Label harness="codex" />);
  expect(fetches).toHaveBeenCalledTimes(2);
  await act(async () => finish(Response.json({models: [{id: 'sol', label: 'Sol'}]})));
});
