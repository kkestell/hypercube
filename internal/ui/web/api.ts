// Every HTTP call the page makes to the server.
import type {Profile} from './agent/types.ts';
import type {Grant} from './app/types.ts';
import type {ChangesResult, DiffRead} from './changes/types.ts';
import type {FileEntry, FileRead, SearchFile} from './files/types.ts';
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

// fetchProfiles returns the profiles, or null after showing why it failed.
export const fetchProfiles = (store: Store) =>
  request<Profile[]>(store, 'profiles', undefined, response => response.json());

export const fetchGrants = (store: Store) =>
  request<Grant[]>(store, 'grants', undefined, response => response.json());

export const fetchVersion = (store: Store) =>
  request<string>(store, 'version', {cache: 'no-store'}, async response => (await response.text()).trim());

// selectProfile applies a configured profile to a ready session.
export const selectProfile = (store: Store, session: string, profile: string) =>
  post(store, 'session', {action: 'profile', id: session, profile});

// answerRequest answers a session's attention request and reports whether the
// server accepted the answer.
export async function answerRequest(store: Store, session: string, request: string, choice: string, text?: string): Promise<boolean> {
  return await post(store, 'session', {action: 'answer', id: session, request, choice}, text) !== null;
}

// alwaysAllow answers a session's approval request with its first choice and
// always allows its tool in the session, or in all sessions when all is set. It
// reports whether the server accepted it.
export async function alwaysAllow(store: Store, session: string, request: string, all: boolean): Promise<boolean> {
  return await post(store, 'session', {action: 'allow', id: session, request, scope: all ? 'all' : 'session'}) !== null;
}

// listFiles returns a folder's entries, relative to the session's working
// directory.
export const listFiles = (session: string, dir: string) => get<FileEntry[]>('files', {session, path: dir});

// mentionFiles lists paths in the selected session or unsent workspace.
export const mentionFiles = (params: {session: string} | {workspace: string}, signal: AbortSignal) =>
  get<string[]>('mention-files', params, signal);

export const searchFiles = (session: string, query: string, signal: AbortSignal) =>
  get<SearchFile[]>('search-files', {session, q: query}, signal);

// searchSessions finds session titles and conversation text across workspaces.
// The server sends null when nothing matches.
export const searchSessions = (query: string, signal: AbortSignal): Promise<SessionSearchResult[]> =>
  get<SessionSearchResult[] | null>('search-sessions', {q: query}, signal).then(results => results ?? []);

// fetchChanges lists the changes within the session's folder, or
// returns null for a folder outside a repository.
export const fetchChanges = (session: string) => get<ChangesResult | null>('changes', {session});

// readFile reads text or browser preview metadata, or reports an unchanged,
// deleted, or unsupported file.
export async function readFile(session: string, path: string, etag?: string | null): Promise<FileRead> {
  const headers: Record<string, string> = etag ? {'If-None-Match': etag} : {};
  const response = await fetch(url('file', {session, path}), {headers, cache: 'no-store'});
  if (response.status === 304) return {unchanged: true};
  if (response.status === 404) return {deleted: true};
  if (response.status === 413) return {message: tooLarge};
  if (response.status === 415) return {message: unsupported};
  if (!response.ok) return {error: await response.text()};
  if (response.headers.get('Content-Type') === 'application/json') return {...await response.json(), etag: response.headers.get('ETag')};
  return {text: await response.text(), etag: response.headers.get('ETag')};
}

// readDiff reads a whole-file patch, with the text viewer's size and binary messages.
export async function readDiff(session: string, path: string): Promise<DiffRead> {
  const response = await fetch(url('diff', {session, path}), {cache: 'no-store'});
  if (response.status === 413) return {message: tooLarge};
  if (response.status === 415) return {message: unsupported};
  if (!response.ok) return {error: await response.text()};
  return {text: await response.text()};
}
