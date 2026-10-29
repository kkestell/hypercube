import { useEffect, useState } from "react";
import { layoutCards, type CardPlacement, type Size } from "./overviewLayout";
import { WorkspaceView } from "./WorkspaceView";
import { stubWorkspaces, type Workspace } from "./workspaces";
import "./index.css";

const transition = "duration-350 ease-[cubic-bezier(0.2,0,0,1)] motion-reduce:transition-none";

function useViewportSize(): Size {
  const [size, setSize] = useState(() => ({ width: window.innerWidth, height: window.innerHeight }));
  useEffect(() => {
    const onResize = () => setSize({ width: window.innerWidth, height: window.innerHeight });
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);
  return size;
}

type WorkspaceFrameProps = {
  workspace: Workspace;
  card: CardPlacement;
  viewport: Size;
  active: boolean;
  overview: boolean;
  /** The workspace has been opened, so its sessions are mounted. */
  opened: boolean;
  onSelect: () => void;
  onShowOverview: () => void;
};

/**
 * A workspace laid out at full window size. In overview it is scaled down
 * into its card, so its content never reflows, and cross-fades from the
 * workspace UI to the card view.
 */
function WorkspaceFrame({ workspace, card, viewport, active, overview, opened, onSelect, onShowOverview }: WorkspaceFrameProps) {
  const fullscreen = active && !overview;
  const shown = active || overview;
  const transform = fullscreen
    ? "translate(0px, 0px) scale(1)"
    : `translate(${card.x}px, ${card.y}px) scale(${card.scale})`;

  return (
    <div
      inert={!shown}
      className={`absolute top-0 left-0 origin-top-left overflow-hidden transition-[transform,opacity,border-radius] ${transition}`}
      style={{
        width: viewport.width,
        height: viewport.height,
        transform,
        opacity: shown ? 1 : 0,
        borderRadius: fullscreen ? 0 : 10 / card.scale,
        zIndex: active ? 1 : 0,
      }}
    >
      <div
        inert={!fullscreen}
        className={`h-full transition-opacity ${transition} ${active ? "" : "invisible-subtree"}`}
        style={{ opacity: fullscreen ? 1 : 0 }}
      >
        {opened && <WorkspaceView workspace={workspace} active={fullscreen} onShowOverview={onShowOverview} />}
      </div>

      <button
        type="button"
        inert={!overview}
        onClick={onSelect}
        className={`absolute inset-0 flex items-center justify-center rounded-[inherit] bg-[#252526] text-neutral-200 transition-opacity ${transition} ${
          overview ? "" : "pointer-events-none"
        } ${active ? "[--ring-color:var(--color-sky-500)]" : "hover:[--ring-color:var(--color-neutral-500)]"}`}
        style={{
          opacity: overview ? 1 : 0,
          fontSize: 18 / card.scale,
          boxShadow: `inset 0 0 0 ${2 / card.scale}px var(--ring-color, transparent)`,
        }}
      >
        {workspace.name}
      </button>
    </div>
  );
}

export function App() {
  const viewport = useViewportSize();
  const [activeWorkspaceId, setActiveWorkspaceId] = useState(stubWorkspaces[0]!.id);
  const [overview, setOverview] = useState(false);
  // Workspaces mount on first visit and stay mounted, so their terminals keep running in the background.
  const [openedWorkspaceIds, setOpenedWorkspaceIds] = useState(() => new Set([activeWorkspaceId]));
  const cards = layoutCards(stubWorkspaces.length, viewport);

  const openWorkspace = (workspaceId: string) => {
    setActiveWorkspaceId(workspaceId);
    setOpenedWorkspaceIds(current => (current.has(workspaceId) ? current : new Set(current).add(workspaceId)));
    setOverview(false);
  };

  useEffect(() => {
    if (!overview) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOverview(false);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [overview]);

  return (
    <div
      className="relative h-screen overflow-hidden bg-[#141414]"
      onClick={event => {
        if (overview && event.target === event.currentTarget) setOverview(false);
      }}
    >
      {stubWorkspaces.map((workspace, index) => (
        <WorkspaceFrame
          key={workspace.id}
          workspace={workspace}
          card={cards[index]!}
          viewport={viewport}
          active={workspace.id === activeWorkspaceId}
          overview={overview}
          opened={openedWorkspaceIds.has(workspace.id)}
          onSelect={() => openWorkspace(workspace.id)}
          onShowOverview={() => setOverview(true)}
        />
      ))}
    </div>
  );
}

export default App;
