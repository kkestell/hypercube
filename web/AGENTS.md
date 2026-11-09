# Website

This is the Hugo site for Hypercube. `taikonaut.dev` is the placeholder public
URL until deployment. Follow the root `AGENTS.md`.

- Run `make web-build` or `make web-serve` from the repository root. Inside
  `web/`, use `make build` or `make serve`. These install the Hugo version pinned
  in `Makefile` into gitignored `bin/`; do not depend on a system Hugo.
- `hugo.toml` owns site configuration and mounts. Hextra is pinned as a Hugo
  module in `go.mod`; Hugo Extended builds the site without a JavaScript build.
- `content/` holds website-specific pages and the docs landing page. User guides
  live only in `../docs/*.md`, mounted at `content/docs`; screenshots live only
  in `../docs/images/`, mounted at `static/docs/images`. Never publish the
  capture fixtures in `../docs/screenshots/`.
- Shared guides keep their Markdown H1 and have no frontmatter; each guide's
  title and weight come from a `[[cascade]]` entry in `hugo.toml`. Hextra
  renders the page title; the heading render hook suppresses the duplicate guide
  H1. Website-specific pages use TOML frontmatter.
- Keep guide links relative with `.md` filenames; Hugo's embedded link render
  hook resolves them. Links to repository files outside the guides use GitHub
  URLs. Screenshots use raw `<picture>` markup (`unsafe` Goldmark) with paths
  relative to the guide; `uglyURLs` makes `images/...` resolve to
  `/docs/images/`.
- `public/`, `resources/`, `.hugo_build.lock`, and `bin/` are generated and
  gitignored. Never edit them. `make check` includes the website build.

Deployment settings are in `../agents/documentation.md` under Website.
