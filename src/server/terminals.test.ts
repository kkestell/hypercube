import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { dlopen, FFIType } from "bun:ffi";
import { Terminal as HeadlessTerminal } from "@xterm/headless";
import { aliveCount, waitFor } from "../../test/helpers";
import { localEnvironment } from "./environment";
import {
  attachTerminal,
  closeTerminal,
  createTerminal,
  getTerminal,
  listTerminals,
  resizeTerminal,
  setHooks,
  setOwnerHooks,
  shutdownTerminals,
  writeInput,
  type TerminalEntry,
} from "./terminals";

const decoder = new TextDecoder();

let output: Record<string, string>;
let events: string[];

beforeEach(() => {
  output = {};
  events = [];
  setHooks({
    output: (entry, chunk) => {
      output[entry.id] = (output[entry.id] ?? "") + decoder.decode(chunk, { stream: true });
      events.push(`output:${entry.id}`);
    },
    removed: id => events.push(`removed:${id}`),
  });
  setOwnerHooks({ ended: id => events.push(`ended:${id}`) });
});

afterEach(async () => {
  await shutdownTerminals();
});

/** A terminal running `command` (a shell, unless given) through the local environment. */
function make(id: string, cols: number, rows: number, command: string[], cwd = "/tmp") {
  return createTerminal(id, cols, rows, onData => localEnvironment.spawnPty(command, cwd, { cols, rows, onData }));
}

const snapshotOf = (entry: TerminalEntry) => new Promise<string>(resolve => attachTerminal(entry, null, resolve));

const { getsid } = dlopen(process.platform === "darwin" ? "libSystem.B.dylib" : "libc.so.6", {
  getsid: { args: [FFIType.i32], returns: FFIType.i32 },
}).symbols;

/** Starts an interactive zsh and waits until it has printed its prompt, so typed jobs run under job control. */
async function interactiveZsh(id: string) {
  const entry = make(id, 80, 24, ["/bin/zsh", "-f"]);
  await waitFor(() => (output[id]?.length ?? 0) > 0);
  return entry;
}

describe("registry", () => {
  test("a terminal runs in its directory with a clean environment, and is forgotten on close", async () => {
    const saved = process.env.CLAUDE_CODE_CHILD_SESSION;
    process.env.CLAUDE_CODE_CHILD_SESSION = "1";
    let entry: TerminalEntry;
    try {
      entry = make("a", 80, 24, ["/bin/sh"]);
    } finally {
      if (saved === undefined) delete process.env.CLAUDE_CODE_CHILD_SESSION;
      else process.env.CLAUDE_CODE_CHILD_SESSION = saved;
    }
    expect(getTerminal("a")).toBe(entry);
    expect(() => make("a", 80, 24, ["/bin/sh"])).toThrow();

    // A server started from inside Claude Code must not leak that session's markers into its terminals.
    writeInput("a", 'echo "marker-$((6*7)) $(pwd -P) [$CLAUDE_CODE_CHILD_SESSION]"\n');
    await waitFor(() => /marker-42 (\/private)?\/tmp \[\]/.test(output.a ?? ""));

    expect(await closeTerminal("a")).toBe(true);
    expect(getTerminal("a")).toBeUndefined();
    expect(events).toContain("removed:a");
    expect(await closeTerminal("a")).toBe(false);
  });

  test("a terminal that can't be started is not registered", () => {
    expect(() => make("bad", 80, 24, ["/bin/sh"], "/no/such/directory")).toThrow();
    expect(getTerminal("bad")).toBeUndefined();
  });

  test("keeps running with nobody attached", async () => {
    make("bg", 80, 24, ["/bin/sh"]);
    writeInput("bg", "sleep .3; echo later\n");
    await waitFor(() => output.bg?.includes("later") ?? false);
    expect(listTerminals().map(e => e.id)).toEqual(["bg"]);
  });

  test("snapshots the screen and scrollback", async () => {
    const entry = make("snap", 40, 5, ["/bin/sh"]);
    writeInput("snap", "for i in 1 2 3 4 5 6 7 8 9 10; do echo line-$i; done\n");
    await waitFor(() => output.snap?.includes("line-10") ?? false);

    const snapshot = await snapshotOf(entry);
    // Scrollback lines that scrolled off the 5-row screen are included.
    expect(snapshot).toContain("line-1\r\n");
    expect(snapshot).toContain("line-10");
  });

  test("a shell that exits is forgotten and reported to its owner, after its last output", async () => {
    for (let i = 0; i < 8; i++) {
      const id = `quick-${i}`;
      make(id, 80, 24, ["/bin/sh", "-c", `echo done-${i}`]);
      await waitFor(() => events.includes(`ended:${id}`));
      expect(output[id]).toContain(`done-${i}`);
      expect(events.indexOf(`output:${id}`)).toBeLessThan(events.indexOf(`removed:${id}`));
      expect(getTerminal(id)).toBeUndefined();
    }
  });
});

