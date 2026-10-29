import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Tests write the harness hook and plugin files here, never into the user's ~/.cache.
process.env.HYPERCUBE_HARNESS_DIR = mkdtempSync(join(tmpdir(), "hypercube-harness-"));
