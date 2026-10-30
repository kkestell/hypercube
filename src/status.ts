import type { Agent } from "./model";

/** Which of the agents are working, and which are waiting on the user to answer a permission request. */
export function activity(agents: Agent[]) {
  return {
    busy: agents.filter(agent => agent.status === "busy"),
    waiting: agents.filter(agent => agent.status === "waiting"),
  };
}
