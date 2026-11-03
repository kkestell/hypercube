# Roadmap

## Background processes survive the idle timeout

Hypercube stops a session after ten minutes without activity unless someone
is viewing it or an agent in it has a turn running
(`internal/session/inactivity.go`). An agent that starts a background process,
such as Bash with `run_in_background`, and ends its turn to wait for it looks
idle. Hypercube then stops the session, the process dies with the agent, and
the completion notification never arrives.

One possible fix: give agents tools to start, list, and stop background
processes that Hypercube hosts. A running Hypercube-hosted process keeps its
session alive, so the idle timeout no longer interrupts it. Because every
agent can list every running process, agents also see what other agents are
running and avoid stepping on each other's work.
