import { Terminal as HeadlessTerminal } from "@xterm/headless";
import { SerializeAddon } from "@xterm/addon-serialize";
import { SCROLLBACK } from "../protocol";
import type { PtyProcess } from "./localPty";

/**
 * Server-side terminal registry. A terminal is created by an explicit request
 * and lives until it is explicitly closed, its shell exits, or the server shuts
 * down; no socket disconnect ends one. Each terminal pairs a PTY with a
 * headless xterm that mirrors its screen, so a client attaching later gets a
 * snapshot. Which processes a PTY owns, and how they are stopped, is the PTY's
 * business (`PtyProcess`).
 *
 * Output is published from the headless terminal's write callbacks, so it goes
 * out in the order it is parsed. Snapshots and resizes are queued the same
 * way, which puts each of them at an exact point in the output stream.
 *
 * Registry state lives on globalThis, and everything a process or the server
 * process triggers goes through `registry.ops`, so a `bun --hot` reload keeps
 * the running processes and routes them to the reloaded code.
 */

const FLUSH_INTERVAL_MS = 16;
/**
 * How long the PTY stays one row off its real size when an attach forces a
 * redraw. Apps read the size when they handle SIGWINCH, so two resizes in quick
 * succession look like no change at all; Claude Code and OpenCode both redraw
 * with a 10ms gap and neither does with none.
 */
const REDRAW_GAP_MS = 50;

export type TerminalEntry = {
  id: string;
  pty: PtyProcess;
  headless: HeadlessTerminal;
  serializer: SerializeAddon;
  /** The size most recently requested. The headless terminal catches up through its write queue. */
  cols: number;
  rows: number;
  state: "running" | "exited";
  /** Output received but not yet passed to the headless terminal. */
  pending: Uint8Array[];
  flushTimer: ReturnType<typeof setTimeout> | null;
  /** Set while the PTY is off its real size to force a redraw. */
  redrawTimer: ReturnType<typeof setTimeout> | null;
  /** Stopping the PTY's processes, started when the shell exits or the terminal is closed. */
  reaping: Promise<void> | null;
};

/** Where the registry reports what happens to terminals. The socket layer provides these. */
export type Hooks = {
  /** A batch of output, already applied to the headless terminal. */
  output(entry: TerminalEntry, chunk: Uint8Array): void;
  /** A terminal was closed or ended, and is forgotten. */
  removed(id: string): void;
};

/** Where the registry reports to whatever owns the terminals (the session model). */
export type OwnerHooks = {
  /** A terminal's shell exited by itself, its output has been published and nothing it started is left. It is forgotten. */
  ended(id: string): void;
};

type Ops = {
  onData(entry: TerminalEntry, data: Uint8Array): void;
  onExit(entry: TerminalEntry): void;
  shutdown(): Promise<void>;
  /** Synchronous last resort when the server process exits. */
  killAllNow(): void;
};

type Registry = {
  terminals: Map<string, TerminalEntry>;
  /** Terminals that are no longer registered but whose processes are still being stopped. */
  closing: Set<TerminalEntry>;
  hooks: Hooks;
  owner: OwnerHooks;
  ops: Ops;
  processHandlersInstalled: boolean;
};

const noop = () => {};
const globals = globalThis as typeof globalThis & { __hypercubeTerminals?: Registry };
const registry: Registry = (globals.__hypercubeTerminals ??= {
  terminals: new Map(),
  closing: new Set(),
  hooks: { output: noop, removed: noop },
  owner: { ended: noop },
  ops: undefined as unknown as Ops,
  processHandlersInstalled: false,
});

export function setHooks(hooks: Hooks) {
  registry.hooks = hooks;
}

export function setOwnerHooks(owner: OwnerHooks) {
  registry.owner = owner;
}

export function getTerminal(id: string) {
  return registry.terminals.get(id);
}

export function listTerminals() {
  return [...registry.terminals.values()];
}

/**
 * Registers a terminal around the PTY that `spawn` starts. `spawn` is given the
 * output callback and throws if the process can't be started; nothing is
 * registered then.
 */
export function createTerminal(id: string, cols: number, rows: number, spawn: (onData: (data: Uint8Array) => void) => PtyProcess) {
  if (registry.terminals.has(id)) throw new Error(`Terminal ${id} already exists`);

  const headless = new HeadlessTerminal({ cols, rows, scrollback: SCROLLBACK, allowProposedApi: true });
  const serializer = new SerializeAddon();
  headless.loadAddon(serializer as never);

  const entry = { id, headless, serializer, cols, rows, state: "running", pending: [], flushTimer: null, redrawTimer: null, reaping: null } as unknown as TerminalEntry;
  try {
    entry.pty = spawn(data => registry.ops.onData(entry, data));
  } catch (error) {
    headless.dispose();
    throw error;
  }
  void entry.pty.exited.then(() => registry.ops.onExit(entry));

  registry.terminals.set(id, entry);
  return entry;
}

export function writeInput(id: string, data: string | Uint8Array) {
  const entry = registry.terminals.get(id);
  if (entry?.state === "running") entry.pty.write(data);
}

export function resizeTerminal(id: string, cols: number, rows: number) {
  const entry = registry.terminals.get(id);
  if (entry) resize(entry, cols, rows);
}

/**
 * Output written before a resize was produced at the old size, so the headless
 * terminal is resized through its write queue, after everything already in it.
 */
