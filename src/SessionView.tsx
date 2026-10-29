import { useEffect, useRef } from "react";
import {
  DockviewReact,
  themeDark,
  type DockviewApi,
  type DockviewGroupPanel,
  type IDockviewHeaderActionsProps,
  type IWatermarkPanelProps,
} from "dockview-react";
import "dockview-react/dist/styles/dockview.css";
import type { Session } from "./sessions";
import { TerminalPanel } from "./TerminalPanel";
import { disposeTerminal, focusTerminal } from "./terminals";

const NEW_TERMINAL_SHORTCUT = "Ctrl+Shift+`";

let terminalCount = 0;

/** Adds a terminal as a new tab in the given group, or in the active group. */
function addTerminal(api: DockviewApi, group?: DockviewGroupPanel) {
  terminalCount += 1;
  api.addPanel({
    id: `terminal-${terminalCount}`,
    component: "terminal",
    title: `Terminal ${terminalCount}`,
    position: group ? { referenceGroup: group } : undefined,
  });
}

/** Lays out a session's agents as terminal tabs. */
function addAgents(api: DockviewApi, session: Session) {
  const panelId = (agentId: string) => `${session.id}/${agentId}`;
  for (const agent of session.agents) {
    api.addPanel({
      id: panelId(agent.id),
      component: "terminal",
      title: agent.name,
      position: agent.position && {
        referencePanel: panelId(agent.position.relativeTo),
        direction: agent.position.direction,
      },
    });
  }
}

function isNewTerminalShortcut(event: KeyboardEvent) {
  return event.ctrlKey && event.shiftKey && !event.altKey && !event.metaKey && event.code === "Backquote";
}

function NewTerminalAction({ containerApi, group }: IDockviewHeaderActionsProps) {
  return (
    <button
      type="button"
      title={`New Terminal (${NEW_TERMINAL_SHORTCUT})`}
      onClick={() => addTerminal(containerApi, group)}
      className="flex h-full items-center px-2 text-neutral-400 hover:text-neutral-100"
    >
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M8 3v10M3 8h10" />
      </svg>
    </button>
  );
}

function Watermark({ containerApi }: IWatermarkPanelProps) {
  return (
    <div className="flex h-full items-center justify-center">
      <button
        type="button"
        onClick={() => addTerminal(containerApi)}
        className="rounded px-3 py-2 text-sm text-neutral-400 hover:bg-neutral-800 hover:text-neutral-200"
      >
        New Terminal <kbd className="ml-2 font-mono text-neutral-500">{NEW_TERMINAL_SHORTCUT}</kbd>
      </button>
    </div>
  );
}

const components = { terminal: TerminalPanel };

/** A session's agents, arranged in tabs and splits. */
export function SessionView({ session, active }: { session: Session; active: boolean }) {
  const apiRef = useRef<DockviewApi | null>(null);

  useEffect(() => {
    if (!active) return;
    const activePanel = apiRef.current?.activePanel;
    if (activePanel) focusTerminal(activePanel.id);

    // Capture phase, so the shortcut works while a terminal has focus.
    const onKeyDown = (event: KeyboardEvent) => {
      if (!isNewTerminalShortcut(event) || !apiRef.current) return;
      event.preventDefault();
      event.stopPropagation();
      addTerminal(apiRef.current);
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [active]);

  return (
    <DockviewReact
      theme={themeDark}
      components={components}
      watermarkComponent={Watermark}
      rightHeaderActionsComponent={NewTerminalAction}
      onReady={({ api }) => {
        apiRef.current = api;
        api.onDidRemovePanel(panel => {
          // Moving a panel between groups also removes it from its old group,
          // so only end the shell once the panel is gone from the layout.
          setTimeout(() => {
            if (!api.getPanel(panel.id)) disposeTerminal(panel.id);
          });
        });
        addAgents(api, session);
      }}
    />
  );
}
