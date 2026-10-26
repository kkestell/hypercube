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

export default {
  id: "hypercube",
  async setup(ctx) {
    const requestedModel = JSON.parse(process.env.OPENCODE_CONFIG_CONTENT).model
    const started = new Set()
    const parents = new Map()

    // Subagent sessions have a parent. Only the top-level session is the agent.
    async function isAgentSession(sessionID) {
      if (!parents.has(sessionID)) {
        parents.set(sessionID, (await ctx.session.get({ sessionID })).parentID)
      }
      return !parents.get(sessionID)
    }

    await ctx.session.hook("prompt", async (event) => {
      if (!(await isAgentSession(event.sessionID))) return
      let text = ""
      if (!started.has(event.sessionID)) {
        // The TUI silently falls back to a recent model for unavailable IDs.
        const { model } = await ctx.session.get({ sessionID: event.sessionID })
        const selectedModel = model ? `${model.providerID}/${model.id}` : "no model"
        if (selectedModel !== requestedModel) {
          throw new Error(`OpenCode selected ${selectedModel} instead of requested ${requestedModel}. Use a full provider/model ID from opencode models (for example, openrouter/owner/model).`)
        }
        started.add(event.sessionID)
        text += context(await hook("SessionStart", event.sessionID))
      }
      text += context(await hook("UserPromptSubmit", event.sessionID))
      if (text) event.prompt.text += "\n\n" + text
    })

    await ctx.tool.hook("execute.after", async (event) => {
      if (event.status !== "completed" || !(await isAgentSession(event.sessionID))) return
      const text = context(await hook("PostToolUse", event.sessionID))
      if (text) event.result = { ...event.result, content: [...event.result.content, { type: "text", text }] }
    })

    // A turn ends when its execution succeeds, fails, or is interrupted.
    const idle = new Set(["session.execution.succeeded", "session.execution.failed", "session.execution.interrupted"])
    const controller = new AbortController()
    void (async () => {
      for await (const event of ctx.event.subscribe({ signal: controller.signal })) {
        if (!idle.has(event.type)) continue
        const { sessionID } = event.data
        if (!started.has(sessionID)) continue
        if (!(await isAgentSession(sessionID))) continue
        const response = await hook("Stop", sessionID)
        if (response.reason) await ctx.session.prompt({ sessionID, text: response.reason })
      }
    })()
    return () => controller.abort()
  },
}
