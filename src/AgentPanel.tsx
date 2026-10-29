import type { IDockviewPanelProps } from "dockview-react";
import { useModel } from "./modelStore";

/** A dockview panel for one agent. Empty for now. */
export function AgentPanel({ params }: IDockviewPanelProps<{ agentId: string }>) {
  const agent = useModel().agents[params.agentId];
  if (!agent) return null;
  return <div className="flex h-full items-center justify-center bg-[#1e1e1e] text-sm text-neutral-500">{agent.name}</div>;
}
