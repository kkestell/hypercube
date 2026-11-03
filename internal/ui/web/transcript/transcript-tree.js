// Turns a transcript's items, in seq order, into the rows the page draws.
//
// An item nests under its parent tool call when the parent is present. A call
// that sends a message to a session draws as a message. The rows are rebuilt
// from the whole list, so an item that arrives before its parent moves under
// the parent when it arrives.
//
// Hypercube saves a state item at each change of the agent's work state. The
// items between a working state and the next idle one are one turn's activity.
// Once the turn succeeds, the idle state names its final response: the input
// that began the turn stays above one activity row holding everything else, and
// the response follows it. A turn that did not succeed, and a transcript saved
// without state items, draw every row as it is.

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

const types = new Set(['user', 'reasoning', 'error', 'attention']);

// transcriptRows returns the rows of a transcript {order, items}:
//   {type: 'user' | 'reasoning' | 'text' | 'error' | 'attention' | 'message', item},
//     where a user row inside a turn's activity has inline set;
//   {type: 'tool', item, children}, whose children are rows;
//   {type: 'activity', key, rows, start, until}, a finished turn's activity,
//     whose start and until are millisecond times, 0 when unknown.
export function transcriptRows({order, items}) {
  const containers = new Map();
  for (const id of order) {
    const item = items[id];
    // Keep status updates in the transcript state without displaying them.
    if (item.kind === 'status') continue;
    const parent = items[item.parent];
    const container = parent && parent.kind === 'tool' && !isMessage(parent) ? parent.id : '';
    if (!containers.has(container)) containers.set(container, []);
    containers.get(container).push(item);
  }
  const rowOf = item => {
    if (item.kind === 'tool') {
      return isMessage(item) ? {type: 'message', item} : {type: 'tool', item, children: (containers.get(item.id) ?? []).map(rowOf)};
    }
    return {type: types.has(item.kind) ? item.kind : 'text', item};
  };
  return turns(containers.get('') ?? [], rowOf);
}

// Items of different launches of an agent have different ID prefixes, which
// Hypercube sets and ends with a colon.
const launchOf = item => item.id.slice(0, item.id.indexOf(':'));

// turns draws the top-level items, grouping the activity of each turn that a
// state item shows to have succeeded.
function turns(items, rowOf) {
  const out = [];
  let turn = null; // {launch, work, opener, rows}: the turn in progress
  let floor = 0; // the input that begins a turn is not above this row
  const end = () => {
    if (turn) out.push(...turn.rows);
    turn = null;
    floor = out.length;
  };
  for (const item of items) {
    if (item.kind !== 'state') {
      const row = rowOf(item);
      if (turn) turn.rows.push(inline(row));
      else out.push(row);
      continue;
    }
    if (turn && turn.launch !== launchOf(item)) end();
    if (item.state === 'working') {
      if (!turn) turn = startTurn(item, out, floor);
    } else if (turn) {
      finishTurn(turn, item, out);
      turn = null;
      floor = out.length;
    }
  }
  end();
  return out;
}

// A user row inside a turn is shown with its text, as the opener is not.
const inline = row => (row.type === 'user' ? Object.assign(row, {inline: true}) : row);

// startTurn begins a turn at a working state. Its opener is the first of the
// user messages just before the state, which stays in out; later ones were sent
// while the turn began.
function startTurn(work, out, floor) {
  let first = out.length;
  while (first > floor && out[first - 1].type === 'user') first--;
  return {launch: launchOf(work), work, opener: out[first], rows: out.splice(first + 1).map(inline)};
}

// finishTurn draws a turn that an idle state ended. The rows its final names
// follow one row of the other activity.
function finishTurn(turn, idle, out) {
  const final = new Set(idle.final);
  const response = turn.rows.filter(row => final.has(row.item.id));
  if (response.length === 0) {
    out.push(...turn.rows);
    return;
  }
  const activity = turn.rows.filter(row => !final.has(row.item.id));
  if (activity.length > 0) {
    const start = turn.opener?.item.at || turn.work.at || 0;
    out.push({type: 'activity', key: 'activity:' + turn.work.id, rows: activity, start, until: idle.at || 0});
  }
  out.push(...response);
}

// ancestorKeys are the expansion keys of the activity around an item, or []
// if the item is not inside one.
export function ancestorKeys(transcript, id) {
  const holds = rows => rows.some(row => row.item.id === id || row.children && holds(row.children));
  const activity = transcriptRows(transcript).find(row => row.type === 'activity' && holds(row.rows));
  return activity ? [activity.key] : [];
}