describe("output ordering", () => {
  test("the end still comes last when the process ends while the client's snapshot is pending", async () => {
    const received: string[] = [];
    setHooks({ output: (_entry, chunk) => received.push(`output:${decoder.decode(chunk)}`), removed: () => received.push("removed") });
    const entry = make("final", 80, 24, ["/bin/sh", "-c", "printf FINAL"]);
    // Attach from the start, so the process ends while the snapshot is pending.
    attachTerminal(entry, null, () => received.push("snapshot"));
    await waitFor(() => received.includes("removed"));

    expect(received).toEqual(["snapshot", "output:FINAL", "removed"]);
  });

  test("a snapshot and the output after it never overlap or leave a gap", async () => {
    const entry = make(
      "seq",
      80,
      24,
      // Fast enough that the parser falls behind, so output keeps arriving while the snapshot waits its turn.
      ["/bin/sh", "-c", "seq -f n%g 0 199999; sleep 5"],
    );
    await waitFor(() => (output.seq?.length ?? 0) > 100);
    const after: Uint8Array[] = [];
    let attached = false;
    setHooks({ ...hooksSpy(), output: (_entry, chunk) => attached && after.push(chunk) });
    const snapshot = await new Promise<string>(resolve =>
      attachTerminal(entry, null, snapshot => {
        attached = true;
        resolve(snapshot);
      }),
    );
    expect(snapshot).not.toContain("n199999");
    await waitFor(() => decoder.decode(concat(after)).includes("n199999"), 20_000);

    const view = new HeadlessTerminal({ cols: 80, rows: 24, scrollback: 5000, allowProposedApi: true });
    await parsed(view, snapshot);
    for (const chunk of after) await parsed(view, chunk);
    await parsed(entry.headless, "");
    // The view's scrollback holds the last lines in a row, with none missing or repeated.
    const numbers = linesOf(view).filter(line => /^n\d+$/.test(line)).map(line => Number(line.slice(1)));
    expect(numbers.length).toBeGreaterThan(4000);
    expect(numbers).toEqual(numbers.map((_, i) => 199999 - (numbers.length - 1) + i));
    expect(linesOf(view)).toEqual(linesOf(entry.headless));
  }, 30_000);

  test("a resize applies after the output written before it, which is parsed at the old size", async () => {
    const data = "1234567890\x1b[1GZ";
    // Resize in the same tick the output arrives, before it has been parsed.
    let resized = false;
    const entry = createTerminal("order", 10, 5, onData =>
      localEnvironment.spawnPty(["/bin/sh", "-c", "printf '1234567890\\033[1GZ'; sleep 5"], "/tmp", {
        cols: 10,
        rows: 5,
        onData: chunk => {
          onData(chunk);
          if (resized) return;
          resized = true;
          resizeTerminal("order", 5, 5);
        },
      }),
    );
    await waitFor(() => resized);

    const expected = new HeadlessTerminal({ cols: 10, rows: 5, allowProposedApi: true });
    await parsed(expected, data);
    expected.resize(5, 5);
    await parsed(expected, "");

    await waitFor(() => entry.headless.cols === 5);
    await parsed(entry.headless, "");
    expect(linesOf(entry.headless)).toEqual(linesOf(expected));
  });

  test("overlapping attaches at different sizes each get a snapshot at their own size", async () => {
    const entry = make("sizes", 80, 24, ["/bin/sh", "-c", "printf %0100d 0; sleep 5"]);
    await waitFor(() => output.sizes?.includes("0".repeat(100)) ?? false);
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

  test("after an attach the app gets SIGWINCH twice, and the sizes end where they were", async () => {
    const entry = make("winch", 80, 24, ["/bin/bash", "-c", "trap 'echo winch $(stty size)' WINCH; echo ready; while :; do sleep .01; done"]);
    await waitFor(() => output.winch?.includes("ready") ?? false);
    await snapshotOf(entry);
    await waitFor(() => (output.winch?.match(/winch/g)?.length ?? 0) >= 2);
    expect(output.winch).toContain("winch 23 80");
    expect(output.winch).toContain("winch 24 80");
    expect([entry.cols, entry.rows, entry.headless.cols, entry.headless.rows]).toEqual([80, 24, 80, 24]);
  });

  test("a real resize during the forced redraw wins, for the process and the headless terminal", async () => {
    const entry = make("race", 80, 24, ["/bin/sh"]);
    await snapshotOf(entry);
    resizeTerminal("race", 100, 40);
    writeInput("race", "stty size\n");
    await waitFor(() => output.race?.includes("40 100") ?? false);
    expect([entry.headless.cols, entry.headless.rows]).toEqual([100, 40]);
  });
});

describe("closing and ownership", () => {
  test("close kills foreground and background jobs, which run in their own process groups", async () => {
    await interactiveZsh("jobs");
    writeInput("jobs", "sleep 7101 &\n");
    await waitFor(() => aliveCount("sleep 7101") === 1);
    writeInput("jobs", "sleep 7102\n");
    await waitFor(() => aliveCount("sleep 7102") === 1);

    await closeTerminal("jobs");
    expect(aliveCount("sleep 710[12]")).toBe(0);
  });

  test("shutdown kills every terminal, including one mid-close whose shell ignores TERM and HUP", async () => {
    make("plain", 80, 24, ["/bin/sh", "-c", "sleep 7104"]);
    make("stuck", 80, 24, ["/bin/sh", "-c", "trap '' TERM HUP; while true; do sleep 7105; done"]);
    await waitFor(() => aliveCount("sleep 710[45]") === 2);

    const closing = closeTerminal("stuck");
    await Bun.sleep(100);
    expect(getTerminal("stuck")).toBeUndefined();
    await shutdownTerminals();
    expect(listTerminals()).toEqual([]);
    expect(aliveCount("sleep 710[45]")).toBe(0);
    expect(aliveCount("while true; do sleep 7105")).toBe(0);
    await closing;
  }, 10_000);

  test("when an interactive shell exits, a nohup'd job in its own process group is killed without waiting for close", async () => {
    const entry = await interactiveZsh("orphan");
    writeInput("orphan", "nohup sleep 7392 >/dev/null 2>&1 &!\n");
    await waitFor(() => aliveCount("sleep 7392") === 1);
    writeInput("orphan", "exit\n");
    await waitFor(() => events.includes("ended:orphan"));
    expect(aliveCount("sleep 7392")).toBe(0);
    expect(entry.state).toBe("exited");
  });

  test("a process that left the session with setsid is not the terminal's", async () => {
    make("daemon", 80, 24, ["/bin/sh", "-c", `perl -MPOSIX -e 'fork and exit; setsid; exec "sleep", "7395"' & sleep 7396`]);
    try {
      await waitFor(() => aliveCount("^sleep 739[56]") === 2);
      await closeTerminal("daemon");
      expect(aliveCount("sleep 7396")).toBe(0);
      expect(aliveCount("sleep 7395")).toBe(1);
    } finally {
      Bun.spawnSync(["pkill", "-f", "sleep 7395"]);
    }
  });

  test("an exited terminal signals nothing on close or server exit, even once its session id belongs to an unrelated session", async () => {
    const entry = make("reused", 80, 24, ["/bin/sh", "-c", "exit 0"]);
    await waitFor(() => events.includes("ended:reused"));

    // Stand in for session id reuse: an unrelated session whose leader has also exited, leaving a child.
    const leader = Bun.spawn(["/bin/sh", "-c", "nohup sleep 7393 >/dev/null 2>&1 & sleep .1"], {
      terminal: { cols: 80, rows: 24, data: () => {} },
    });
    await leader.exited;
    leader.terminal?.close();
    try {
      await waitFor(() => aliveCount("sleep 7393") === 1);
      const child = Number(Bun.spawnSync(["pgrep", "-f", "sleep 7393"]).stdout.toString());
      expect(getsid(child)).toBe(leader.pid);
      entry.pty.sid = leader.pid;

      // The last-resort kill that runs when the server process exits looks the session up too.
      entry.pty.killNow();
      await entry.pty.reap();
      await Bun.sleep(50);
      expect(aliveCount("sleep 7393")).toBe(1);
    } finally {
      Bun.spawnSync(["pkill", "-f", "sleep 7393"]);
    }
  });
});

/** Lines of a headless terminal's active buffer, trailing blanks trimmed. */
function linesOf(terminal: HeadlessTerminal) {
  const buffer = terminal.buffer.active;
  const lines: string[] = [];
  for (let i = 0; i < buffer.length; i++) lines.push(buffer.getLine(i)!.translateToString(true));
  while (lines.at(-1) === "") lines.pop();
  return lines;
}

const parsed = (terminal: HeadlessTerminal, data: string | Uint8Array) => new Promise<void>(resolve => terminal.write(data, resolve));

function hooksSpy() {
  return {
    output: (entry: TerminalEntry, chunk: Uint8Array) => {
      output[entry.id] = (output[entry.id] ?? "") + decoder.decode(chunk, { stream: true });
    },
    removed: () => {},
  };
}

function concat(chunks: Uint8Array[]) {
  return new Uint8Array(chunks.flatMap(chunk => [...chunk]));
}
