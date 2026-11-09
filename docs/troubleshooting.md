# Troubleshoot Hypercube

Find the symptom you see, then follow its fix. Most problems leave a message
in the waiting page, in chat, or in the service log.

## Read the service log

The background service writes its output and errors to
`~/.local/state/hypercube/ui.log`; the waiting page shows the path in use. See
[Files and storage](storage.md#hypercube-files) for other locations.

Show the most recent lines:

```sh
tail -n 50 ~/.local/state/hypercube/ui.log
```

Each start of the service writes a line naming its build and ending in
`serving at http://localhost:4280/`. A line starting with `hypercube:` is an
error.

## The waiting page does not go away

**Symptom:** the window shows **Waiting for Hypercube** and says the service is
not answering at `http://localhost:4280/`.

**Cause:** the desktop app shows its workspaces only after the background
service answers. The page names what is missing below that line.

**Fix:** follow the message on the page:

| Message | Fix |
| --- | --- |
| **This app is running from a temporary location.** | macOS only. Quit Hypercube, move `Hypercube.app` to `~/Applications`, and open it from there. |
| **Install Hypercube with** a path ending in `resources/install` | Linux only. You opened the extracted copy. Run the command shown, then open Hypercube from the application launcher. |
| **Set Up Background Service starts Hypercube at login** | The service is not set up yet. Choose **Set Up Background Service**. |
| **The service is installed but not answering.** | Read the [service log](#read-the-service-log) for the cause, fix it, then run the command shown on the page to start the service. |

When **Set Up Background Service** fails, the page shows the installer's error
in red below the button. A port conflict is the most common;
see [Port 4280 is already in use](#port-4280-is-already-in-use).

## Port 4280 is already in use

**Symptom:** setup fails with an error below the button.

**macOS:**

```text
install-service: port 4280 is in use by <command>
install-service: stop it with: kill <process ID>
```

**Linux:**

```text
install-service: port 4280 is in use: <process details>
```

Or the waiting page says the service is installed but not answering, and the
service log contains:

```text
hypercube: listen tcp 127.0.0.1:4280: bind: address already in use
```

**Cause:** another program listens on port 4280, which the background service
needs.

**Fix:**

1. Identify the program from the error. If the error does not name it, find
   it:
   - **macOS:** `lsof -nP -iTCP:4280 -sTCP:LISTEN`
   - **Linux:** `ss -ltnp 'sport = :4280'`
2. Stop that program.
3. **macOS:** choose **Set Up Background Service** again. **Linux:** run the
   start command the waiting page shows.

## A harness is missing

**Symptom:** a session's chat shows an error such as:

```text
Could not start the agent: Claude Code is missing: install claude
```

**Cause:** the background service cannot find the harness's executable
(`claude`, `opencode`, or `pi`) on its `PATH`. The service starts through your
login shell, so it uses the `PATH` your shell configuration sets, read when the
service started.

**Fix:**

1. Install and sign in to the harness; see
   [harness requirements](harness-support.md#requirements).
2. Check that your login shell finds it. Replace `claude` with the harness's
   executable:

   ```sh
   "$SHELL" -l -i -c 'command -v claude'
   ```

3. If you changed your shell configuration to add the executable's folder to
   `PATH`, reload the background service with the command in step 3 of
   [Replace the app](upgrades.md#replace-the-app) for your platform. Running
   agents keep running.
4. Open the session or send a prompt to retry.

## A model or effort is rejected

**Symptom:** starting a session, or changing its model, shows an error in chat
such as:

```text
Could not start the agent: Claude Code does not offer model "claude-opsu"; see list_models
```

Other forms name the problem directly: the model needs an effort, does not
accept the chosen effort, or, for Pi, is not a full `provider/id`.

**Cause:** the plugin checks the model and effort against the harness at start
and at each model change. An entry in **Settings › Agents › Models** must use
the harness's exact model ID and offered effort levels.

**Fix:**

1. Correct the entry in **Settings › Agents › Models**; see the
   [model ID forms](harness-support.md#requirements) for each harness. Choose
   **Save changes**.
2. For a rejected model change, choose the corrected model below the message
   box. The selector shows the settings the harness accepted.
3. For a session that could not start, the selector stays unavailable because
   the agent is not running. Choose **Delete Session** in its menu, then start
   a new session with the corrected model.

## A session fails to start or resume

**Symptom:** chat shows `Could not start the agent:` or `The agent exited:`
followed by a reason, and the session stays dimmed.

**Cause:** the reason names it. Common causes are a missing harness, a rejected
model, or a native conversation the harness can no longer resume. An agent
that fails to start is not relaunched automatically, and one that exits three
times in a row within 10 seconds of starting stays stopped.

**Fix:**

1. Fix the cause the message names. For a missing harness or rejected model,
   see the sections above.
2. Open the session or send a prompt to retry.

When the harness can no longer resume the conversation, start a new session to
continue the work. See
[What keeps running when you leave](lifecycle.md#a-saved-session-is-not-always-a-running-process).

## An agent behaves like the old version after an upgrade

**Symptom:** after an upgrade, a session that kept running still shows
behavior the release changed.

**Cause:** an agent that survived the upgrade keeps the plugin it started with.

**Fix:** choose **Restart Agent** in the session's context menu. See
[Load new plugins into existing sessions](upgrades.md#load-new-plugins-into-existing-sessions).

## Archive or delete is refused

**Symptom:** **Archive Session**, **Delete Session**, or **Delete Workspace**
shows an error such as:

```text
cannot remove worktrees:
  Fix login: worktree /path/to/worktree has uncommitted changes
```

Other reasons are `has commits not merged into HEAD` and
`branch <name> is not merged into HEAD`.

**Cause:** a managed worktree fails the Git checks in
[session cleanup](storage.md#session-cleanup).

**Fix:**

1. Open the named session and inspect its changes in the Git panel.
2. Commit the files you want to keep, or discard them.
3. Merge the commits you want to keep as that section describes; see
   [Merge and finish a child's work](teamwork.md#merge-and-finish-a-childs-work).
4. Choose the action again. For how a refusal affects **Delete Workspace**,
   see [Delete a workspace](usage.md#delete-a-workspace).

## macOS blocks Hypercube from opening

**Symptom:** macOS refuses to open `Hypercube.app` the first time.

**Fix:** allow the app as described in step 3 of
[Install on macOS](installation.md#install-on-macos).
