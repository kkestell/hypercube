import { useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import type { DockviewApi } from "dockview-react";
import { Group, Panel, Separator, usePanelRef, type PanelImperativeHandle } from "react-resizable-panels";
import { AgentsPanel } from "./AgentsPanel";
import { SessionList } from "./SessionList";
import type { Workspace } from "./model";
import { sessionsOf, useModel } from "./modelStore";
import { SessionView } from "./SessionView";
import { TerminalPanel } from "./TerminalPanel";

type Side = "left" | "bottom" | "right";

// Panels default to overflow: auto, which flashes scrollbars while their content catches up to a resize.
const panelStyle = { overflow: "hidden" } as const;

function togglePanel(panelRef: RefObject<PanelImperativeHandle | null>) {
  const panel = panelRef.current;
  if (!panel) return;
  if (panel.isCollapsed()) panel.expand();
  else panel.collapse();
}

function LayoutIcon({ side, open }: { side: Side; open: boolean }) {
  const fill = open ? "currentColor" : "none";
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1">
      <rect x="1.5" y="2.5" width="13" height="11" rx="1" />
      {side === "left" && <rect x="1.5" y="2.5" width="4.5" height="11" fill={fill} />}
      {side === "right" && <rect x="10" y="2.5" width="4.5" height="11" fill={fill} />}
      {side === "bottom" && <rect x="1.5" y="9" width="13" height="4.5" fill={fill} />}
    </svg>
  );
}

function OverviewIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1">
      <rect x="1.5" y="2.5" width="5.5" height="4.5" rx="0.5" />
      <rect x="9" y="2.5" width="5.5" height="4.5" rx="0.5" />
      <rect x="1.5" y="9" width="5.5" height="4.5" rx="0.5" />
      <rect x="9" y="9" width="5.5" height="4.5" rx="0.5" />
    </svg>
  );
}

function ToggleButton({ side, open, onClick }: { side: Side; open: boolean; onClick: () => void }) {
  const label = `${open ? "Hide" : "Show"} ${side[0]!.toUpperCase()}${side.slice(1)} Panel`;
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={open}
      onClick={onClick}
      className="rounded p-1 text-neutral-400 hover:bg-neutral-700 hover:text-neutral-100"
    >
      <LayoutIcon side={side} open={open} />
    </button>
  );
}

function SidePanel({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="flex h-full flex-col bg-[#252526]">
      <div className="px-4 py-2 text-[11px] font-semibold tracking-wide text-neutral-400 uppercase">{title}</div>
      <div className="min-h-0 flex-1 overflow-auto">{children}</div>
    </div>
  );
}

function ResizeHandle({ orientation }: { orientation: "horizontal" | "vertical" }) {
  const size = orientation === "horizontal" ? "w-px" : "h-px";
  return <Separator className={`${size} bg-neutral-700 transition-colors data-[separator=hover]:bg-sky-500 data-[separator=active]:bg-sky-500`} />;
}

type WorkspaceViewProps = {
  workspace: Workspace;
  onShowOverview: () => void;
};

