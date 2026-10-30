import { useSyncExternalStore } from "react";
import type { AgentInfo, TranscriptItem } from "./model";

/** Each agent's transcript and panel info as the server last sent them. Read them from UI with `useTranscript` and `useAgentInfo`. */

const transcripts = new Map<string, TranscriptItem[]>();
const none: TranscriptItem[] = [];
const infos = new Map<string, AgentInfo>();
const noInfo: AgentInfo = {};
const listeners = new Set<() => void>();

function notify() {
  for (const listener of listeners) listener();
}

export function setTranscript(agentId: string, items: TranscriptItem[]) {
  transcripts.set(agentId, items);
  notify();
}

/** Replaces the item with the same id, or adds it at the end. */
export function upsertItem(agentId: string, item: TranscriptItem) {
  const items = transcripts.get(agentId) ?? none;
  const at = items.findLastIndex(existing => existing.id === item.id);
  transcripts.set(agentId, at === -1 ? [...items, item] : items.with(at, item));
  notify();
}

/** The server sends an agent's whole info, so this replaces it. */
export function setInfo(agentId: string, info: AgentInfo) {
  infos.set(agentId, info);
  notify();
}

/** Forgets everything, before the server sends the transcripts and info again on a new connection. */
export function clearTranscripts() {
  transcripts.clear();
  infos.clear();
  notify();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function useTranscript(agentId: string) {
  return useSyncExternalStore(subscribe, () => transcripts.get(agentId) ?? none);
}

export function useAgentInfo(agentId: string) {
  return useSyncExternalStore(subscribe, () => infos.get(agentId) ?? noInfo);
}
