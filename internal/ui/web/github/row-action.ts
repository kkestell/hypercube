// One request per pull request row and action at a time: a row's button is
// disabled while its request runs.
import {useState} from 'react';

// useRowAction tracks running actions by row key. run gives a handler that
// runs action for the key, or undefined while that key's action is running,
// which disables its button; running says whether it is.
export function useRowAction() {
  const [keys, setKeys] = useState<string[]>([]);
  const running = (key: string) => keys.includes(key);
  const run = (key: string, action: () => Promise<void>) => running(key) ? undefined : async () => {
    setKeys(current => [...current, key]);
    await action();
    setKeys(current => current.filter(other => other !== key));
  };
  return {run, running};
}
