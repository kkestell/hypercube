import { useEffect, useRef } from "react";
import { DockviewDefaultTab, DockviewReact, themeDark, type DockviewApi, type IDockviewPanelHeaderProps } from "dockview-react";
import "dockview-react/dist/styles/dockview.css";
import type { Session } from "./model";
import { useModel } from "./modelStore";
import { AgentPanel } from "./AgentPanel";

const components = { agent: AgentPanel };

// Agents are part of the session; their tabs can be moved but not closed.
const Tab = (props: IDockviewPanelHeaderProps) => <DockviewDefaultTab {...props} hideClose />;

/** A session's agents, arranged in tabs and splits. */
export function SessionView({ session }: { session: Session }) {
  const apiRef = useRef<DockviewApi | null>(null);
  const { agents } = useModel();

  // Adds a tab for each agent the session gained (all of them the first time).
  const syncPanels = () => {
    const api = apiRef.current;
    if (!api) return;
    for (const agentId of session.agentIds) {
      const agent = agents[agentId];
      if (!agent || api.getPanel(agentId)) continue;
      api.addPanel({ id: agentId, component: "agent", title: agent.name, params: { agentId } });
    }
  };

  useEffect(syncPanels, [session.agentIds, agents]);

  return (
    <DockviewReact
      theme={themeDark}
      components={components}
      defaultTabComponent={Tab}
      onReady={({ api }) => {
        apiRef.current = api;
        syncPanels();
      }}
    />
  );
}
