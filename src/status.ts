import type { Agent } from "./model";

/** Which of the agents are working, and which are waiting on the user to answer a permission request. */
export function activity(agents: (Agent | undefined)[]) {
  const present = agents.filter((agent): agent is Agent => agent !== undefined);
  return {
    busy: present.filter(agent => agent.status === "busy"),
    waiting: present.filter(agent => agent.status === "waiting"),
  };
}
