// Hypercube extension for Pi. The path of the hypercube executable replaces
// the value of `exe` when the extension is written.
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

const text = (content) => content.filter((part) => part.type === "text").map((part) => part.text).join("\n")

export default function (pi) {
  const requestedModel = process.env.HYPERCUBE_MODEL
  let started = false
  let verified = false
  let lastAssistant

  pi.registerMcpServer("hypercube", JSON.parse(process.env.HYPERCUBE_MCP))

  // Starts a turn, or joins the running one, without typing into the editor.
  async function send(text) {
    try {
      await pi.sendUserMessage(text, { deliverAs: "followUp" })
    } catch {}
  }

  // `hypercube wait` exits with status 2 after printing the messages of an
  // idle agent, and they start a turn. Starting a waiter replaces the one
  // before it.
  let waiter
  function wait() {
    const proc = execFile(exe, ["wait"], (err, stdout) => {
      if (waiter === proc) waiter = undefined
      if (err?.code !== 2) return
      void send(stdout.trim())
    })
    proc.stdin.end()
    waiter = proc
  }

  // A resumed conversation can be woken at once. A new one exists from its
  // first prompt, and the end of that turn starts a waiter.
  pi.on("session_start", async (_event, ctx) => {
    // A new or switched session needs its start context again.
    started = false
    if (ctx.sessionManager.getEntries().some((entry) => entry.type === "message")) wait()
  })

  pi.on("session_shutdown", async () => {
    waiter?.kill()
  })

  // Pi resolves a partial model ID to another model. The first prompt is
  // rejected when the selected model is not the requested one.
  pi.on("input", async (_event, ctx) => {
    if (verified) return
    const selected = ctx.model ? `${ctx.model.provider}/${ctx.model.id}` : "no model"
    if (selected !== requestedModel) {
      const error = `Pi selected ${selected} instead of requested ${requestedModel}. Use a full provider/model ID from pi --list-models.`
      ctx.ui.notify(error, "error")
      await report("failed", { error }).catch(() => {})
      return { action: "handled" }
    }
    verified = true
  })

  pi.on("before_agent_start", async (event, ctx) => {
    if (event.messageType !== "user") return
    const texts = []
    if (!started) {
      started = true
      texts.push(await report("start"))
    }
    texts.push(await report("prompt", { session: ctx.sessionManager.getSessionId(), text: event.prompt }))
    const context = texts.filter(Boolean).join("\n\n")
    if (context) return { message: { customType: "hypercube", content: context, display: false } }
  })

  pi.on("tool_result", async (event) => {
    // Hypercube logs failed calls of its own tools. Logging never changes the
    // call's result.
    if (event.isError) {
      if (event.toolName.startsWith("mcp__hypercube__")) {
        await report("toolfailed", { tool: event.toolName, input: event.input, error: text(event.content) }).catch(() => {})
      }
      return
    }
    if (!started || event.parentToolCallId) return
    const context = await report("tool")
    if (context) return { content: [...event.content, { type: "text", text: context }] }
  })

  // Usage follows each model response. Usage only feeds the user's view.
  pi.on("turn_end", async (_event, ctx) => {
    if (!started) return
    const context = ctx.getContextUsage()
    let cost = 0
    for (const entry of ctx.sessionManager.getEntries()) {
      if (entry.type === "message") cost += entry.message.usage?.cost?.total ?? 0
    }
    await report("usage", { context: context?.tokens ?? 0, window: context?.contextWindow ?? 0, cost }).catch(() => {})
  })

  pi.on("message_end", async (event) => {
    if (event.message.role === "assistant") lastAssistant = event.message
  })

  // Settling is the true end of a run: Pi has no retry, compaction, or queued
  // message left to start.
  pi.on("agent_settled", async () => {
    if (!started) return
    const last = lastAssistant
    lastAssistant = undefined
    if (last?.stopReason === "error") {
      await report("failed", { error: last.errorMessage }).catch(() => {})
      wait()
      return
    }
    const context = await report("stop").catch(() => "")
    if (context) void send(context)
    else wait()
  })
}
