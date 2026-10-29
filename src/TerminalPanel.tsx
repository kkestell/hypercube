import { useEffect, useRef, type ReactNode } from "react";
import type { IDockviewPanelProps } from "dockview-react";
import { HARNESS_LABELS, type Agent } from "./model";
import { useModel } from "./modelStore";
import { request } from "./terminalSocket";
import { getOrCreateView } from "./terminals";

/**
 * Shows a view's terminal, or `overlay` while there is none. The view, and so
 * its screen, outlives this component.
 */
export function TerminalHost({ viewId, terminalId, active, overlay }: { viewId: string; terminalId: string | null; active: boolean; overlay?: ReactNode }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const view = getOrCreateView(viewId);

  useEffect(() => {
    const container = containerRef.current!;
    container.appendChild(view.element);
    const resizeObserver = new ResizeObserver(() => view.fit());
    resizeObserver.observe(container);
    return () => {
      resizeObserver.disconnect();
      view.element.remove();
    };
  }, [view]);

  useEffect(() => view.setTerminal(terminalId), [view, terminalId]);

  useEffect(() => {
    if (active && terminalId) view.focus();
  }, [view, active, terminalId]);

  return (
    <div className="relative h-full w-full">
      <div ref={containerRef} className={`h-full w-full overflow-hidden bg-[#1e1e1e] pl-2 pt-1 ${terminalId ? "" : "invisible"}`} />
      {!terminalId && overlay && <div className="absolute inset-0 flex items-center justify-center bg-[#1e1e1e]">{overlay}</div>}
    </div>
  );
}

export const button = "rounded bg-neutral-700 px-3 py-1 text-neutral-100 hover:bg-neutral-600";

function run(message: Parameters<typeof request>[0]) {
  request(message).catch((error: Error) => alert(error.message));
}

function AgentOverlay({ agent }: { agent: Agent }) {
  const label = HARNESS_LABELS[agent.harness];
  switch (agent.state) {
    case "failed":
      return (
        <div className="flex flex-col items-center gap-3 text-sm text-neutral-300">
          <div className="text-red-300">{label} failed</div>
          {agent.failure && <div className="max-w-md text-center text-neutral-400">{agent.failure}</div>}
          <button type="button" className={button} onClick={() => run({ t: "retryAgent", agentId: agent.id })}>
            Retry
          </button>
        </div>
      );
    case "restarting":
      return <div className="text-sm text-neutral-400">{label} exited. Restarting…</div>;
    case "stopping":
      return <div className="text-sm text-neutral-400">Stopping…</div>;
    default:
      return (
        <div className="flex flex-col items-center gap-3 text-sm text-neutral-400">
          <div>Session stopped</div>
          <button type="button" className={button} onClick={() => run({ t: "startSession", sessionId: agent.sessionId })}>
            Start
          </button>
        </div>
      );
  }
}

/** A dockview panel showing one agent, whichever process it currently has. */
export function AgentPanel({ api, params }: IDockviewPanelProps<{ agentId: string; active: boolean }>) {
  const agent = useModel().agents[params.agentId];
  if (!agent) return null;
  return (
    <TerminalHost
      viewId={agent.id}
      terminalId={agent.terminalId}
      active={params.active && api.isActive}
      overlay={<AgentOverlay agent={agent} />}
    />
  );
}