function resize(entry: TerminalEntry, cols: number, rows: number) {
  if (entry.cols === cols && entry.rows === rows) return;
  flush(entry);
  entry.cols = cols;
  entry.rows = rows;
  if (entry.state === "running") entry.pty.resize(cols, rows);
  entry.headless.write("", () => entry.headless.resize(cols, rows));
}

/**
 * Resizes the terminal if `size` is given, then serializes the screen and
 * scrollback at that size and calls `deliver` with it. `deliver` runs at an
 * exact point in the output stream: everything published before it is in the
 * snapshot and everything published after it is not, so it must send the
 * snapshot and subscribe the client in the same synchronous step.
 *
 * The snapshot can't capture every bit of terminal state, so afterwards the
 * app is made to redraw.
 */
export function attachTerminal(
  entry: TerminalEntry,
  size: { cols: number; rows: number } | null,
  deliver: (snapshot: string) => void,
) {
  if (size) resize(entry, size.cols, size.rows);
  else flush(entry);
  entry.headless.write("", () => {
    if (!isLive(entry)) return;
    deliver(entry.serializer.serialize());
    forceRedraw(entry);
  });
}

/**
 * Makes the app redraw by taking the PTY one row off its size and back, which
 * sends it SIGWINCH twice. Only the PTY changes: what the app draws in between
 * is parsed at the real size and painted over by the redraw. The restore reads
 * the size when it runs, so a real resize in between wins.
 */
function forceRedraw(entry: TerminalEntry) {
  if (entry.state !== "running" || entry.redrawTimer) return;
  entry.pty.resize(entry.cols, entry.rows > 1 ? entry.rows - 1 : entry.rows + 1);
  entry.redrawTimer = setTimeout(() => {
    entry.redrawTimer = null;
    if (entry.state === "running") entry.pty.resize(entry.cols, entry.rows);
  }, REDRAW_GAP_MS);
}

function isLive(entry: TerminalEntry) {
  return registry.terminals.get(entry.id) === entry;
}

function scheduleFlush(entry: TerminalEntry) {
  entry.flushTimer ??= setTimeout(() => flush(entry), FLUSH_INTERVAL_MS);
}

/** Passes pending output to the headless terminal, which publishes it once parsed. */
function flush(entry: TerminalEntry) {
  if (entry.flushTimer) clearTimeout(entry.flushTimer);
  entry.flushTimer = null;
  if (entry.pending.length === 0) return;

  const chunk = concat(entry.pending);
  entry.pending = [];
  entry.headless.write(chunk, () => {
    // A closed terminal's id may already belong to a new one.
    if (isLive(entry)) registry.hooks.output(entry, chunk);
  });
}

function concat(chunks: Uint8Array[]) {
  if (chunks.length === 1) return chunks[0]!;
  const out = new Uint8Array(chunks.reduce((total, chunk) => total + chunk.length, 0));
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
  }
  return out;
}

registry.ops = {
  onData(entry, data) {
    if (!isLive(entry)) return;
    entry.pending.push(data);
    scheduleFlush(entry);
  },

  onExit(entry) {
    if (entry.state === "exited") return;
    entry.state = "exited";
    // Whatever the shell left behind goes now, while its session still has members and so can't be reused.
    const reaping = (entry.reaping ??= entry.pty.reap());
    if (!isLive(entry)) return;
    flush(entry);
    // The terminal is forgotten only after its last output has gone out and nothing it started is left running.
    entry.headless.write("", () => {
      if (!isLive(entry)) return;
      void reaping.then(() => forget(entry));
    });
  },

  async shutdown() {
    const closes = listTerminals().map(entry => closeTerminal(entry.id));
    await Promise.all([...closes, ...[...registry.closing].map(entry => entry.reaping)]);
  },

  killAllNow() {
    for (const entry of [...registry.terminals.values(), ...registry.closing]) entry.pty.killNow();
  },
};

/** Drops an exited terminal whose processes are all gone. */
function forget(entry: TerminalEntry) {
  if (!isLive(entry)) return;
  for (const timer of [entry.flushTimer, entry.redrawTimer]) if (timer) clearTimeout(timer);
  registry.terminals.delete(entry.id);
  registry.hooks.removed(entry.id);
  entry.headless.dispose();
  registry.owner.ended(entry.id);
}

/** Kills a terminal's processes and forgets it. Resolves once everything is gone. */
export async function closeTerminal(id: string) {
  const entry = registry.terminals.get(id);
  if (!entry) return false;
  registry.terminals.delete(id);
  registry.closing.add(entry);
  registry.hooks.removed(id);

  for (const timer of [entry.flushTimer, entry.redrawTimer]) if (timer) clearTimeout(timer);
  entry.flushTimer = entry.redrawTimer = null;
  entry.pending = [];

  entry.pty.hangup();
  entry.reaping ??= entry.pty.reap();
  await entry.reaping;
  await entry.pty.exited;
  entry.headless.dispose();
  registry.closing.delete(entry);
  return true;
}

/** Kills every terminal. Used on SIGINT/SIGTERM; one made after this is caught by the exit handler below. */
export function shutdownTerminals() {
  return registry.ops.shutdown();
}

if (!registry.processHandlersInstalled) {
  registry.processHandlersInstalled = true;
  // Last resort if the server dies without a clean shutdown, including terminals mid-close.
  process.on("exit", () => registry.ops.killAllNow());
}
