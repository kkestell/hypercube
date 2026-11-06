// A session's transcript: the items its agent and Hypercube record, by kind.
import type {Settings} from '../agent/types.ts';
import type {Image} from '../images.ts';

// WorkState is the agent's work state that a state item records.
export type WorkState = 'working' | 'idle';

// A later item with the same ID replaces an earlier one. A native subagent's
// items name the tool item that started it as their parent. at is when
// Hypercube first received the item, in Unix milliseconds.
interface ItemBase {
  id: string;
  parent?: string;
  at?: number;
  done?: boolean;
}

// UserItem is a prompt, or a message from the session named in from.
export interface UserItem extends ItemBase {
  kind: 'user';
  text?: string;
  images?: Image[];
  from?: string;
  automatic?: boolean;
}

export interface ProposalItem extends ItemBase {
  kind: 'proposal';
  text?: string;
  input: {
    name: string;
    title: string;
    settings: Settings;
    dir?: string;
    message?: string;
    fork?: boolean;
  };
  answer?: string;
}

// StateItem records a work state change; an idle one's final names the items
// that answered the turn it ended.
export interface StateItem extends ItemBase {
  kind: 'state';
  state: WorkState;
  final?: string[];
}

export interface TextItem extends ItemBase {
  kind: 'text';
  text?: string;
  images?: Image[];
}

export interface ReasoningItem extends ItemBase {
  kind: 'reasoning';
  text?: string;
}

// ToolItem is a tool call. A call to an MCP server names the server, and tool
// is the server's own name for the tool. input is the call's JSON input.
export interface ToolItem extends ItemBase {
  kind: 'tool';
  server?: string;
  tool?: string;
  input?: unknown;
  output?: string;
  failed?: boolean;
  images?: Image[];
}

export interface ErrorItem extends ItemBase {
  kind: 'error';
  text?: string;
}

// AttentionItem asks the user: text is the prompt, choices the options,
// notes[i] describes choices[i], and free allows typed text. It stays open
// until done, with the answer when there is one. An approval names the tool it
// asks to use in grant.
export interface AttentionItem extends ItemBase {
  kind: 'attention';
  text?: string;
  choices?: string[];
  multi?: boolean;
  notes?: string[];
  free?: boolean;
  answer?: string;
  grant?: string;
}

export type Item = UserItem | StateItem | TextItem | ReasoningItem | ToolItem | ErrorItem | AttentionItem | ProposalItem;

// TranscriptItem is an item with its position in the transcript.
export type TranscriptItem = Item & {seq: number};

// Transcript holds one item per ID, with the IDs ordered by seq.
export interface Transcript {
  order: string[];
  items: Record<string, TranscriptItem>;
}
