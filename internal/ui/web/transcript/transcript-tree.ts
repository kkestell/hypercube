// Turns a transcript's items, in seq order, into the rows the page draws.
//
// An item nests under its parent tool call when the parent is present. A call
// that sends a message to a session draws as a message. The rows are rebuilt
// from the whole list, so an item that arrives before its parent moves under
// the parent when it arrives. Adjacent reasoning items draw as one row.
//
// Hypercube saves a state item at each change of the agent's work state. The
// items between a working state and the next idle one are one turn's activity.
// Once a turn succeeds, the idle state names its final response, which follows
// the turn's activity. The input that began the turn stays above it, and each
// message that arrived during the turn stays in place; the agent's work between
// them draws as one activity row. A running turn, a turn that did not succeed,
// and a transcript saved without state items draw every row as it is.

import type {AttentionItem, ProposalItem, ErrorItem, Item, ReasoningItem, StateItem, TextItem, ToolItem, Transcript, UserItem} from './types.ts';

// A row is what the page draws for an item.
export type ItemRow =
  | {type: 'user'; item: UserItem}
  | ReasoningRow
  | {type: 'text'; item: TextItem}
  | {type: 'error'; item: ErrorItem}
  | {type: 'attention'; item: AttentionItem}
  | {type: 'message'; item: ToolItem}
  | {type: 'proposal'; item: ProposalItem};

// ReasoningRow is a run of adjacent reasoning items, named by the first.
export interface ReasoningRow {
  type: 'reasoning';
  item: ReasoningItem;
  items: ReasoningItem[];
}

// ToolRow is a tool call with the rows nested under it.
export interface ToolRow {
  type: 'tool';
  item: ToolItem;
  children: TurnRow[];
}

export type TurnRow = ItemRow | ToolRow;

// ActivityRow is the agent's work between two messages of a finished turn.
// start and until are millisecond times, 0 when unknown.
export interface ActivityRow {
  type: 'activity';
  key: string;
  rows: TurnRow[];
  start: number;
  until: number;
}

export type Row = TurnRow | ActivityRow;

const isMessage = (item: ToolItem): boolean => item.server === 'hypercube' && item.tool === 'send_message';

// Some harnesses send a text copy of a question and its options immediately
// before its structured form. The form owns that content, even while pending.
function repeatsQuestions(item: Item, index: number, {order, items}: Transcript): boolean {
  if (item.kind !== 'text' || !item.text || item.images?.length) return false;
  const questions: string[] = [];
  for (let i = index + 1; i < order.length; i++) {
    const next = items[order[i]];
    if (next.kind !== 'attention' || next.parent !== item.parent) break;
    questions.push([next.text ?? '', ...(next.choices ?? []).map(choice => '- ' + choice)].join('\n'));
    if (item.text.trim() === questions.join('\n\n').trim()) return true;
  }
  return false;
}

// isPending reports an open question, approval, or proposed child session,
// which waits on the user.
export const isPending = <T extends Item>(item: T): item is Extract<T, {kind: 'attention' | 'proposal'}> => (item.kind === 'attention' || item.kind === 'proposal') && !item.done;

// isQuestion reports an attention item that is neither an approval nor a task
// offer.
export const isQuestion = (item: Item): boolean => item.kind === 'attention' && !item.grant && !item.offer;

// pendingGroups groups the pending items the card shows together. Adjacent
// questions, asked together, form one group; every other item is its own.
export function pendingGroups<T extends Item>(pending: T[]): T[][] {
  const groups: T[][] = [];
  for (const item of pending) {
    const group = groups.at(-1);
    if (group && isQuestion(item) && isQuestion(group[0])) group.push(item);
    else groups.push([item]);
  }
  return groups;
}

