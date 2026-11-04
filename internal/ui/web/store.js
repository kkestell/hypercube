// The one owner of the page's shared state. Changes go through update with a
// pure function of the state; App redraws from each new state.
import {createContext} from 'react';
import {useContext} from 'react';

export function createStore(initial) {
  let state = initial;
  const subscribers = new Set();
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

export const StoreContext = createContext(null);

// useStore returns [state, store] for the state App last drew.
export const useStore = () => useContext(StoreContext);
