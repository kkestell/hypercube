export async function waitFor(condition: () => boolean, timeoutMs = 5000) {
  const deadline = Date.now() + timeoutMs;
  while (!condition()) {
    if (Date.now() > deadline) throw new Error("Timed out waiting for condition");
    await Bun.sleep(10);
  }
}

/** Sends raw HTTP to localhost, so headers like Host are exactly what is given, and returns the status line. */
export function rawHttp(port: number, lines: string[], body = "") {
  const request = `${lines.join("\r\n")}\r\n\r\n${body}`;
  return new Promise<string>((resolve, reject) => {
    Bun.connect({
      hostname: "localhost",
      port,
      socket: {
        open: socket => void socket.write(request),
        data: (socket, data) => {
          resolve(data.toString().split("\r\n")[0]!);
          socket.end();
        },
        error: (_socket, error) => reject(error),
      },
    });
  });
}

let sleeps = 0;

/**
 * A duration for `sleep` that no other test, run or process uses (the pid is
 * in it), so `aliveCount("^sleep <it>$")` counts this test's processes only.
 */
export const uniqueSleep = () => `${3000 + sleeps++}.${process.pid}`;

/** How many live processes' command lines match the regular expression. */
export const aliveCount = (pattern: string) =>
  Bun.spawnSync(["pgrep", "-f", pattern]).stdout.toString().trim().split("\n").filter(Boolean).length;
