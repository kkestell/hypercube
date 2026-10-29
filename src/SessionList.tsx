import type { Session } from "./sessions";

type SessionListProps = {
  sessions: Session[];
  activeSessionId: string;
  onSelect: (sessionId: string) => void;
};

export function SessionList({ sessions, activeSessionId, onSelect }: SessionListProps) {
  return (
    <ul>
      {sessions.map(session => {
        const active = session.id === activeSessionId;
        return (
          <li key={session.id}>
            <button
              type="button"
              aria-current={active}
              onClick={() => onSelect(session.id)}
              className={`block w-full truncate px-4 py-1.5 text-left ${
                active ? "bg-[#37373d] text-neutral-100" : "hover:bg-[#2a2d2e]"
              }`}
            >
              {session.name}
            </button>
          </li>
        );
      })}
    </ul>
  );
}
