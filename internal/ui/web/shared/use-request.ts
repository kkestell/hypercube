// useRequest loads data for a component while it is shown. It runs load again
// whenever deps change, aborting the request it replaces, and clears its
// result when load is null.
import {useEffect, useState, type DependencyList} from 'react';
import {errorText} from './error-text.ts';

export interface Request<T> {
  data?: T;
  error: string;
  loading: boolean;
}

export function useRequest<T>(load: ((signal: AbortSignal) => Promise<T>) | null, deps: DependencyList): Request<T> {
  const [request, setRequest] = useState<Request<T>>({error: '', loading: false});
  useEffect(() => {
    if (!load) {
      setRequest({error: '', loading: false});
      return;
    }
    const controller = new AbortController();
    setRequest({error: '', loading: true});
    load(controller.signal).then(
      data => { if (!controller.signal.aborted) setRequest({data, error: '', loading: false}); },
      (error: unknown) => { if (!controller.signal.aborted) setRequest({error: errorText(error), loading: false}); });
    return () => controller.abort();
  }, deps);
  return request;
}
