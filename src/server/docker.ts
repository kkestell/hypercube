import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { collect, localEnvironment, requireInside, type Environment } from "./environment";
import { spawnLocalPty } from "./localPty";

/**
 * The Docker environment: a session gets a container made from Hypercube's base
 * image, its clone is inside it, and everything runs there with `docker exec`.
 *
 * Nothing from the host's home directory is mounted. The container gets the
 * host's GitHub token as `GH_TOKEN` and OpenCode's credentials through
 * `opencode auth import`, from stdin; neither is written to a file on the host,
 * and neither is passed as a command-line argument, which `ps` would show.
 *
 * `docker exec` does not pass signals on to what it runs, so killing the docker
 * CLI would leave the process running in the container. Each process is started
 * as the leader of a session of its own (a PTY's is already one) and records its
 * pid in a file; stopping it signals every process in that session, background
 * and reparented ones included. A process that moves itself to a session of its
 * own is no longer ours and is left, like a local terminal's.
 */

/** A container has one clone, so its path doesn't need to differ between sessions. */
export const DOCKER_CLONE_PATH = "/workspace";

/** How an agent in a container reaches the host. Docker Desktop provides the name. */
export const CONTAINER_HOST = "host.docker.internal";

/** `url`, a URL of this server, as a process in a container reaches it. */
export function containerUrl(url: string) {
  const reachable = new URL(url);
  reachable.hostname = CONTAINER_HOST;
  return reachable.href;
}

const DOCKERFILE = join(import.meta.dir, "../../docker/Dockerfile");
/** Where each process in a container records its pid. */
const PIDS = "/run/hypercube";
const RECORD_PID = `echo $$ > ${PIDS}/$0.pid`;

/**
 * Stops the process `$0` started and everything in its session: SIGTERM and
 * SIGHUP, then SIGKILL for what is left after two seconds. It waits for the pid
 * file in case the process was only just started. A session id is not reused
 * while the session has members, and the callers only run this while the
 * process is still running, so a pid read here is the process's own.
 */
const STOP = `
i=0; while [ ! -s ${PIDS}/$0.pid ] && [ $i -lt 20 ]; do sleep 0.1; i=$((i+1)); done
pid=$(cat ${PIDS}/$0.pid 2>/dev/null) || exit 0
pkill -TERM -s "$pid"; pkill -HUP -s "$pid"
i=0; while pgrep -s "$pid" >/dev/null && [ $i -lt 20 ]; do sleep 0.1; i=$((i+1)); done
pkill -KILL -s "$pid"
exit 0`;

const docker = (...args: string[]) => ["docker", ...args];

/** `-e NAME` flags that pass `env`'s variables on from the docker CLI's own environment, so values stay out of the argument list. */
const envFlags = (env: Record<string, string | undefined>) => Object.entries(env).flatMap(([name, value]) => (value === undefined ? [] : ["-e", name]));

const building = new Map<string, Promise<void>>();

/**
 * The base image's tag, building the image if it isn't there. The tag is a hash
 * of what goes into the build, so changing the Dockerfile or the host's extra CA
 * certificates builds a new image on the next session instead of using a stale one.
 */
async function ensureImage() {
  // A TLS-intercepting proxy on the host's network signs HTTPS with a CA the container doesn't know. The host's extra CA file is how it is told.
  const caFile = process.env.NODE_EXTRA_CA_CERTS;
  const extraCa = caFile ? await Bun.file(caFile).text().catch(() => "") : "";
  const dockerfile = await Bun.file(DOCKERFILE).text();
  const tag = `hypercube-base:${new Bun.CryptoHasher("sha256").update(dockerfile).update("\0").update(extraCa).digest("hex").slice(0, 12)}`;

  if ((await Bun.spawn(docker("image", "inspect", tag), { stdout: "ignore", stderr: "ignore" }).exited) === 0) return tag;
  let build = building.get(tag);
  if (!build) {
    build = (async () => {
      const context = await mkdtemp(join(tmpdir(), "hypercube-image-"));
      try {
        await Bun.write(join(context, "Dockerfile"), dockerfile);
        await Bun.write(join(context, "extra-ca.crt"), extraCa);
        const child = Bun.spawn(docker("build", "--quiet", "--tag", tag, context), { stdin: "ignore", stdout: "pipe", stderr: "pipe" });
        const [stderr, code] = await Promise.all([new Response(child.stderr).text(), child.exited, new Response(child.stdout).text()]);
        if (code !== 0) throw new Error(`Could not build the Docker image: ${stderr.trim().split("\n").slice(-8).join("\n")}`);
      } finally {
        await rm(context, { recursive: true, force: true });
      }
    })().finally(() => building.delete(tag));
    building.set(tag, build);
  }
  await build;
  return tag;
}

/** Runs a command on the host that handles a secret, and throws a failure without its output. */
async function hostOutput(cmd: string[], what: string) {
  const child = localEnvironment.spawn(cmd, process.cwd());
  child.stdin.end();
  const [stdout, code] = await Promise.all([new Response(child.stdout).text(), child.exited, new Response(child.stderr).text()]);
  if (code !== 0 || !stdout.trim()) throw new Error(`Could not get ${what} from the host (\`${cmd.join(" ")}\` ${code === 0 ? "printed nothing" : `exited with code ${code}`})`);
  return stdout;
}

