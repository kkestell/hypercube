import type { Direction } from "dockview-react";

/** An agent's tab, placed relative to an agent listed before it in the same session. */
export type AgentStub = {
  id: string;
  name: string;
  position?: { relativeTo: string; direction: Direction };
};

export type Session = {
  id: string;
  name: string;
  agents: AgentStub[];
};
