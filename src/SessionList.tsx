import { useState } from "react";
import type { Session, UnsavedWork } from "./model";
import { useModel } from "./modelStore";
import { activity } from "./status";
import { request, RequestError } from "./socket";

type SessionListProps = {
  workspaceId: string;
  sessions: Session[];
  activeSessionId: string | null;
  onSelect: (sessionId: string) => void;
};

function NewSession({ workspaceId, onCreated }: { workspaceId: string; onCreated: (sessionId: string) => void }) {
  const [error, setError] = useState<string | null>(null);
  const [environment, setEnvironment] = useState<Session["environment"]>("local");

  const create = () => {
    setError(null);
    request({ t: "createSession", workspaceId, environment })
      .then(id => onCreated(id!))
      .catch((reason: Error) => setError(reason.message));
  };

  return (
    <div className="px-4 py-2">
      <div className="flex gap-1">
        <button type="button" onClick={create} className="min-w-0 flex-1 rounded bg-neutral-700 px-2 py-1 hover:bg-neutral-600">
          New session
        </button>
        <select aria-label="Environment" value={environment} onChange={event => setEnvironment(event.target.value as Session["environment"])} className="rounded bg-neutral-700 px-1 py-1">
          <option value="local">Local</option>
          <option value="docker">Docker</option>
        </select>
      </div>
      {error && <div className="mt-2 text-xs text-red-300">{error}</div>}
    </div>
  );
}

const SHOWN = 8;

/** What a refused delete would lose, as the text of the confirmation. */
function describeUnsaved(unsaved: UnsavedWork[]) {
  const some = (lines: string[]) => [...lines.slice(0, SHOWN), ...(lines.length > SHOWN ? [`…and ${lines.length - SHOWN} more`] : [])].map(line => `    ${line}`);
  return unsaved
    .flatMap(({ path, uncommitted, unpushed, error }) => [
      path === "." ? "In the clone:" : `In ${path}:`,
      ...(error ? [`  Could not check: ${error}`] : []),
      ...(uncommitted.length ? ["  Uncommitted changes:", ...some(uncommitted)] : []),
      ...(unpushed.length ? ["  Commits on no remote branch:", ...some(unpushed)] : []),
    ])
    .join("\n");
}

function SessionRow({ session, active, onSelect }: { session: Session; active: boolean; onSelect: () => void }) {
  const { agents } = useModel();
  const { busy: working, waiting } = activity(session.agentIds.map(id => agents[id]));
  const names = (list: typeof working) => list.map(agent => agent.name).join(", ");
  const [busy, setBusy] = useState<"deleting" | "stopping" | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = (doing: "deleting" | "stopping" | null, ask: Promise<unknown>) => {
    setBusy(doing);
    setError(null);
    return ask.catch((reason: Error) => setError(reason.message)).finally(() => setBusy(null));
  };
  const remove = () => {
    setBusy("deleting");
    setError(null);
    request({ t: "deleteSession", sessionId: session.id })
      .catch(async (reason: Error) => {
        // Unsaved work: the user decides, and a yes deletes it anyway.
        if (!(reason instanceof RequestError && reason.unsaved)) throw reason;
        if (!confirm(`${session.name} has work that is not on GitHub. Deleting it loses this:\n\n${describeUnsaved(reason.unsaved)}\n\nDelete anyway?`)) return;
        await request({ t: "deleteSession", sessionId: session.id, confirmed: true });
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setBusy(null));
  };
  const stop = () => run("stopping", request({ t: "stopSession", sessionId: session.id }));
  const start = () => run(null, request({ t: "startSession", sessionId: session.id }));

  return (
    <li className={active ? "bg-[#37373d] text-neutral-100" : "hover:bg-[#2a2d2e]"}>
      <div className="flex items-center gap-1 pr-2">
        <button type="button" aria-current={active} onClick={onSelect} className="min-w-0 flex-1 py-1.5 pl-4 text-left">
          <span className="block truncate">{session.name}</span>
          {waiting.length > 0 && (
            <span className="flex items-center gap-1.5 text-xs text-amber-300">
              <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-amber-400" />
              <span className="truncate">Waiting on you: {names(waiting)}</span>
            </span>
          )}
          {working.length > 0 && <span className="block truncate text-xs text-sky-300">Busy: {names(working)}</span>}
          {waiting.length === 0 && working.length === 0 && (
            <span className="flex items-center gap-1.5 text-xs text-neutral-400">
              {session.status === "cloning" && <span aria-hidden="true" className="size-3 animate-spin rounded-full border-2 border-neutral-500 border-t-sky-400 motion-reduce:animate-none" />}
              {session.status === "cloning" ? "Cloning…" : session.status === "failed" ? "Failed" : session.status === "stopped" ? "Stopped" : "Ready"}
            </span>
          )}
        </button>
        {session.status === "ready" && (
          <button
            type="button"
            aria-label={`Stop ${session.name}`}
            title="Stop its agents. Start resumes them."
            disabled={busy !== null}
            onClick={stop}
            className="rounded px-1.5 py-1 text-xs text-neutral-400 hover:bg-neutral-600 hover:text-neutral-100 disabled:opacity-40"
          >
            {busy === "stopping" ? "Stopping…" : "Stop"}
          </button>
        )}
        {session.status === "stopped" && (
          <button
            type="button"
            aria-label={`Start ${session.name}`}
            title="Resume its agents"
            onClick={start}
            className="rounded px-1.5 py-1 text-xs text-neutral-400 hover:bg-neutral-600 hover:text-neutral-100 disabled:opacity-40"
          >
            Start
          </button>
        )}
        <button
          type="button"
          aria-label={`Delete ${session.name}`}
          title={session.status === "cloning" ? "Wait for cloning to finish" : "Delete session and clone"}
          disabled={busy !== null || session.status === "cloning"}
          onClick={remove}
          className="rounded px-1.5 py-1 text-xs text-neutral-400 hover:bg-neutral-600 hover:text-red-200 disabled:opacity-40"
        >
          {busy === "deleting" ? "Deleting…" : "Delete"}
        </button>
      </div>
      {(session.error || error) && <div className="px-4 pb-2 text-xs break-words whitespace-pre-wrap text-red-300">{error || session.error}</div>}
    </li>
  );
}

export function SessionList({ workspaceId, sessions, activeSessionId, onSelect }: SessionListProps) {
  return (
    <div>
      <NewSession workspaceId={workspaceId} onCreated={onSelect} />
      <ul>
        {sessions.map(session => (
          <SessionRow key={session.id} session={session} active={session.id === activeSessionId} onSelect={() => onSelect(session.id)} />
        ))}
      </ul>
    </div>
  );
}
