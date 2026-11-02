// Renders a component for a test with a store, as App provides it. Each
// store update redraws the page before update returns.
import {render} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {act} from 'preact/test-utils';
import {StoreContext} from './web/store.js';

function Provider({store, children}) {
  const [state, setState] = useState(store.get());
  useEffect(() => store.subscribe(next => act(() => setState(next))), []);
  return <StoreContext.Provider value={[state, store]}>{children}</StoreContext.Provider>;
}

export function mount(store, element) {
  const root = document.createElement('div');
  document.body.append(root);
  act(() => render(<Provider store={store}>{element}</Provider>, root));
  return root;
}

export function unmountAll() {
  for (const root of document.body.children) render(null, root);
  document.body.replaceChildren();
}
