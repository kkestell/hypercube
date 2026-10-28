// Hypercube plugin for OpenCode. The path of the hypercube executable replaces
// the value of `exe` when the plugin is written.
import { execFile } from "node:child_process"

const exe = HYPERCUBE_EXE

async function hook(event, sessionID) {
  const out = await new Promise((resolve, reject) => {
    const proc = execFile(exe, ["hook", event], (err, stdout) => {
      if (err) reject(err)
      else resolve(stdout)
    })
    proc.stdin.end(JSON.stringify({ session_id: sessionID }))
  })
  return out.trim() ? JSON.parse(out) : {}
}

const context = (response) => response.hookSpecificOutput?.additionalContext ?? ""

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

    // `hypercube wait` exits with status 2 after writing the messages of an
    // idle agent to stderr, and they start a turn in the session. Starting a
    // waiter replaces the one before it.
    let waiter
    function wait(sessionID) {
      const proc = execFile(exe, ["wait"], (err, _stdout, stderr) => {
        if (waiter === proc) waiter = undefined
        if (err?.code !== 2) return
        ctx.session.prompt({ sessionID, text: stderr.trim() }).catch(() => {})
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
        texts.push(context(await hook("SessionStart", event.sessionID)))
      }
      texts.push(context(await hook("UserPromptSubmit", event.sessionID)))
      const text = texts.filter(Boolean).join("\n\n")
      if (text) event.prompt.text += "\n\n" + text
    })

    await ctx.tool.hook("execute.after", async (event) => {
      if (event.status !== "completed" || !(await isAgentSession(event.sessionID))) return
      const text = context(await hook("PostToolUse", event.sessionID))
      if (!text) return
      const content = event.result.content
      const before = typeof content === "string" ? [{ type: "text", text: content }] : (content ?? [])
      event.result = { ...event.result, content: [...before, { type: "text", text }] }
    })

    // The event stream fails when its consumer falls behind, so the loop
    // asks only for the ends of turns, doesn't wait for the turn a blocked
    // stop starts, and subscribes again a second after the stream ends. A
    // loop that ended would leave the agent busy and never woken.
    const controller = new AbortController()
    async function stopped(sessionID) {
      const response = await hook("Stop", sessionID)
      if (response.reason) ctx.session.prompt({ sessionID, text: response.reason }).catch(() => {})
      else wait(sessionID)
    }
    void (async () => {
      while (!controller.signal.aborted) {
        try {
          for await (const event of ctx.event.subscribe({ types: [...ends], signal: controller.signal })) {
            if (!ends.has(event.type) || !started.has(event.data.sessionID)) continue
            await stopped(event.data.sessionID).catch(() => {})
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
