// Every HTTP call the page makes to the server.
import {showError} from './notifications/errors.js';

const url = (path, params) => path + '?' + new URLSearchParams(params);

// attempt fetches, or returns null after showing why the request failed.
async function attempt(store, input, init) {
  try {
    return await fetch(input, init);
  } catch (error) {
    showError(store, error.message);
    return null;
  }
}

// post sends an action with an optional body and returns the response text, or
// null after showing the user why it failed or the server refused.
export async function post(store, path, params, body) {
  const response = await attempt(store, url(path, params), {method: 'POST', body});
  if (!response) return null;
  const text = await response.text();
  if (!response.ok) {
    showError(store, text);
    return null;
  }
  return text;
}

// fetchLeads returns the lead profiles, or null after showing why it failed.
export async function fetchLeads(store) {
  const response = await attempt(store, 'leads');
  if (!response) return null;
  if (!response.ok) {
    showError(store, await response.text());
    return null;
  }
  return response.json();
}

// answerRequest answers an agent's attention request and reports whether the
// server accepted the answer.
export async function answerRequest(store, agent, request, choice, text) {
  return await post(store, 'agent', {action: 'answer', id: agent, request, choice}, text) !== null;
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
