import { useState } from "react";
import type { Session } from "./model";
import { request } from "./socket";

type SessionListProps = {
  workspaceId: string;
  sessions: Session[];
  activeSessionId: string | null;
  onSelect: (sessionId: string) => void;
};

function NewSession({ workspaceId, onCreated }: { workspaceId: string; onCreated: (sessionId: string) => void }) {
  const [error, setError] = useState<string | null>(null);

  const create = () => {
    setError(null);
    request({ t: "createSession", workspaceId })
      .then(id => onCreated(id!))
      .catch((reason: Error) => setError(reason.message));
  };

  return (
    <div className="px-4 py-2">
      <button type="button" onClick={create} className="w-full rounded bg-neutral-700 px-2 py-1 hover:bg-neutral-600">
        New session
      </button>
      {error && <div className="mt-2 text-xs text-red-300">{error}</div>}
    </div>
  );
}

export function SessionList({ workspaceId, sessions, activeSessionId, onSelect }: SessionListProps) {
  return (
    <div>
      <NewSession workspaceId={workspaceId} onCreated={onSelect} />
      <ul>
        {sessions.map(session => {
          const active = session.id === activeSessionId;
          return (
            <li key={session.id} className={active ? "bg-[#37373d] text-neutral-100" : "hover:bg-[#2a2d2e]"}>
              <button type="button" aria-current={active} onClick={() => onSelect(session.id)} className="block w-full truncate py-1.5 pl-4 text-left">
                {session.name}
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
