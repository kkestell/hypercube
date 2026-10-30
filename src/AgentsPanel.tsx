import type { Agent, Session } from "./model";
import { agentsOf, useModel } from "./modelStore";

const statusLabels = {
  starting: ["Starting", "text-neutral-400"],
  idle: ["Idle", "text-neutral-400"],
  busy: ["Busy", "text-sky-300"],
  waiting: ["Waiting on you", "text-amber-300"],
  exited: ["Exited", "text-red-300"],
} as const;

function counts(agent: Agent) {
  const steers = agent.steer?.prompts.length ?? 0;
  return [agent.queue.length && `${agent.queue.length} queued`, steers && `${steers} steer`].filter(Boolean).join(" · ");
}

/** The session's agents with their status. Selecting one shows its tab. */
export function AgentsPanel({ session, onSelect }: { session: Session | null; onSelect: (agentId: string) => void }) {
  const model = useModel();
  const rows = session ? agentsOf(model, session.id) : [];
  if (rows.length === 0) return <div className="px-4 py-2 text-neutral-500">No agents</div>;

  return (
    <ul>
      {rows.map(agent => {
        // The agents of a stopped session are exited by our own doing, which isn't an error.
        const [label, color] = session!.status === "stopped" ? (["Stopped", "text-neutral-400"] as const) : statusLabels[agent.status];
        return (
          <li key={agent.id}>
            <button type="button" onClick={() => onSelect(agent.id)} className="block w-full px-4 py-1.5 text-left hover:bg-[#2a2d2e]">
              <span className="flex items-baseline justify-between gap-2">
                <span className="truncate text-neutral-100">{agent.name}</span>
                <span title={agent.error} className={`shrink-0 text-xs ${color}`}>
                  {label}
                </span>
              </span>
              <span className="block truncate text-xs text-neutral-400">
                {agent.role}
                {agent.profile && ` · ${agent.profile}`}
              </span>
              {counts(agent) && <span className="block truncate text-xs text-neutral-400">{counts(agent)}</span>}
            </button>
          </li>
        );
      })}
    </ul>
  );
}
