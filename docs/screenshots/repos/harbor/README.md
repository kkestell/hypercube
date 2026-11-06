# harbor

A small static site generator. harbor turns a folder of Markdown pages into
HTML using Go templates.

```sh
harbor build            # render content/ into public/
harbor serve -addr :8080
```

## Layout

- `content/` holds Markdown pages. A page's front matter sets its title and
  template.
- `templates/` holds the Go templates pages render with.
- `public/` is the output. harbor replaces it on every build.
