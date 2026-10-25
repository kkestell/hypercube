// Hypercube plugin for OpenCode. It maps OpenCode's session and tool events
// onto `hypercube hook` calls. HYPERCUBE_EXE is replaced with the path of the
// hypercube executable when the plugin is written.
const exe = HYPERCUBE_EXE

async function hook(event, sessionID) {
  const proc = Bun.spawn([exe, "hook", event], {
    stdin: new Blob([JSON.stringify({ session_id: sessionID })]),
    stdout: "pipe",
  })
  const out = await new Response(proc.stdout).text()
  await proc.exited
  return out.trim() ? JSON.parse(out) : {}
}

const context = (response) => response.hookSpecificOutput?.additionalContext ?? ""

export const Hypercube = async ({ client }) => {
  const started = new Set()
  const parents = new Map()

  // Subagent sessions have a parent. Only the top-level session is the agent.
  async function isAgentSession(id) {
    if (!parents.has(id)) {
      const { data } = await client.session.get({ path: { id } })
      parents.set(id, data?.parentID)
    }
    return !parents.get(id)
  }

  return {
    "chat.message": async (input, output) => {
      if (!(await isAgentSession(input.sessionID))) return
      let text = ""
      if (!started.has(input.sessionID)) {
        started.add(input.sessionID)
        text += context(await hook("SessionStart", input.sessionID))
      }
      text += context(await hook("UserPromptSubmit", input.sessionID))
      const part = output.parts.find((p) => p.type === "text")
      if (text && part) part.text += "\n\n" + text
    },
    "tool.execute.after": async (input, output) => {
      if (!(await isAgentSession(input.sessionID))) return
      const text = context(await hook("PostToolUse", input.sessionID))
      if (text) output.output += "\n\n" + text
    },
    event: async ({ event }) => {
      if (event.type !== "session.idle") return
      const id = event.properties.sessionID
      if (!(await isAgentSession(id))) return
      const response = await hook("Stop", id)
      if (response.reason) {
        await client.session.promptAsync({ path: { id }, body: { parts: [{ type: "text", text: response.reason }] } })
      }
    },
  }
}
