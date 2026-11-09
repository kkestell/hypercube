# Isolate Hypercube's Codex conversations

## Goal

Keep new Hypercube conversations out of normal Codex Recents while preserving
native durable resume, the user's settings and credentials, and their `HOME`.
Preserve existing conversation resumes rather than silently starting replacements.

## Decisions

- Use `StartArgs.StateDir/codex` as the child process's `CODEX_HOME`. Override
  `sqlite_home` to that directory on the command line so a linked user config
  cannot send thread metadata back into the normal Codex database.
- Read the source home from the inherited `CODEX_HOME`, otherwise `~/.codex`.
  Share only known inputs through symlinks: `config.toml`, `auth.json`,
  `.credentials.json`, `AGENTS.md`, `skills`, `agents`, `rules`, `hooks`,
  `plugins`, and `managed_config.toml`. Do not mirror the home or link session,
  database, cache, or log storage. Do not copy or synchronize credentials.
  Codex's `resolve_auth_file` follows credential symlinks before atomic refresh,
  allowing its existing native file-auth lifecycle to continue.
- Remove `threadSource="chatgpt_hidden"`: the installed protocol documents it as
  analytics attribution, not visibility. Keep ordinary `thread/start` and
  `thread/resume` behavior and identifiers.
- Import only a requested older conversation's rollout into private storage at
  its next launch; retain its native ID and fail clearly if it cannot resume.
  The installed app-server rebuilt the same turn and item counts from a copied
  paginated rollout without copying SQLite. Original history remains intact.
  Archive existing inactive Hypercube threads through the native API only after
  preserving their rollouts; leave running agents alone.

## Implementation plan

1. Add `plugins/codex/storage.go` with direct helpers for source-home resolution,
   private directory creation, and the explicit shared-input links. Return clear
   filesystem errors. Keep process environment construction here and leave
   unrelated inherited variables, including `HOME`, intact.
2. Modify `plugins/codex/main.go` to launch using the selected storage home and
   per-launch SQLite override, and remove the ineffective analytics flag.
   Keep credential values out of logs and errors. Import only the saved
   conversation requested by this launch before resuming it privately.
3. Add `plugins/codex/storage_test.go` for inherited/default source selection,
   shared inputs resolving to originals, isolated sessions/database paths,
   repeated setup, and actual setup failures. Use temporary directories only.
4. Update `plugins/codex/main_test.go` protocol stand-ins to assert private
   CODEX_HOME, SQLite argument, unchanged HOME, durable start/resume IDs,
   private import of existing conversations, and resume failure without a
   replacement start. Keep native archival cleanup separate from launch behavior.

## Test plan

- A new thread persists and resumes within private storage without writing its
  thread artifacts to the source home; normal Recents cannot discover that home.
- Linked configuration and existing file credentials remain readable; changing
  either source file is reflected without copies. Native credential refresh
  retains the credential symlink and updates the source target.
- Existing conversation IDs remain resumable, with complete history. Archive
  cleanup targets only Hypercube-owned stopped conversations and never interrupts
  a running worker or the user's own conversations.
