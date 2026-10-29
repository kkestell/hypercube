import { useEffect, useRef } from "react";
import type { IDockviewPanelProps } from "dockview-react";
import { getOrCreateTerminal } from "./terminals";

export function TerminalPanel({ api }: IDockviewPanelProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current!;
    const terminal = getOrCreateTerminal(api.id, () => api.close());
    container.appendChild(terminal.element);

    const resizeObserver = new ResizeObserver(() => terminal.fit());
    resizeObserver.observe(container);

    const activeListener = api.onDidActiveChange(({ isActive }) => {
      if (isActive) terminal.focus();
    });
    if (api.isActive) terminal.focus();

    return () => {
      resizeObserver.disconnect();
      activeListener.dispose();
      terminal.element.remove();
    };
  }, [api]);

  return <div ref={containerRef} className="h-full w-full overflow-hidden bg-[#1e1e1e] pl-2 pt-1" />;
}
