# Gantt chart of the orchestrator's plan

The orchestrator writes its plan as a small text file: each task, the agent
doing it, an estimated duration, and the tasks it waits on. Hypercube renders
the plan as a Gantt chart in the terminal and keeps the chart current as the
work progresses.

## Plan format

One task per line: name, agent, duration, and an optional `after` list of
dependencies.

```
store      ox    2h
config     odd   1h
mux        ergo  4h
workspace  ox    1h    after store
host       odd   2.5h  after config, mux
mcp        ox    2h    after store, workspace
itest      ergo  1.5h  after host, mcp
review     rev   1h    after itest
```

## Rendered chart

Each task starts when all of its dependencies are done and its agent has
finished its previous task.

```
#  task       agent  0   1   2   3   4   5   6   7   8   9 after
1  store      ox    │████████                            │
2  config     odd   │████                                │
3  mux        ergo  │████████████████                    │
4  workspace  ox    │        ████                        │ 1
5  host       odd   │    ░░░░░░░░░░░░██████████          │ 2, 3
6  mcp        ox    │            ████████                │ 1, 4
7  itest      ergo  │                ░░░░░░░░░░██████    │ 5, 6
8  review     rev   │                                ████│ 7

█ working   ░ idle, waiting on a dependency
```

- Rows are tasks in plan order. The `agent` column names the worker, and each
  agent's bars share a color.
- The `after` column lists a task's dependencies. A `░` stretch is an agent
  waiting on one.
- The scale here is 4 columns per hour. Bars end in the eighth-width blocks
  (`▏▎▍▌▋▊▉`) for fractions of a column.
- The critical path, here store → workspace → mcp → itest → review, is drawn in
  bold or a brighter color.

## Live updates

As work progresses, the chart shows each task's state: not started, in
progress, or done. The time axis shows the current time. Bars for finished
tasks show actual start and end times, and the rest of the schedule moves to
follow them.

## Open questions

- Where the plan lives: a file in the workspace, the state file, or an MCP
  tool the orchestrator calls.
- How task state changes: the orchestrator edits the plan, workers report
  through a tool, or Hypercube infers it from agent activity and messages.
- Where the chart appears: its own pane, an overlay, or a `hypercube` command.
- Whether durations are hours, agent turns, or relative sizes, since agent work
  times are hard to estimate.
- How this fits the principle that agents decide how to divide and track work,
  with no task registry or workflow in Hypercube. One answer is that the plan
  is optional and only drawn, and Hypercube never acts on it.
