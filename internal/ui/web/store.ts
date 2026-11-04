// The one owner of the page's shared state. Changes go through update with a
// pure function of the state; App redraws from each new state.
import {createContext, useContext} from 'react';
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

export const StoreContext = createContext<[State, Store] | null>(null);

// useStore returns [state, store] for the state App last drew.
export function useStore(): [State, Store] {
  const context = useContext(StoreContext);
  if (!context) throw new Error('useStore needs a StoreContext provider.');
  return context;
}
