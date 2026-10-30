
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { pathToFileURL } from "node:url"
const { default: plugin } = await import(pathToFileURL(process.env.HYPERCUBE_TEST_PLUGIN))

// A stand-in for OpenCode's plugin context. s1 is the launched session, s2
// is on another model, and child is a subagent's session.
const model = { providerID: "p", id: "m" }
const sessions = { s1: { model }, s2: { model: { providerID: "p", id: "other" } }, child: { model, parentID: "s1" } }
const hooks = new Map()
const prompts = []
const events = []
let push
const ctx = {
  session: {
    get: async ({ sessionID }) => {
      if (!sessions[sessionID]) throw new Error(`no session ${sessionID}`)
      return sessions[sessionID]
    },
    hook: async (name, callback) => { hooks.set(`session.${name}`, callback) },
    prompt: async (input) => { prompts.push(input) },
  },
  tool: { hook: async (name, callback) => { hooks.set(`tool.${name}`, callback) } },
  event: {
    subscribe: async function* ({ signal }) {
      while (!signal.aborted) {
        if (events.length) yield events.shift()
        else await new Promise((resolve) => { push = resolve; signal.addEventListener("abort", resolve) })
      }
    },
  },
}
const emit = (type, sessionID, data = {}) => { events.push({ type, data: { sessionID, ...data } }); push?.() }
const until = async (what, f) => {
  for (let i = 0; i < 500; i++) {
    if (f()) return
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
  assert.fail(`timed out waiting for ${what}`)
}
const lines = (path) => {
  try { return readFileSync(path, "utf8").trim().split("\n") } catch { return [] }
}
const calls = () => lines(process.env.HYPERCUBE_TEST_LOG).map((line) => {
  const space = line.indexOf(" ")
  return [line.slice(0, space), JSON.parse(line.slice(space + 1)).session_id]
})
const waits = () => lines(`${process.env.HYPERCUBE_TEST_LOG}.waits`).map(Number)
const alive = (pid) => {
  try { process.kill(pid, 0); return true } catch { return false }
}

// The launched session exists, so its waiter starts, and its messages start
// a turn there.
const cleanup = await plugin.setup(ctx)
await until("the waiter's messages", () => prompts.length === 1)
assert.deepEqual(prompts, [{ sessionID: "s1", text: "Message from o (o1):\nhi" }])

// The first prompt of a session runs SessionStart, and every prompt runs
// UserPromptSubmit. Their context is added to the prompt.
const prompt = async (sessionID, text) => {
  const event = { sessionID, prompt: { text } }
  await hooks.get("session.prompt")(event)
  return event.prompt.text
}
assert.equal(await prompt("s1", "hello"), "hello\n\nSessionStart context\n\nUserPromptSubmit context")
assert.equal(await prompt("s1", "again"), "again\n\nUserPromptSubmit context")
assert.deepEqual(calls(), [["SessionStart", "s1"], ["UserPromptSubmit", "s1"], ["UserPromptSubmit", "s1"]])

// A session on another model is refused before any hook runs.
await assert.rejects(prompt("s2", "x"), /OpenCode selected p\/other instead of requested p\/m/)

// A subagent's prompts and tools never consume queued messages.
assert.equal(await prompt("child", "sub"), "sub")
const tool = async (event) => {
  await hooks.get("tool.execute.after")(event)
  return event.result
}
const result = { output: 1, content: "original" }
assert.equal(await tool({ status: "completed", sessionID: "child", result }), result)
assert.equal(await tool({ status: "error", sessionID: "s1", error: {} }), undefined)
assert.equal(calls().length, 3)

// A completed tool's result gets PostToolUse context.
assert.deepEqual(await tool({ status: "completed", sessionID: "s1", result }), {
  output: 1,
  content: [{ type: "text", text: "original" }, { type: "text", text: "PostToolUse context" }],
})
assert.equal(result.content, "original")

// The end of a turn runs Stop. A block reason starts a turn with it;
// otherwise a waiter starts. Sessions without a prompt here are ignored.
emit("session.execution.succeeded", "child")
emit("session.execution.succeeded", "s1")
await until("the blocked stop's turn", () => prompts.length === 2)
assert.deepEqual(prompts[1], { sessionID: "s1", text: "queued" })
assert.equal(waits().length, 1, "a blocked stop continues the turn, so no waiter starts")
emit("session.execution.interrupted", "s1")
await until("a waiter after the stop", () => waits().length === 2)
assert.equal(prompts.length, 2)
assert.deepEqual(calls().slice(3), [["PostToolUse", "s1"], ["Stop", "s1"], ["Stop", "s1"]])

// A failed execution runs StopFailure with its structured error, not Stop.
// End the stand-in waiter before replacing it; it has no generation check.
process.kill(waits()[1])
await until("the previous waiter to end", () => !alive(waits()[1]))
const error = { type: "provider.api", message: "This request would exceed your available credits.", status: 402 }
emit("session.execution.failed", "child", { error })
emit("session.execution.failed", "s1", { error })
await until("a waiter after the failure", () => waits().length === 3)
assert.equal(prompts.length, 2, "a failure never continues the turn with a Stop block reason")
assert.deepEqual(calls().slice(6), [["StopFailure", "s1"]])
const failure = JSON.parse(lines(process.env.HYPERCUBE_TEST_LOG).at(-1).slice("StopFailure ".length))
assert.deepEqual(failure, { session_id: "s1", error: error.type, error_details: error.message })

// Cleanup ends the waiter.
const waiter = waits().at(-1)
assert(alive(waiter))
cleanup()
await until("the waiter to end", () => !alive(waiter))
