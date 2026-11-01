// Hypercube plugin for OpenCode. The path of the hypercube executable replaces
// the value of `exe` when the plugin is written.
import { execFile } from "node:child_process"

const exe = HYPERCUBE_EXE

// `hypercube report` answers with text for the model to read now.
async function report(event, body = {}) {
  const out = await new Promise((resolve, reject) => {
    const proc = execFile(exe, ["report", event], (err, stdout) => {
      if (err) reject(err)
      else resolve(stdout)
    })
    proc.stdin.end(JSON.stringify(body))
  })
  return JSON.parse(out).context ?? ""
}

// A turn ends when its execution succeeds, fails, or is interrupted.
const ends = new Set(["session.execution.succeeded", "session.execution.failed", "session.execution.interrupted"])

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

    // `hypercube wait` exits with status 2 after printing the messages of an
    // idle agent, and they start a turn in the session. Starting a waiter
    // replaces the one before it.
    let waiter
    function wait(sessionID) {
      const proc = execFile(exe, ["wait"], (err, stdout) => {
        if (waiter === proc) waiter = undefined
        if (err?.code !== 2) return
        ctx.session.prompt({ sessionID, text: stdout.trim() }).catch(() => {})
      })
      proc.stdin.end()
      waiter = proc
    }

    // A resumed session can be woken at once. A new one exists from its first
    // prompt, and the end of that turn starts a waiter.
    const launched = process.env.HYPERCUBE_SESSION_ID
    if (await ctx.session.get({ sessionID: launched }).then(() => true, () => false)) wait(launched)

    await ctx.session.hook("prompt", async (event) => {
      if (!(await isAgentSession(event.sessionID))) return
      const texts = []
      if (!started.has(event.sessionID)) {
        // OpenCode silently falls back to a recent model for unavailable IDs.
        const { model } = await ctx.session.get({ sessionID: event.sessionID })
        const selectedModel = model ? `${model.providerID}/${model.id}` : "no model"
        if (selectedModel !== requestedModel) {
          throw new Error(`OpenCode selected ${selectedModel} instead of requested ${requestedModel}. Use a full provider/model ID from opencode models (for example, openrouter/owner/model).`)
        }
        started.add(event.sessionID)
        texts.push(await report("start"))
      }
      texts.push(await report("prompt", { conversation: event.sessionID, text: event.prompt.text }))
      const text = texts.filter(Boolean).join("\n\n")
      if (text) event.prompt.text += "\n\n" + text
    })

    await ctx.tool.hook("execute.after", async (event) => {
      // Hypercube logs failed calls of its own tools, subagents' included.
      // Logging never changes the call's result.
      if (event.status === "error") {
        if (event.tool.startsWith("hypercube_")) {
          await report("toolfailed", { tool: event.tool, input: event.input, error: event.error?.message }).catch(() => {})
        }
        return
      }
      if (event.status !== "completed" || !(await isAgentSession(event.sessionID))) return
      const text = await report("tool")
      if (!text) return
      const content = event.result.content
      const before = typeof content === "string" ? [{ type: "text", text: content }] : (content ?? [])
      event.result = { ...event.result, content: [...before, { type: "text", text }] }
    })

    // Usage follows each step of a turn: the step's tokens, the session's
    // cost so far, and the context limit of the session's model. Usage only
    // feeds the user's view.
    async function usage({ sessionID, tokens }) {
      const { model, cost } = await ctx.session.get({ sessionID })
      const info = (await ctx.model.list()).data.find((m) => m.providerID === model?.providerID && m.id === model?.id)
      const context = tokens.input + tokens.cache.read + tokens.cache.write
      await report("usage", { context, window: info?.limit?.context ?? 0, cost })
    }

    // The event stream fails when its consumer falls behind, so the loop
    // asks only for the ends of steps and turns, doesn't wait for usage or
    // for the turn a blocked stop starts, and subscribes again a second after
    // the stream ends. A loop that ended would leave the agent busy and never
    // woken.
    const controller = new AbortController()
    async function stopped(event) {
      const { sessionID, error } = event.data
      if (event.type === "session.execution.failed") {
        await report("failed", { error: error?.message || error?.type })
        wait(sessionID)
        return
      }
      const text = await report("stop")
      if (text) ctx.session.prompt({ sessionID, text }).catch(() => {})
      else wait(sessionID)
    }
    void (async () => {
      while (!controller.signal.aborted) {
        try {
          for await (const event of ctx.event.subscribe({ types: [...ends, "session.step.ended"], signal: controller.signal })) {
            if (!started.has(event.data.sessionID)) continue
            if (event.type === "session.step.ended") void usage(event.data).catch(() => {})
            if (!ends.has(event.type)) continue
            await stopped(event).catch(() => {})
          }
        } catch {}
        await new Promise((resolve) => setTimeout(resolve, 1000))
      }
    })()
    return () => {
      controller.abort()
      waiter?.kill()
    }
  },
}
