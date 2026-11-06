// Shared model catalogs survive picker and session changes for this page.
// Labels read persisted choices; picker actions can refresh or retry them.
import {useEffect} from 'react';
import {fetchModels} from '../api.ts';
import type {State} from '../state.ts';
import {useStore, type Store} from '../store.ts';
import type {Catalog} from './types.ts';

export interface CatalogKey {
  workspace: string;
  harness: string;
  model: string;
}

export interface CatalogEntry {
  value?: Catalog;
  loading: false | 'cached' | 'refresh';
  error?: string;
}

export const catalogKey = ({workspace, harness, model}: CatalogKey) => JSON.stringify([workspace, harness, model]);
export const catalogOf = (state: State, key: CatalogKey) => state.catalogs[catalogKey(key)];

const withCatalog = (state: State, key: CatalogKey, entry: CatalogEntry): State =>
  ({...state, catalogs: {...state.catalogs, [catalogKey(key)]: entry}});

export async function loadCatalog(store: Store, key: CatalogKey, refresh = false): Promise<void> {
  const entry = catalogOf(store.get(), key);
  if (entry?.loading || (!refresh && entry)) return;
  store.update(state => withCatalog(state, key, {value: entry?.value, loading: refresh ? 'refresh' : 'cached'}));
  try {
    const value = await fetchModels(key.workspace, key.harness, key.model, refresh);
    store.update(state => withCatalog(state, key, {value, loading: false}));
  } catch (error) {
    store.update(state => withCatalog(state, key, {
      value: entry?.value, loading: false, error: error instanceof Error ? error.message : String(error),
    }));
  }
}

// Labels ask once for a workspace's saved models, even when several mount together.
export function useModels(workspace: string | null | undefined, harness: string | undefined) {
  const [state, store] = useStore();
  useEffect(() => {
    if (workspace && harness) void loadCatalog(store, {workspace, harness, model: ''});
  }, [store, workspace, harness]);
  return workspace && harness ? catalogOf(state, {workspace, harness, model: ''})?.value?.models ?? [] : [];
}
