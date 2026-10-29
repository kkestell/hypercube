import { homedir } from "node:os";
import { join } from "node:path";
import { ModelError } from "./errors";

/**
 * `~/.config/hypercube/config.json` (or under `$XDG_CONFIG_HOME`): the lead's
 * settings and the profiles the lead can give workers. Shaped like the Rust
 * Hypercube's file, which it may share; fields we don't use are ignored. Read
 * again on each use, so edits apply to the next agent started.
 */

/** What an agent's model and effort are set to. OpenCode is the only harness we run. */
export type Settings = { harness: "opencode"; model: string; effort: string };

export type Profile = Settings & { name: string; description: string };

export type Config = { lead: Settings; profiles: Profile[] };

/** Used when there is no config file, and for the lead when the file has no `lead`. */
const defaultLead: Settings = { harness: "opencode", model: "openai/gpt-6-luna", effort: "high" };

const defaultConfig: Config = {
  lead: defaultLead,
  profiles: [
    { name: "luna", description: "General implementation, investigation and review.", ...defaultLead },
    { name: "sol", description: "Hard implementation, debugging and design work.", harness: "opencode", model: "openai/gpt-6.1-sol", effort: "high" },
    { name: "terra", description: "Small, well-defined tasks: lookups, simple edits and quick checks.", harness: "opencode", model: "openai/gpt-6.1-terra", effort: "medium" },
  ],
};

export function configPath() {
  const base = process.env.XDG_CONFIG_HOME || join(homedir(), ".config");
  return join(base, "hypercube", "config.json");
}

export async function loadConfig(): Promise<Config> {
  const path = configPath();
  const file = Bun.file(path);
  if (!(await file.exists())) return defaultConfig;
  let raw: unknown;
  try {
    raw = await file.json();
  } catch (error) {
    throw new ModelError(`Cannot read ${path}: ${error instanceof Error ? error.message : String(error)}`);
  }
  return parseConfig(raw, path);
}

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === "object" && value !== null && !Array.isArray(value);

/** `harness` is `claude` or `opencode` as in the Rust Hypercube's file; only `opencode` runs here. */
function parseSettings(value: unknown, what: string, path: string) {
  const fail = (problem: string) => new ModelError(`${path}: ${what} ${problem}`);
  if (!isObject(value)) throw fail("must be an object");
  const { harness, model, effort } = value;
  if (harness !== "claude" && harness !== "opencode") throw fail(`needs "harness" to be "claude" or "opencode"`);
  if (typeof model !== "string" || !model) throw fail(`needs a non-empty "model"`);
  if (typeof effort !== "string" || !effort) throw fail(`needs a non-empty "effort"`);
  return { harness, model, effort };
}

function parseConfig(raw: unknown, path: string): Config {
  if (!isObject(raw)) throw new ModelError(`${path}: expected a JSON object`);

  let lead = defaultLead;
  if (raw.lead !== undefined) {
    const settings = parseSettings(raw.lead, "the lead", path);
    if (settings.harness !== "opencode") throw new ModelError(`${path}: the lead's harness must be "opencode"; no other harness runs yet`);
    lead = { ...settings, harness: "opencode" };
  }

  if (!Array.isArray(raw.profiles) || raw.profiles.length === 0) throw new ModelError(`${path}: "profiles" must be a non-empty array`);
  const names = new Set<string>();
  const profiles: Profile[] = [];
  raw.profiles.forEach((entry: unknown, index) => {
    const name = isObject(entry) ? entry.name : undefined;
    const what = typeof name === "string" && name ? `profile "${name}"` : `profile #${index + 1}`;
    const settings = parseSettings(entry, what, path);
    const { description } = entry as Record<string, unknown>;
    if (typeof name !== "string" || !name) throw new ModelError(`${path}: ${what} needs a "name"`);
    if (typeof description !== "string" || !description) throw new ModelError(`${path}: ${what} needs a "description"`);
    if (names.has(name)) throw new ModelError(`${path}: two profiles are named "${name}"`);
    names.add(name);
    // The file may list profiles for harnesses we can't run (the Rust Hypercube's); they aren't offered.
    if (settings.harness === "opencode") profiles.push({ name, description, ...settings, harness: "opencode" });
  });
  return { lead, profiles };
}
