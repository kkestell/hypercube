// Every HTTP call the page makes to the server.
import {showError} from './notifications/errors.js';

const url = (path, params) => path + '?' + new URLSearchParams(params);

// post sends an action with an optional body and returns the response text, or
// null after showing the user why it failed or the server refused.
export async function post(store, path, params, body) {
  try {
    const response = await fetch(url(path, params), {method: 'POST', body});
    const text = await response.text();
    if (!response.ok) {
      showError(store, text);
      return null;
    }
    return text;
  } catch (error) {
    showError(store, error.message);
    return null;
  }
}

// fetchProfiles returns the profiles, or null after showing why it failed.
export async function fetchProfiles(store) {
  try {
    const response = await fetch('profiles');
    if (!response.ok) {
      showError(store, await response.text());
      return null;
    }
    return await response.json();
  } catch (error) {
    showError(store, error.message);
    return null;
  }
}

// answerRequest answers a session's attention request and reports whether the
// server accepted the answer.
export async function answerRequest(store, session, request, choice, text) {
  return await post(store, 'session', {action: 'answer', id: session, request, choice}, text) !== null;
}

// listFiles returns a folder's entries, relative to the session's working
// directory. A failure throws an error carrying the response status.
export async function listFiles(session, dir) {
  const response = await fetch(url('files', {session, path: dir}), {cache: 'no-store'});
  if (!response.ok) {
    const error = new Error(await response.text());
    error.status = response.status;
    throw error;
  }
  return response.json();
}

// readFile reads a file, or reports that it is unchanged since the read that
// returned etag, deleted, or not shown, with VS Code's reasons.
export async function readFile(session, path, etag) {
  const headers = etag ? {'If-None-Match': etag} : {};
  const response = await fetch(url('file', {session, path}), {headers, cache: 'no-store'});
  if (response.status === 304) return {unchanged: true};
  if (response.status === 404) return {deleted: true};
  if (response.status === 413) return {message: 'The file is not displayed in the text editor because it is very large.'};
  if (response.status === 415) return {message: 'The file is not displayed in the text editor because it is either binary or uses an unsupported text encoding.'};
  if (!response.ok) return {error: await response.text()};
  return {text: await response.text(), etag: response.headers.get('ETag')};
}
