# Serve the session's files

## Plan

`agents/plans/2026-11-01-002-file-browser-backend.md`

## Summary

The approved file browser now lists and reads the session's workspace through
confined GET routes. Visible folders and file tabs refresh sequentially each
second, with ETags, deleted labels, and the planned file refusal messages.

## Departures from the plan

- With the lead's approval, changed one line in `store.Tx.Session` to wrap
  `sql.ErrNoRows`. Previously it discarded the error identity, preventing a
  reliable distinction between an unknown session (404) and a database failure.

## Decisions

- Lowered `maxFile` to 256 KiB as the plan requires after measurement. With
  grammars loaded, repeated `internal/ui/ui.go` highlighted in 83/77/83 ms at
  256 KiB, compared with 399/335/326 ms at 1 MiB and 163/159/160 ms at 512 KiB.

## Checks run

- `GOCACHE=/tmp/hypercube-c6387821-go-cache go test ./internal/ui -run '^TestFiles$'`
  — passed.
- `cd internal/ui && bun test files.test.js` — passed, seven tests.
- `TMPDIR=/tmp BUN_INSTALL_CACHE_DIR=/tmp/hypercube-c6387821-bun-cache make ui`
  — passed; rebuilt `static/ui.css`.
- `GOCACHE=/tmp/hypercube-c6387821-go-cache TMPDIR=/tmp make check` — passed:
  builds, all 48 Bun tests, Go vet, formatting, and the full Go race suite.
- `git diff --check` and parsing the page's inline module with `Bun.Transpiler`
  — passed.

Initial focused Go testing and `make ui` could not write their default caches
or temporary directories under the sandbox; the commands above used `/tmp`.
Go builds emitted nonfatal module stat-cache write warnings during `make check`.
No dev server or visual checks were run, as instructed.

## Manual verification

Reproduce the highlight measurement from `internal/ui`:

```sh
bun -e '
import {highlight} from "./viewer.js";
const source = await Bun.file("ui.go").text();
highlight(source, "go");
for (const size of [1048576, 524288, 262144]) {
  const text = source.repeat(Math.ceil(size / source.length)).slice(0, size);
  const times = [];
  for (let i = 0; i < 3; i++) {
    const start = performance.now();
    highlight(text, "go");
    times.push(Math.round(performance.now() - start));
  }
  console.log(size, times);
}'
```

The measured results are recorded above. Live browser verification remains with
the lead.
