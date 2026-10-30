import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { dlopen, FFIType } from "bun:ffi";
import { Terminal as HeadlessTerminal } from "@xterm/headless";
import { SCROLLBACK } from "../protocol";
import { aliveCount, uniqueSleep, waitFor } from "../../test/helpers";
import { localEnvironment } from "./environment";
import {
  attachTerminal,
  closeTerminal,
  createTerminal,
  getTerminal,
  resizeTerminal,
  setHooks,
  shutdownTerminals,
  writeInput,
  type TerminalEntry,
} from "./terminals";

// Tests run concurrently, so each uses terminal ids and `sleep` durations of its own.
const decoder = new TextDecoder();
const encoder = new TextEncoder();

/** What the hooks saw for each terminal, in order: `output:<text>` and `removed`. */
const logs: Record<string, string[]> = {};
const log = (id: string) => (logs[id] ??= []);
const outputOf = (id: string) => log(id).filter(event => event.startsWith("output:")).map(event => event.slice(7)).join("");

beforeAll(() => {
  setHooks({
    output: (entry, chunk) => log(entry.id).push(`output:${decoder.decode(chunk)}`),
    removed: id => log(id).push("removed"),
  });
});

afterAll(() => shutdownTerminals());

/** A terminal running `command` through the local environment, killed 100ms after SIGTERM rather than 2s. */
function make(id: string, command: string[], cwd = "/tmp") {
  return createTerminal(id, 80, 24, onData => localEnvironment.spawnPty(command, cwd, { cols: 80, rows: 24, onData, killGraceMs: 100 }));
}

/** A terminal with no process, whose output the test feeds by hand; `resizes` is what reached the PTY. */
function fake(id: string, cols = 80, rows = 24) {
  const resizes: [number, number][] = [];
  let feed!: (text: string) => void;
  let exit!: () => void;
  const exited = new Promise<void>(resolve => (exit = resolve));
  const entry = createTerminal(id, cols, rows, onData => {
    feed = text => onData(encoder.encode(text));
    return { sid: 0, write() {}, resize: (c, r) => void resizes.push([c, r]), hangup: exit, exited, stop: async () => {}, stopNow() {} };
  });
  return { entry, feed, resizes };
}

const { getsid } = dlopen(process.platform === "darwin" ? "libSystem.B.dylib" : "libc.so.6", {
  getsid: { args: [FFIType.i32], returns: FFIType.i32 },
}).symbols;

/** Starts an interactive zsh and waits until it has printed its prompt, so typed jobs run under job control. */
async function interactiveZsh(id: string) {
  const entry = make(id, ["/bin/zsh", "-f"]);
  await waitFor(() => outputOf(id).length > 0);
  return entry;
}

/** Lines of a headless terminal's active buffer, trailing blanks trimmed. */
function linesOf(terminal: HeadlessTerminal) {
  const buffer = terminal.buffer.active;
  const lines: string[] = [];
  for (let i = 0; i < buffer.length; i++) lines.push(buffer.getLine(i)!.translateToString(true));
  while (lines.at(-1) === "") lines.pop();
  return lines;
}

const parsed = (terminal: HeadlessTerminal, data: string) => new Promise<void>(resolve => terminal.write(data, resolve));

describe("registry", () => {
  test("a terminal runs in its directory with a clean environment, and is forgotten on close", async () => {
    const saved = process.env.CLAUDE_CODE_CHILD_SESSION;
    process.env.CLAUDE_CODE_CHILD_SESSION = "1";
    let entry: TerminalEntry;
    try {
      entry = make("a", ["/bin/sh"]);
    } finally {
      if (saved === undefined) delete process.env.CLAUDE_CODE_CHILD_SESSION;
      else process.env.CLAUDE_CODE_CHILD_SESSION = saved;
    }
    expect(getTerminal("a")).toBe(entry);
    expect(() => make("a", ["/bin/sh"])).toThrow();

    // A server started from inside Claude Code must not leak that session's markers into its terminals.
    writeInput("a", 'echo "marker-$((6*7)) $(pwd -P) [$CLAUDE_CODE_CHILD_SESSION]"\n');
    await waitFor(() => /marker-42 (\/private)?\/tmp \[\]/.test(outputOf("a")));

    expect(await closeTerminal("a")).toBe(true);
    expect(getTerminal("a")).toBeUndefined();
    expect(log("a")).toContain("removed");
    expect(await closeTerminal("a")).toBe(false);
  });

  test("a terminal that can't be started is not registered", () => {
    expect(() => make("bad", ["/bin/sh"], "/no/such/directory")).toThrow();
    expect(getTerminal("bad")).toBeUndefined();
  });
});

