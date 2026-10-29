import { useEffect, useRef } from "react";
import type { Session } from "./model";
import { request } from "./socket";
import { focusView, getOrCreateView } from "./terminals";

const button = "rounded bg-neutral-700 px-3 py-1 text-neutral-100 hover:bg-neutral-600";

function run(message: Parameters<typeof request>[0]) {
  return request(message).catch((error: Error) => alert(error.message));
}

/** The active session's shell, or a way to start one. One xterm per workspace, switched between sessions' terminals. */
export function TerminalPanel({ viewId, session }: { viewId: string; session: Session | null }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const view = getOrCreateView(viewId);
  const terminalId = session?.terminal ? session.id : null;

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

  const start = () => {
    if (session) void run({ t: "startTerminal", sessionId: session.id }).then(() => focusView(viewId));
  };
  const close = () => {
    if (session && confirm("Close the terminal? Everything running in it stops.")) void run({ t: "closeTerminal", sessionId: session.id });
  };

  return (
    <div className="flex h-full flex-col bg-[#252526]">
      <div className="flex items-center px-4 py-2 text-[11px] font-semibold tracking-wide text-neutral-400 uppercase">
        <span className="flex-1">Terminal</span>
        {terminalId && (
          <button type="button" title="Close terminal" onClick={close} className="text-sm leading-none hover:text-neutral-100">
            ×
          </button>
        )}
      </div>
      <div className="relative min-h-0 flex-1">
        <div ref={containerRef} className={`h-full w-full overflow-hidden bg-[#1e1e1e] pl-2 pt-1 ${terminalId ? "" : "invisible"}`} />
        {!terminalId && (
          <div className="absolute inset-0 flex items-center justify-center bg-[#1e1e1e] text-neutral-400">
            {session?.status === "ready" ? (
              <button type="button" className={button} onClick={start}>
                Start terminal
              </button>
            ) : (
              <span>{session ? "The session isn't running" : "No session"}</span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
