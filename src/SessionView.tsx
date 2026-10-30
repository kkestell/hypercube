import { useEffect, useRef } from "react";
import { DockviewDefaultTab, DockviewReact, themeDark, type DockviewApi, type IDockviewPanelHeaderProps } from "dockview-react";
import "dockview-react/dist/styles/dockview.css";
import type { Session } from "./model";
import { agentsOf, useModel } from "./modelStore";
import { AgentPanel } from "./AgentPanel";
import { request } from "./socket";

const components = { agent: AgentPanel };

// Agents are part of the session; their tabs can be moved but not closed. A worker's tab has a remove action instead.
// A tab carries no status text: only a marker when the agent waits on the user.
const Tab = (props: IDockviewPanelHeaderProps) => {
  const agent = useModel().agents[props.api.id];
  const remove = () => {
    if (!confirm(`Remove ${agent?.name}? Its process stops and its transcript is lost.`)) return;
    request({ t: "removeWorker", agentId: props.api.id }).catch((reason: Error) => alert(reason.message));
  };
  return (
    <div className="flex items-center">
      <DockviewDefaultTab {...props} hideClose />
      {agent?.status === "waiting" && <span role="img" aria-label="Waiting on you" title="Waiting on you" className="mr-2 size-2 shrink-0 rounded-full bg-amber-400" />}
      {agent?.role === "worker" && (
        <button type="button" title="Remove worker" onClick={remove} className="mr-2 text-neutral-400 hover:text-neutral-100">
          ×
        </button>
      )}
    </div>
  );
};

/** A session's agents, arranged in tabs and splits. `onApi` gets the layout, so the workspace can show an agent's tab. */
export function SessionView({ session, onApi }: { session: Session; onApi: (api: DockviewApi) => void }) {
  const apiRef = useRef<DockviewApi | null>(null);
  const model = useModel();

  // Adds a tab for each agent the session gained (all of them the first time) and drops the tabs of removed workers.
  const syncPanels = () => {
    const api = apiRef.current;
    if (!api) return;
    const sessionAgents = agentsOf(model, session.id);
    for (const panel of [...api.panels]) if (!sessionAgents.some(agent => agent.id === panel.id)) api.removePanel(panel);
    for (const agent of sessionAgents) {
      if (!api.getPanel(agent.id)) api.addPanel({ id: agent.id, component: "agent", title: agent.name, params: { agentId: agent.id } });
    }
  };

  useEffect(syncPanels, [model.agents]);

  return (
    <DockviewReact
      theme={themeDark}
      components={components}
      defaultTabComponent={Tab}
      onReady={({ api }) => {
        apiRef.current = api;
        onApi(api);
        syncPanels();
      }}
    />
  );
}