describe("output ordering", () => {
  test("a snapshot has everything before the attach, and the output after it has the rest", async () => {
    // A 5-row screen, so the first lines are scrollback.
    const { entry, feed, resizes } = fake("boundary", 40, 5);
    const first = Array.from({ length: 8 }, (_, i) => `line-${i + 1}\r\n`).join("");
    feed(first);

    let snapshot = "";
    let publishedBefore = 0;
    await new Promise<void>(resolve =>
      attachTerminal(entry, null, text => {
        snapshot = text;
        publishedBefore = log("boundary").length;
        resolve();
      }),
    );
    // Arrives after the attach was requested but before the client is subscribed, and after it.
    feed("between\r\n");
    await waitFor(() => outputOf("boundary").includes("between"));
    feed("after\r\n");
    await waitFor(() => outputOf("boundary").includes("after"));

    expect(snapshot).toContain("line-1\r\n");
    expect(snapshot).toContain("line-8");
    expect(snapshot).not.toContain("between");
    expect(log("boundary").slice(0, publishedBefore)).toEqual([`output:${first}`]);
    const after = log("boundary").slice(publishedBefore).map(event => event.slice(7)).join("");
    expect(after).toBe("between\r\nafter\r\n");

    // A client that applies the snapshot and then the output ends up where the server is.
    const view = new HeadlessTerminal({ cols: 40, rows: 5, scrollback: SCROLLBACK, allowProposedApi: true });
    await parsed(view, snapshot);
    await parsed(view, after);
    await parsed(entry.headless, "");
    expect(linesOf(view)).toEqual(linesOf(entry.headless));

    // The app is made to redraw: a row off its size and back, which is two SIGWINCHes.
    await waitFor(() => entry.redrawTimer === null);
    expect(resizes).toEqual([[40, 4], [40, 5]]);
  });

  test("the end still comes last when the process ends while the client's snapshot is pending", async () => {
    const entry = make("final", ["/bin/sh", "-c", "printf FINAL"]);
    // Attach from the start, so the process ends while the snapshot is pending.
    attachTerminal(entry, null, () => log("final").push("snapshot"));
    await waitFor(() => log("final").includes("removed"));

    expect(log("final")).toEqual(["snapshot", "output:FINAL", "removed"]);
    expect(getTerminal("final")).toBeUndefined();
  });

  test("a resize applies after the output written before it, which is parsed at the old size", async () => {
    const data = "1234567890\x1b[1GZ";
    const { entry, feed, resizes } = fake("order", 10, 5);
    feed(data);
    // In the same tick, before the output has been parsed.
    resizeTerminal("order", 5, 5);

    const expected = new HeadlessTerminal({ cols: 10, rows: 5, allowProposedApi: true });
    await parsed(expected, data);
    expected.resize(5, 5);
    await parsed(expected, "");

    await waitFor(() => entry.headless.cols === 5);
    await parsed(entry.headless, "");
    expect(linesOf(entry.headless)).toEqual(linesOf(expected));
    expect(resizes).toEqual([[5, 5]]);
  });

  test("overlapping attaches at different sizes each get a snapshot at their own size", async () => {
    const { entry, feed } = fake("sizes");
    feed("0".repeat(100));
    const sizes: string[] = [];
    await new Promise<void>(resolve => {
      for (const [cols, rows] of [[40, 10], [60, 20], [30, 5]] as const) {
        attachTerminal(entry, { cols, rows }, () => {
          sizes.push(`${entry.headless.cols}x${entry.headless.rows}`);
          if (sizes.length === 3) resolve();
        });
      }
    });
    expect(sizes).toEqual(["40x10", "60x20", "30x5"]);
  });

  test("a real resize during the forced redraw wins, for the process and the headless terminal", async () => {
    const entry = make("winch", ["/bin/bash", "-c", "trap 'echo winch $(stty size)' WINCH; echo ready; while :; do sleep .01; done"]);
    await waitFor(() => outputOf("winch").includes("ready"));
    await new Promise<void>(resolve => attachTerminal(entry, null, () => resolve()));
    resizeTerminal("winch", 100, 40);
    await waitFor(() => entry.redrawTimer === null);

    // The restore has run. Signal the app by hand to read the size it ends with.
    const before = outputOf("winch").match(/winch/g)?.length ?? 0;
    process.kill(entry.pty.sid, "SIGWINCH");
    await waitFor(() => (outputOf("winch").match(/winch/g)?.length ?? 0) > before);

    expect(outputOf("winch").match(/winch \d+ \d+/g)!.at(-1)).toBe("winch 40 100");
    expect([entry.headless.cols, entry.headless.rows]).toEqual([100, 40]);
  });
});