/** A workspace: its sessions, the active session's agents, and the side panels. */
export function WorkspaceView({ workspace, onShowOverview }: WorkspaceViewProps) {
  const model = useModel();
  const sessions = sessionsOf(model, workspace.id);
  const leftRef = usePanelRef();
  const bottomRef = usePanelRef();
  const rightRef = usePanelRef();
  const [open, setOpen] = useState<Record<Side, boolean>>({ left: true, bottom: false, right: false });
  // The bottom and right panels start hidden. Collapsing them after layout, rather than giving them no default size,
  // lets the first expand restore their default size.
  useLayoutEffect(() => {
    bottomRef.current?.collapse();
    rightRef.current?.collapse();
  }, []);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(sessions[0]?.id ?? null);
  const activeSession = sessions.find(session => session.id === selectedSessionId) ?? sessions[0] ?? null;
  const activeSessionId = activeSession?.id ?? null;
  // Sessions mount on first visit and stay mounted, so their views keep their layouts.
  const [openedSessionIds, setOpenedSessionIds] = useState(() => new Set(activeSessionId ? [activeSessionId] : []));

  const docks = useRef(new Map<string, DockviewApi>());
  const showAgent = (agentId: string) => docks.current.get(activeSessionId!)?.getPanel(agentId)?.api.setActive();
  const anyWaiting = Object.values(model.agents).some(agent => agent.status === "waiting");

  const selectSession = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    setOpenedSessionIds(current => (current.has(sessionId) ? current : new Set(current).add(sessionId)));
  };

  const trackOpen = (side: Side) => (size: { inPixels: number }) =>
    setOpen(current => (current[side] === size.inPixels > 0 ? current : { ...current, [side]: size.inPixels > 0 }));

  return (
    <div className="flex h-full flex-col text-sm text-neutral-300">
      <header className="flex h-9 shrink-0 items-center gap-1 border-b border-neutral-700 bg-[#323233] px-2">
        <button
          type="button"
          title="Show Overview"
          aria-label="Show Overview"
          onClick={onShowOverview}
          className="relative rounded p-1 text-neutral-400 hover:bg-neutral-700 hover:text-neutral-100"
        >
          <OverviewIcon />
          {anyWaiting && <span title="An agent is waiting on you" className="absolute top-0.5 right-0.5 size-2 rounded-full bg-amber-400" />}
        </button>
        <span className="ml-1 flex-1 truncate text-neutral-400">{workspace.repo}</span>
        <ToggleButton side="left" open={open.left} onClick={() => togglePanel(leftRef)} />
        <ToggleButton side="bottom" open={open.bottom} onClick={() => togglePanel(bottomRef)} />
        <ToggleButton side="right" open={open.right} onClick={() => togglePanel(rightRef)} />
      </header>

      <Group orientation="horizontal" className="min-h-0 flex-1">
        <Panel
          id={`${workspace.id}-left`}
          style={panelStyle}
          panelRef={leftRef}
          collapsible
          defaultSize={260}
          minSize={160}
          groupResizeBehavior="preserve-pixel-size"
          onResize={trackOpen("left")}
        >
          <SidePanel title="Sessions">
            <SessionList workspaceId={workspace.id} sessions={sessions} activeSessionId={activeSessionId} onSelect={selectSession} />
          </SidePanel>
        </Panel>
        <ResizeHandle orientation="horizontal" />

        <Panel id={`${workspace.id}-center`} style={panelStyle} minSize={200}>
          <Group orientation="vertical">
            <Panel id={`${workspace.id}-content`} style={panelStyle} minSize={100}>
              <div className="relative h-full">
                {sessions
                  .filter(session => openedSessionIds.has(session.id) || session.id === activeSessionId)
                  .map(session => {
                    const shown = session.id === activeSessionId;
                    // Hidden sessions keep their size so their layouts stay put.
                    return (
                      <div
                        key={session.id}
                        inert={!shown}
                        className={`absolute inset-0 ${shown ? "" : "invisible-subtree"}`}
                      >
                        <SessionView session={session} onApi={api => docks.current.set(session.id, api)} />
                      </div>
                    );
                  })}
              </div>
            </Panel>
            <ResizeHandle orientation="vertical" />
            <Panel
              id={`${workspace.id}-bottom`}
              style={panelStyle}
              panelRef={bottomRef}
              collapsible
              defaultSize={220}
              minSize={100}
              groupResizeBehavior="preserve-pixel-size"
              onResize={trackOpen("bottom")}
            >
              <TerminalPanel viewId={workspace.id} session={activeSession} />
            </Panel>
          </Group>
        </Panel>

        <ResizeHandle orientation="horizontal" />
        <Panel
          id={`${workspace.id}-right`}
          style={panelStyle}
          panelRef={rightRef}
          collapsible
          defaultSize={260}
          minSize={160}
          groupResizeBehavior="preserve-pixel-size"
          onResize={trackOpen("right")}
        >
          <SidePanel title="Agents">
            <AgentsPanel session={activeSession} onSelect={showAgent} />
          </SidePanel>
        </Panel>
      </Group>
    </div>
  );
}
