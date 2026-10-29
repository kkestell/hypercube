import { useState } from "react";
import { HARNESS_LABELS, type Harness, type Session } from "./model";
import { useModel } from "./modelStore";
import { request } from "./terminalSocket";

type SessionListProps = {
  workspaceId: string;
  sessions: Session[];
  activeSessionId: string | null;
  onSelect: (sessionId: string) => void;
};

const small = "rounded px-1.5 text-xs text-neutral-400 hover:bg-neutral-700 hover:text-neutral-100";

/** Picks the lead's harness and creates a session. */
function NewSession({ workspaceId, onCreated }: { workspaceId: string; onCreated: (sessionId: string) => void }) {
  const [picking, setPicking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const create = (harness: Harness) => {
    setError(null);
    request({ t: "createSession", workspaceId, harness })
      .then(id => {
        setPicking(false);
        onCreated(id!);
      })
      .catch((reason: Error) => setError(reason.message));
  };

  return (
    <div className="px-4 py-2">
      {picking ? (
        <div className="flex flex-col gap-1">
          <div className="text-xs text-neutral-400">Lead:</div>
          {(Object.keys(HARNESS_LABELS) as Harness[]).map(harness => (
            <button key={harness} type="button" onClick={() => create(harness)} className="rounded bg-neutral-700 px-2 py-1 text-left hover:bg-neutral-600">
              {HARNESS_LABELS[harness]}
            </button>
          ))}
          <button type="button" onClick={() => setPicking(false)} className="px-2 py-1 text-left text-neutral-500 hover:text-neutral-300">
            Cancel
          </button>
        </div>
      ) : (
        <button type="button" onClick={() => setPicking(true)} className="w-full rounded bg-neutral-700 px-2 py-1 hover:bg-neutral-600">
          New session
        </button>
      )}
      {error && <div className="mt-2 text-xs text-red-300">{error}</div>}
    </div>
  );
}

export function SessionList({ workspaceId, sessions, activeSessionId, onSelect }: SessionListProps) {
  const { agents } = useModel();
  const run = (message: Parameters<typeof request>[0]) => request(message).catch((error: Error) => alert(error.message));

  return (
    <div>
      <NewSession workspaceId={workspaceId} onCreated={onSelect} />
      <ul>
        {sessions.map(session => {
          const active = session.id === activeSessionId;
          const lead = session.agentIds.map(id => agents[id]).find(agent => agent?.role === "lead");
          const failed = lead?.state === "failed";
          const status = failed ? "failed" : session.state;
          return (
            <li key={session.id} className={`group flex items-center ${active ? "bg-[#37373d] text-neutral-100" : "hover:bg-[#2a2d2e]"}`}>
              <button type="button" aria-current={active} onClick={() => onSelect(session.id)} className="min-w-0 flex-1 truncate py-1.5 pl-4 text-left">
                {session.name}
              </button>
              <span className={`mr-2 text-xs ${failed ? "text-red-300" : session.state === "running" ? "text-emerald-400" : "text-neutral-500"}`}>{status}</span>
              {failed && lead ? (
                <button type="button" className={`${small} mr-2`} onClick={() => run({ t: "retryAgent", agentId: lead.id })}>
                  Retry
                </button>
              ) : session.state === "running" ? (
                <button type="button" className={`${small} mr-2`} onClick={() => run({ t: "stopSession", sessionId: session.id })}>
                  Stop
                </button>
              ) : (
                <button type="button" className={`${small} mr-2`} onClick={() => run({ t: "startSession", sessionId: session.id })}>
                  Start
                </button>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