// transcriptRows returns the rows of a transcript.
export function transcriptRows(transcript: Transcript): Row[] {
  const {order, items} = transcript;
  const containers = new Map<string, Item[]>();
  // Each run of reasoning, under the ID of its first item.
  const reasoning = new Map<string, ReasoningItem[]>();
  for (let i = 0; i < order.length; i++) {
    const item = items[order[i]];
    // Harnesses may record reasoning without exposing its text, or empty text
    // placeholders. Neither should create an empty activity disclosure.
    if ((item.kind === 'reasoning' || item.kind === 'text') && !item.text?.trim() && !(item.kind === 'text' && item.images?.length)) continue;
    if (repeatsQuestions(item, i, transcript)) continue;
    // Open requests and proposals are pinned below the scrolling history.
    if (isPending(item)) continue;
    const parent = item.parent === undefined ? undefined : items[item.parent];
    const container = parent?.kind === 'tool' && !isMessage(parent) ? parent.id : '';
    const siblings = containers.get(container) ?? [];
    containers.set(container, siblings);
    const last = siblings.at(-1);
    if (item.kind === 'reasoning' && last?.kind === 'reasoning') {
      reasoning.get(last.id)?.push(item);
      continue;
    }
    if (item.kind === 'reasoning') reasoning.set(item.id, [item]);
    siblings.push(item);
  }
  const rowOf = (item: Exclude<Item, StateItem>): TurnRow => {
    switch (item.kind) {
      case 'tool': {
        if (isMessage(item)) return {type: 'message', item};
        const children = (containers.get(item.id) ?? []).filter(child => child.kind !== 'state').map(rowOf);
        return {type: 'tool', item, children};
      }
      case 'user': return {type: 'user', item};
      case 'reasoning': return {type: 'reasoning', item, items: reasoning.get(item.id) ?? [item]};
      case 'error': return {type: 'error', item};
      case 'attention': return {type: 'attention', item};
      case 'proposal': return {type: 'proposal', item};
      case 'text': return {type: 'text', item};
    }
  };
  return turns(containers.get('') ?? [], rowOf);
}

// Items of different launches of an agent have different ID prefixes, which
// Hypercube sets and ends with a colon.
const launchOf = (item: Item): string => item.id.slice(0, item.id.indexOf(':'));

// A Turn is the turn in progress: its launch, its working state, when the user
// row that opened it came, and its rows so far.
interface Turn {
  launch: string;
  work: StateItem;
  start: number;
  rows: TurnRow[];
}

// turns draws the top-level items, grouping the activity of each turn that a
// state item shows to have succeeded.
function turns(items: Item[], rowOf: (item: Exclude<Item, StateItem>) => TurnRow): Row[] {
  const out: Row[] = [];
  let turn: Turn | null = null;
  let floor = 0; // the input that begins a turn is not above this row
  const end = () => {
    if (turn) out.push(...turn.rows);
    turn = null;
    floor = out.length;
  };
  for (const item of items) {
    if (item.kind !== 'state') {
      const row = rowOf(item);
      if (turn) turn.rows.push(row);
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

// startTurn begins a turn at a working state. Its opener is the first of the
// user messages just before the state, which stays in out; later ones were sent
// while the turn began.
function startTurn(work: StateItem, out: Row[], floor: number): Turn {
  let first = out.length;
  while (first > floor && out[first - 1].type === 'user') first--;
  const opener = out[first];
  const openedAt = opener?.type === 'user' ? opener.item.at : undefined;
  return {launch: launchOf(work), work, start: openedAt || work.at || 0, rows: out.splice(first + 1).filter(isTurnRow)};
}

const isTurnRow = (row: Row): row is TurnRow => row.type !== 'activity';

// finishTurn draws a turn that an idle state ended. The rows its final names
// follow the rest of its activity.
function finishTurn(turn: Turn, idle: StateItem, out: Row[]) {
  const final = new Set(idle.final);
  const response = turn.rows.filter(row => final.has(row.item.id) || row.type === 'proposal');
  if (response.length === 0) {
    out.push(...turn.rows);
    return;
  }
  const rest = turn.rows.filter(row => !final.has(row.item.id) && row.type !== 'proposal');
  out.push(...activity(turn, rest, idle.at || 0), ...response);
}

// activity draws a turn's rows, which ended at until: its messages in place,
// and the work between them as activity rows, each keyed by the turn or the
// message that begins it.
function activity(turn: Turn, rows: TurnRow[], until: number): Row[] {
  const out: Row[] = [];
  let key = turn.work.id, start = turn.start, work: TurnRow[] = [];
  const close = (end: number) => {
    if (work.length > 0) out.push({type: 'activity', key: 'activity:' + key, rows: work, start, until: end});
  };
  for (const row of rows) {
    if (row.type !== 'user') {
      work.push(row);
      continue;
    }
    close(row.item.at || 0);
    out.push(row);
    key = row.item.id;
    start = row.item.at || 0;
    work = [];
  }
  close(until);
  return out;
}
