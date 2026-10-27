// Hypercube extension for Pi. The path of the hypercube executable replaces
// the value of `exe` when the extension is written.
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

export default function (pi) {
  pi.registerMcpServer("hypercube", {
    command: exe,
    args: ["mcp"],
    env: {
      HYPERCUBE_AGENT_ID: process.env.HYPERCUBE_AGENT_ID,
      HYPERCUBE_STATE: process.env.HYPERCUBE_STATE,
    },
    exposure: "direct",
  })

  const started = new Set()
  pi.on("before_agent_start", async (_event, ctx) => {
    const sessionID = ctx.sessionManager.getSessionId()
    const texts = []
    if (!started.has(sessionID)) {
      texts.push(context(await hook("SessionStart", sessionID)))
      started.add(sessionID)
    }
    texts.push(context(await hook("UserPromptSubmit", sessionID)))
    const content = texts.filter(Boolean).join("\n\n")
    if (content) return { message: { customType: "hypercube", content, display: true } }
  })

  pi.on("tool_result", async (event, ctx) => {
    // Nested calls never appear in the transcript, so they cannot deliver messages.
    if (event.parentToolCallId) return
    const text = context(await hook("PostToolUse", ctx.sessionManager.getSessionId()))
    if (text) return { content: [...event.content, { type: "text", text }] }
  })

  pi.on("agent_settled", async (_event, ctx) => {
    const response = await hook("Stop", ctx.sessionManager.getSessionId())
    if (response.reason) pi.sendUserMessage(response.reason)
  })
}