export function dockerEnvironment(sessionId: string): Environment {
  const container = `hypercube-${sessionId}`;

  const exec = (flags: string[], cwd: string, command: string[]) => docker("exec", ...flags, "-w", cwd, container, ...command);

  /** Stops what the process `id` started, if it is still running. Best effort: nothing is left to stop if the container is gone. */
  const stopIn = (id: string) => localEnvironment.run(docker("exec", container, "sh", "-c", STOP, id)).catch(() => {});
  const stopInNow = (id: string) => void Bun.spawnSync(docker("exec", container, "sh", "-c", STOP, id), { stdin: "ignore", stdout: "ignore", stderr: "ignore", timeout: 5000 });

  const environment: Environment = {
    async setup(repo, clonePath) {
      const image = await ensureImage();
      const ghToken = (await hostOutput(["gh", "auth", "token"], "a GitHub token")).trim();
      const create = docker("run", "--detach", "--init", "--name", container, "--label", `hypercube.session=${sessionId}`, "--workdir", clonePath, "-e", "GH_TOKEN", image, "sleep", "infinity");
      await collect(localEnvironment.spawn(create, process.cwd(), { GH_TOKEN: ghToken }), create);
      try {
        // OpenCode's credentials go from the host's OpenCode to the container's, through memory.
        // Only the OpenAI login: agents run on openai/* models, and the container needs no other account.
        const exported = await hostOutput(["opencode", "auth", "export"], "OpenCode's credentials");
        try {
          const credentials = JSON.stringify((JSON.parse(exported) as { integrationID: string }[]).filter(credential => credential.integrationID === "openai"));
          const imported = environment.spawn(["opencode", "auth", "import"], "/");
          imported.stdin.write(credentials);
          await collect(imported, ["opencode", "auth", "import"]);
        } catch {
          // Nothing from the parse or the import goes in the message: either could echo a credential, and the message is shown, logged and stored.
          throw new Error("Could not import OpenCode's credentials into the container");
        }

        await environment.run(["gh", "auth", "setup-git"]);
        // Commits need an author. A host without one leaves the container without, and git says so when an agent commits.
        for (const key of ["user.name", "user.email"]) {
          const value = await hostOutput(["git", "config", "--get", key], key).catch(() => "");
          if (value.trim()) await environment.run(["git", "config", "--global", key, value.trim()]);
        }
        await environment.run(["gh", "repo", "clone", repo, clonePath]);
      } catch (error) {
        await environment.teardown(clonePath).catch(() => {});
        throw error;
      }
    },

    async ensureRunning() {
      // Docker restarting stops containers, and nothing starts them again.
      const asked = (...args: string[]) =>
        localEnvironment.run(docker(...args)).catch(error => {
          throw new Error(`Docker said: ${error instanceof Error ? error.message : String(error)}`);
        });
      const state = (await asked("ps", "--all", "--filter", `name=^${container}$`, "--format", "{{.State}}")).stdout.trim();
      if (!state) throw new Error("This session's container no longer exists. Delete the session.");
      if (state !== "running") await asked("start", container);
    },

    spawn(cmd, cwd, env = {}) {
      const id = crypto.randomUUID();
      let done = false;
      const cli = localEnvironment.spawn(
        exec(["-i", ...envFlags(env)], cwd, ["sh", "-c", `mkdir -p ${PIDS} && exec setsid --wait sh -c '${RECORD_PID}; exec "$@"' "$0" "$@"`, id, ...cmd]),
        process.cwd(),
        env,
      );
      void cli.exited.then(() => (done = true));
      return {
        ...cli,
        kill() {
          if (done) return;
          // If the container can't be reached the CLI is stopped here, which ends the process's stdin and with it, normally, the process.
          void stopIn(id).finally(() => done || cli.kill());
        },
        killNow() {
          if (done) return;
          stopInNow(id);
          cli.kill();
        },
      };
    },

    run: (cmd, cwd = "/") => collect(environment.spawn(cmd, cwd), cmd),

    spawnPty(cmd, cwd, options) {
      const id = crypto.randomUUID();
      // The docker CLI needs a PTY of its own to give the container one. It gets the terminal's variables from its own environment.
      const local = spawnLocalPty(
        exec(["-it", "-e", "TERM", "-e", "COLORTERM"], cwd, ["sh", "-c", `mkdir -p ${PIDS} && ${RECORD_PID}; exec "$@"`, id, ...cmd]),
        process.cwd(),
        options,
        { TERM: "xterm-256color", COLORTERM: "truecolor" },
      );
      return {
        ...local,
        async reap() {
          await stopIn(id);
          await local.reap();
        },
        killNow() {
          stopInNow(id);
          local.killNow();
        },
      };
    },

    loginShell: () => ["bash", "-l"],

    async teardown() {
      // Removing the container ends everything in it. `--force` is also fine if it is already gone.
      await localEnvironment.run(docker("rm", "--force", "--volumes", container));
    },

    async resolveDir(clonePath, dir) {
      // Real paths, so a symlink in the clone can't lead out of it.
      let root: string;
      let target: string;
      try {
        [root, target] = (await environment.run(["sh", "-c", 'cd "$0" && pwd -P && cd "$1" && pwd -P', clonePath, dir], clonePath)).stdout.trim().split("\n") as [string, string];
      } catch {
        throw new Error(`"${dir}" is not a directory that exists`);
      }
      requireInside(root, target, dir);
      return target;
    },
  };
  return environment;
}
