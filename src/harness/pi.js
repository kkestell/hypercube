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

// `hypercube wait` exits with status 2 after writing the messages of an idle
// agent to stderr. Starting a waiter replaces the one before it.
let waiter
function wait(pi) {
  const proc = execFile(exe, ["wait"], (err, _stdout, stderr) => {
    if (waiter === proc) waiter = undefined
    if (err?.code !== 2) return
    const content = stderr.trim()
    pi.sendMessage({ customType: "hypercube", content, display: true }, { triggerTurn: true, deliverAs: "followUp" })
  })
  proc.stdin.end()
  waiter = proc
}

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

  pi.on("session_start", () => wait(pi))
  pi.on("session_shutdown", () => waiter?.kill())

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
    else wait(pi)
  })
}
