// The one owner of the page's shared state. Changes go through update with a
// pure function of the state; App redraws from each new state.
import {createContext, createElement, useContext, useSyncExternalStore, type ReactNode} from 'react';
import type {State} from './state.ts';

export interface Store {
  get(): State;
  update(change: (state: State) => State): void;
  subscribe(subscriber: (state: State) => void): () => void;
}

export function createStore(initial: State): Store {
  let state = initial;
  const subscribers = new Set<(state: State) => void>();
  return {
    get: () => state,
    update(change) {
      const next = change(state);
      if (next === state) return;
      state = next;
      for (const subscriber of subscribers) subscriber(state);
    },
    subscribe(subscriber) {
      subscribers.add(subscriber);
      return () => subscribers.delete(subscriber);
    },
  };
}

const StoreContext = createContext<[State, Store] | null>(null);
// The store alone, which never changes, for components that select a value.
const StoreHandle = createContext<Store | null>(null);

// StoreProvider gives its children the store and the state App last drew.
export function StoreProvider({state, store, children}: {state: State; store: Store; children: ReactNode}) {
  return createElement(StoreHandle.Provider, {value: store}, createElement(StoreContext.Provider, {value: [state, store]}, children));
}

// useStore returns [state, store] for the state App last drew. A component
// that uses it redraws on every change of state.
export function useStore(): [State, Store] {
  const context = useContext(StoreContext);
  if (!context) throw new Error('useStore needs a StoreProvider.');
  return context;
}

// useStoreValue returns select's value of the state, and the store. A
// component that uses it redraws only when that value changes.
export function useStoreValue<T>(select: (state: State) => T): [T, Store] {
  const store = useContext(StoreHandle);
  if (!store) throw new Error('useStoreValue needs a StoreProvider.');
  return [useSyncExternalStore(store.subscribe, () => select(store.get())), store];
}
