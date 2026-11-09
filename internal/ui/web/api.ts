// Every HTTP call the page makes to the server.
import type {EditorTarget} from './editor/target.ts';
import type {Harness} from './agent/types.ts';
import type {Config, Grant} from './app/types.ts';
import type {ChangeComparison, ChangesResult, DiffRead} from './changes/types.ts';
import type {FileRead, FolderListing, SearchFile} from './files/types.ts';
import {showError} from './notifications/errors.ts';
import type {SessionSearchResult} from './sessions/types.ts';
import type {Store} from './store.ts';

// HttpError is a refused request, with the response's status.
export class HttpError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

const url = (path: string, params: Record<string, string>) => path + '?' + new URLSearchParams(params);

const tooLarge = 'The file is not displayed in the text editor because it is very large.';
const unsupported = 'The file is not displayed in the text editor because it is either binary or uses an unsupported text encoding.';

// get fetches a JSON response without caching, throwing an HttpError when the
// server refuses.
async function get<T>(path: string, params: Record<string, string>, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url(path, params), {signal, cache: 'no-store'});
  if (!response.ok) throw new HttpError(await response.text(), response.status);
  return response.json();
}

// request fetches path and returns read(response), or null after showing the
// user why the request failed or the server refused it.
async function request<T>(store: Store, path: string, init: RequestInit | undefined, read: (response: Response) => Promise<T>): Promise<T | null> {
  try {
    const response = await fetch(path, init);
    if (!response.ok) {
      showError(store, await response.text());
      return null;
    }
    return await read(response);
  } catch (error) {
    showError(store, error instanceof Error ? error.message : String(error));
    return null;
  }
}

// post sends an action with an optional body and returns the response text,
// or null after showing why it failed.
export const post = (store: Store, path: string, params: Record<string, string>, body?: string) =>
  request(store, url(path, params), {method: 'POST', body}, response => response.text());

// Prompt failures belong beside their retained message. Creation can fail
// after persisting a session, in which case its ID still belongs to the page.
export async function submitPrompt(params: Record<string, string>, body: string): Promise<{session?: string; error?: string}> {
  const unconfirmed = ' Delivery was not confirmed; retrying may send this message again.';
  try {
    const response = await fetch(url('session', params), {method: 'POST', body});
    if (response.headers.get('Content-Type')?.includes('application/json')) return await response.json();
    const text = await response.text();
    return response.ok ? {} : {error: text || 'Sending failed.'};
  } catch (error) {
    return {error: (error instanceof Error ? error.message : String(error)) + unconfirmed};
  }
}

// cloneWorkspace asks the server to clone a pull request's repository into a
// folder and add it. Aborting signal cancels the clone. A refusal comes back as
// its message, not a toast, so the clone dialog can show it.
export async function cloneWorkspace(params: {repo: string; account: string; path: string}, signal: AbortSignal): Promise<{workspace: string} | {error: string}> {
  try {
    const response = await fetch(url('workspace', {action: 'clone', ...params}), {method: 'POST', signal});
    const text = await response.text();
    return response.ok ? {workspace: text} : {error: text || 'Cloning failed.'};
  } catch (error) {
    return {error: error instanceof Error ? error.message : String(error)};
  }
}

export const fetchConfig = (store: Store) =>
  request<Config>(store, 'config', {cache: 'no-store'}, response => response.json());

// saveConfig throws so the modal can show a failed save beside its draft.
export async function saveConfig(config: Config): Promise<void> {
  const response = await fetch('config', {method: 'POST', body: JSON.stringify(config)});
  if (!response.ok) throw new HttpError(await response.text(), response.status);
}

export const fetchGrants = (store: Store) =>
  request<Grant[]>(store, 'grants', undefined, response => response.json());

// selectModel changes a ready session's model and effort within its harness.
export const selectModel = (store: Store, session: string, model: string, effort: string) =>
  post(store, 'session', {action: 'selection', id: session, model, effort});

// fetchHarnesses lists the installed harnesses, scanning for new ones.
export const fetchHarnesses = (signal: AbortSignal) => get<Harness[]>('harnesses', {}, signal);

// answerRequest answers a session's attention request and reports whether the
// server accepted the answer.
export async function answerRequest(store: Store, session: string, requestID: string, choices: string[], text?: string): Promise<boolean> {
  const params = new URLSearchParams({action: 'answer', id: session, request: requestID});
  for (const choice of choices.length ? choices : ['']) params.append('choice', choice);
  return await request(store, `session?${params}`, {method: 'POST', body: text}, response => response.text()) !== null;
}

// alwaysAllow answers a session's approval request with its first choice and
// always allows its tool in the session, or in all sessions when all is set. It
// reports whether the server accepted it.
export async function alwaysAllow(store: Store, session: string, request: string, all: boolean): Promise<boolean> {
  return await post(store, 'session', {action: 'allow', id: session, request, scope: all ? 'all' : 'session'}) !== null;
}

// listFileTree returns each requested folder's entries or read failure.
export async function listFileTree(target: EditorTarget, dirs: string[]): Promise<Record<string, FolderListing>> {
  const response = await fetch(url('files/tree', target), {method: 'POST', body: JSON.stringify(dirs), cache: 'no-store'});
  if (!response.ok) throw new HttpError(await response.text(), response.status);
  return response.json();
}

// mentionFiles lists paths in the selected session or unsent workspace.
export const mentionFiles = (params: {session: string} | {workspace: string}, signal: AbortSignal) =>
  get<string[]>('mention-files', params, signal);

export const searchFiles = (target: EditorTarget, query: string, signal: AbortSignal) =>
  get<SearchFile[]>('search-files', {...target, q: query}, signal);

// searchSessions finds session titles and conversation text across workspaces.
// The server sends null when nothing matches.
export const searchSessions = (query: string, signal: AbortSignal): Promise<SessionSearchResult[]> =>
  get<SessionSearchResult[] | null>('search-sessions', {q: query}, signal).then(results => results ?? []);

// fetchChanges lists the changes within the session's folder, or
// returns null for a folder outside a repository.
export const fetchChanges = (target: EditorTarget, comparison: ChangeComparison = 'uncommitted') => get<ChangesResult | null>('changes', comparison === 'branch' ? {...target, comparison} : target);

// readFile reads text or browser preview metadata, or reports an unchanged,
// deleted, or unsupported file.
export async function readFile(target: EditorTarget, path: string, etag?: string | null): Promise<FileRead> {
  const headers: Record<string, string> = etag ? {'If-None-Match': etag} : {};
  const response = await fetch(url('file', {...target, path}), {headers, cache: 'no-store'});
  if (response.status === 304) return {unchanged: true};
  if (response.status === 404) return {deleted: true};
  if (response.status === 413) return {message: tooLarge};
  if (response.status === 415) return {message: unsupported};
  if (!response.ok) return {error: await response.text()};
  if (response.headers.get('Content-Type') === 'application/json') return {...await response.json(), etag: response.headers.get('ETag')};
  return {text: await response.text(), etag: response.headers.get('ETag')};
}

// readDiff reads a whole-file patch, with the text viewer's size and binary messages.
export async function readDiff(target: EditorTarget, path: string, comparison: ChangeComparison = 'uncommitted'): Promise<DiffRead> {
  const response = await fetch(url('diff', {...target, path, ...(comparison === 'branch' ? {comparison} : {})}), {cache: 'no-store'});
  if (response.status === 413) return {message: tooLarge};
  if (response.status === 415) return {message: unsupported};
  if (!response.ok) return {error: await response.text()};
  return {text: await response.text()};
}