describe("closing and ownership", () => {
  test("close kills foreground and background jobs, which run in their own process groups", async () => {
    const [background, foreground] = [uniqueSleep(), uniqueSleep()];
    await interactiveZsh("jobs");
    writeInput("jobs", `sleep ${background} &\n`);
    await waitFor(() => aliveCount(`^sleep ${background}$`) === 1);
    writeInput("jobs", `sleep ${foreground}\n`);
    await waitFor(() => aliveCount(`^sleep ${foreground}$`) === 1);

    await closeTerminal("jobs");
    expect(aliveCount(`^sleep (${background}|${foreground})$`)).toBe(0);
  });

  // Shutdown closes every terminal, including other tests', so this one runs alone.
  test.serial("shutdown kills every terminal, including one mid-close whose shell ignores TERM and HUP", async () => {
    const [plain, stuck] = [uniqueSleep(), uniqueSleep()];
    make("plain", ["/bin/sh", "-c", `sleep ${plain}`]);
    make("stuck", ["/bin/sh", "-c", `trap '' TERM HUP; while true; do sleep ${stuck}; done`]);
    await waitFor(() => aliveCount(`^sleep (${plain}|${stuck})$`) === 2);

    const closing = closeTerminal("stuck");
    expect(getTerminal("stuck")).toBeUndefined();
    await shutdownTerminals();
    expect(getTerminal("plain")).toBeUndefined();
    expect(aliveCount(`^sleep (${plain}|${stuck})$`)).toBe(0);
    expect(aliveCount(`while true; do sleep ${stuck}`)).toBe(0);
    await closing;
  });

  test("when an interactive shell exits, a nohup'd job in its own process group is killed without waiting for close", async () => {
    const orphan = uniqueSleep();
    const entry = await interactiveZsh("orphan");
    writeInput("orphan", `nohup sleep ${orphan} >/dev/null 2>&1 &!\n`);
    await waitFor(() => aliveCount(`^sleep ${orphan}$`) === 1);
    writeInput("orphan", "exit\n");
    await waitFor(() => log("orphan").includes("removed"));
    expect(aliveCount(`^sleep ${orphan}$`)).toBe(0);
    expect(entry.state).toBe("exited");
  });

  test("a process that left the session with setsid is not the terminal's", async () => {
    const [daemon, shell] = [uniqueSleep(), uniqueSleep()];
    make("daemon", ["/bin/sh", "-c", `perl -MPOSIX -e 'fork and exit; setsid; exec "sleep", "${daemon}"' & sleep ${shell}`]);
    try {
      await waitFor(() => aliveCount(`^sleep (${daemon}|${shell})$`) === 2);
      await closeTerminal("daemon");
      expect(aliveCount(`^sleep ${shell}$`)).toBe(0);
      expect(aliveCount(`^sleep ${daemon}$`)).toBe(1);
    } finally {
      Bun.spawnSync(["pkill", "-f", `^sleep ${daemon}$`]);
    }
  });

  test("an exited terminal signals nothing on close or server exit, even once its session id belongs to an unrelated session", async () => {
    const unrelated = uniqueSleep();
    const entry = make("reused", ["/bin/sh", "-c", "exit 0"]);
    await waitFor(() => log("reused").includes("removed"));

    // Stand in for session id reuse: an unrelated session whose leader has also exited, leaving a child.
    const leader = Bun.spawn(["/bin/sh", "-c", `nohup sleep ${unrelated} >/dev/null 2>&1 & sleep .1`], {
      terminal: { cols: 80, rows: 24, data: () => {} },
    });
    await leader.exited;
    leader.terminal?.close();
    try {
      await waitFor(() => aliveCount(`^sleep ${unrelated}$`) === 1);
      const child = Number(Bun.spawnSync(["pgrep", "-f", `^sleep ${unrelated}$`]).stdout.toString());
      expect(getsid(child)).toBe(leader.pid);
      entry.pty.sid = leader.pid;

      // The last-resort kill that runs when the server process exits looks the session up too.
      entry.pty.stopNow();
      await entry.pty.stop();
      await Bun.sleep(50);
      expect(aliveCount(`^sleep ${unrelated}$`)).toBe(1);
    } finally {
      Bun.spawnSync(["pkill", "-f", `^sleep ${unrelated}$`]);
    }
  });
});
