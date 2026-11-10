// One request per pull request row and action at a time: a row's button is
// disabled while its request runs.
import {useState} from 'react';

// useRowAction gives each row a handler that runs action for the row's key, or
// undefined while that key's action is running, which disables its button.
export function useRowAction() {
  const [running, setRunning] = useState<string[]>([]);
  return (key: string, action: () => Promise<void>) => {
    if (running.includes(key)) return undefined;
    return async () => {
      setRunning(keys => [...keys, key]);
      await action();
      setRunning(keys => keys.filter(other => other !== key));
    };
  };
}
