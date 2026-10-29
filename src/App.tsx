import { useEffect, useState, type FormEvent } from "react";
import type { Workspace } from "./model";
import { useModel } from "./modelStore";
import { layoutCards, type CardPlacement, type Size } from "./overviewLayout";
import { connect, request } from "./socket";
import { WorkspaceView } from "./WorkspaceView";
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
        {opened && <WorkspaceView workspace={workspace} onShowOverview={onShowOverview} />}
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

/** Asks for a directory and opens it as a workspace. */
function AddWorkspace({ onAdded }: { onAdded: (workspaceId: string) => void }) {
  const [editing, setEditing] = useState(false);
  const [path, setPath] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    request({ t: "addWorkspace", path })
      .then(id => {
        setEditing(false);
        setPath("");
        onAdded(id!);
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setBusy(false));
  };

  return (
    <div className="absolute inset-x-0 bottom-6 z-10 flex flex-col items-center gap-2 text-sm">
      {error && <div className="rounded bg-red-950 px-3 py-1 text-red-200">{error}</div>}
      {editing ? (
        <form onSubmit={submit} className="flex gap-2">
          <input
            autoFocus
            value={path}
            onChange={event => setPath(event.target.value)}
            onKeyDown={event => event.key === "Escape" && setEditing(false)}
            placeholder="~/projects/my-repo"
            aria-label="Workspace directory"
            className="w-96 rounded border border-neutral-600 bg-[#1e1e1e] px-3 py-1.5 font-mono text-neutral-100 outline-none focus:border-sky-500"
          />
          <button type="submit" disabled={busy || !path.trim()} className="rounded bg-sky-700 px-3 py-1.5 text-neutral-100 hover:bg-sky-600 disabled:opacity-50">
            Add
          </button>
          <button type="button" onClick={() => setEditing(false)} className="rounded px-3 py-1.5 text-neutral-400 hover:bg-neutral-800">
            Cancel
          </button>
        </form>
      ) : (
        <button
          type="button"
          onClick={() => {
            setError(null);
            setEditing(true);
          }}
          className="rounded bg-neutral-800 px-4 py-1.5 text-neutral-200 hover:bg-neutral-700"
        >
          Add workspace
        </button>
      )}
    </div>
  );
}

export function App() {
  const viewport = useViewportSize();
  const { workspaces } = useModel();
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(null);
  const [overview, setOverview] = useState(true);
  // Workspaces mount on first visit and stay mounted, so their views keep their layouts.
  const [openedWorkspaceIds, setOpenedWorkspaceIds] = useState(() => new Set<string>());
  // Leaves room below the cards for the "Add workspace" button.
  const cards = layoutCards(workspaces.length, viewport, 48);
  const canLeaveOverview = workspaces.some(workspace => workspace.id === activeWorkspaceId);

  useEffect(connect, []);

  const openWorkspace = (workspaceId: string) => {
    setActiveWorkspaceId(workspaceId);
    setOpenedWorkspaceIds(current => (current.has(workspaceId) ? current : new Set(current).add(workspaceId)));
    setOverview(false);
  };

  useEffect(() => {
    if (!overview || !canLeaveOverview) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOverview(false);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [overview, canLeaveOverview]);

  return (
    <div
      className="relative h-screen overflow-hidden bg-[#141414]"
      onClick={event => {
        if (overview && canLeaveOverview && event.target === event.currentTarget) setOverview(false);
      }}
    >
      {workspaces.map((workspace, index) => (
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
      {overview && <AddWorkspace onAdded={openWorkspace} />}
    </div>
  );
}

export default App;
