// Turns a transcript's items, in seq order, into the rows the page draws.
//
// An item nests under its parent tool call when the parent is present.
// Tool calls within one container form a run, passing over user messages
// and closing at the next other item, which ends its duration. User
// messages draw in place, with later calls joining the run above them. A call
// that sends a message to a session draws as a message. The rows are rebuilt
// from the whole list, so an item that arrives before its parent moves under
// the parent when it arrives.

// Tool input fields, in the order that best says what a call does.
const summaryKeys = ['command', 'cmd', 'file_path', 'path', 'pattern', 'query', 'url', 'description', 'prompt'];

// toolSummary is the one line that tells a tool call apart: the input's most
// telling text, shortened.
export function toolSummary(input) {
  let text = '';
  if (typeof input === 'string') {
    text = input;
  } else if (input && typeof input === 'object') {
    const key = summaryKeys.find(k => typeof input[k] === 'string') ?? Object.keys(input).find(k => typeof input[k] === 'string');
    text = key ? input[key] : '';
  }
  text = text.replace(/\s+/g, ' ').trim();
  return text.length > 120 ? text.slice(0, 119) + '…' : text;
}

// Hypercube's own tools, as what each call does and the input that tells
// calls apart. send_message draws as a message instead.
const hypercubeTools = {
  create_session: input => ['Create session', input.name],
  end_session: input => ['End session', input.name],
  list_sessions: () => ['List sessions', ''],
  set_status: input => ['Set status', input.text],
};

// toolTitle is a tool call's name and summary.
export function toolTitle(item) {
  const input = item.input && typeof item.input === 'object' ? item.input : {};
  const named = item.server === 'hypercube' && hypercubeTools[item.tool];
  if (named) {
    const [name, detail] = named(input);
    return [name, toolSummary(typeof detail === 'string' ? detail : '')];
  }
  const name = item.server ? item.server + ' - ' + item.tool : item.tool ?? 'tool';
  return [name, toolSummary(item.input)];
}

// A tool's input and output show this many characters.
const shownLimit = 20000;
export const clip = text => text.length > shownLimit ? text.slice(0, shownLimit) + '\n… (' + (text.length - shownLimit) + ' more characters)' : text;

export const toolInput = input => input === undefined || input === null ? '' : clip(typeof input === 'string' ? input : JSON.stringify(input, null, 2));

const isMessage = item => item.kind === 'tool' && item.server === 'hypercube' && item.tool === 'send_message';
const isCall = item => item.kind === 'tool' && !isMessage(item);

const kinds = new Set(['reasoning', 'error', 'attention']);
const rowType = item => kinds.has(item.kind) ? item.kind : 'text';

const startOf = row => (row.type === 'run' ? row.calls[0].item : row.item).at ?? 0;

// transcriptRows returns the top-level rows of a transcript {order, items}:
//   {type: 'run', key, calls, closed, start, until}, whose calls are
//     {type: 'tool', item, children} or {type: 'message', item};
//   {type: 'reasoning', item, until};
//   {type: 'user' | 'text' | 'error' | 'attention', item}.
// start and until are millisecond times; until is 0 while a run is open. A run
// still open when the session is not working closes at its last call's end.
// Thinking ends at its item's end; one saved without an end ends where the
// next row other than a user message starts.
export function transcriptRows({order, items}, working = true) {
  const containers = new Map();
  for (const id of order) {
    const item = items[id];
    // Keep status updates in the transcript state without displaying them.
    if (item.kind === 'status') continue;
    const parent = items[item.parent];
    const container = parent && isCall(parent) ? parent.id : '';
    if (!containers.has(container)) containers.set(container, []);
    containers.get(container).push(item);
  }
  const rowsOf = container => {
    const rows = [];
    let run = null;
    for (const item of containers.get(container) ?? []) {
      if (item.kind === 'user') {
        rows.push({type: 'user', item});
        continue;
      }
      if (item.kind !== 'tool') {
        if (run) Object.assign(run, {closed: true, until: item.at ?? 0});
        run = null;
        rows.push({type: rowType(item), item});
        continue;
      }
      if (!run) {
        run = {type: 'run', key: 'run:' + item.id, calls: [], closed: false, start: item.at ?? 0, until: 0};
        rows.push(run);
      }
      run.calls.push(isMessage(item) ? {type: 'message', item} : {type: 'tool', item, children: rowsOf(item.id)});
    }
    rows.forEach((row, i) => {
      if (row.type !== 'reasoning') return;
      const next = rows.slice(i + 1).find(r => r.type !== 'user');
      row.until = row.item.end ?? (row.item.done && next ? startOf(next) : 0);
    });
    if (run && !working) Object.assign(run, {closed: true, until: Math.max(0, ...run.calls.map(call => call.item.end ?? 0))});
    return rows;
  };
  return rowsOf('');
}

// ancestorKeys are the expansion keys of the runs and tool calls around an
// item, outermost first, or [] if the item is not in the transcript.
export function ancestorKeys(transcript, id) {
  const search = (rows, path) => {
    for (const row of rows) {
      if (row.type !== 'run') {
        if (row.item.id === id) return path;
        continue;
      }
      for (const call of row.calls) {
        if (call.item.id === id) return [...path, row.key];
        const found = call.type === 'tool' && search(call.children, [...path, row.key, call.item.id]);
        if (found) return found;
      }
    }
    return null;
  };
  return search(transcriptRows(transcript), []) ?? [];
}
