# Teamwork

This document collects published practices for running teams of coding agents:
how many agents to run, how to divide and delegate work, which models to use,
and how to check results. The sources, collected on 2026-10-27, are listed at
the end. The primary sources are engineering reports from Anthropic, Cognition,
and Aider, and one study of multi-agent failures. The rest are practitioner and
vendor write-ups.

## Patterns

Anthropic's _Building effective agents_ names the patterns most writing on the
subject uses:

- **Prompt chaining:** each step works on the previous step's output.
- **Routing:** a classifier sends each input to a specialized handler or model.
- **Parallelization:** independent sections run at once, or the same task runs
  several times and the results are voted on.
- **Orchestrator-workers:** a lead agent divides a task at runtime, delegates
  the parts, and combines the results.
- **Evaluator-optimizer:** one agent produces work and another evaluates it, in
  a loop.

Hypercube's orchestrator and workers follow the orchestrator-workers pattern.

## Team size

- Start with one agent, and add agents when a bottleneck calls for them. An
  over-built team is the most commonly reported mistake.
- Scale the number of agents to the task. Anthropic's research system uses one
  agent with 3 to 10 tool calls for simple fact-finding, 2 to 4 subagents for
  direct comparisons, and more than 10 for complex research.
- Multi-agent work costs more. In Anthropic's measurements, a single agent uses
  about 4 times the tokens of a chat, and a multi-agent system about 15 times.
  Token use alone explained 80% of the variance in their research evaluation.
- A person can actively supervise about one interactive workload at a time.
  Agents running alongside it need long, self-contained tasks.

## Dividing work

- Decomposition determines results more than agent count does. Agents given the
  same vague goal produce different interpretations that do not merge.
- Run independent work in parallel and coupled work in sequence. Every action
  carries implicit decisions, and parallel agents writing code make conflicting
  ones. Coding has fewer parallelizable tasks than research.
- Give each writing agent its own worktree to prevent file collisions. Design
  choices stay unshared across worktrees and surface at merge, so assign each
  shared hotspot file to a single writer.

## Delegation

- A delegation message states the objective, the expected output, the tools or
  sources to use, and the task's boundaries. Subagents given less duplicate
  work, leave gaps, or misread the task.
- The failure study sorts multi-agent failures into three groups: specification
  and system design, misalignment between agents, and task verification and
  termination. Most failures fall in the first two.
- Workers return conclusions. The orchestrator's context fills first, so
  workers send summaries and keep details in their own context or in files.

## Model selection

- Plan with a strong model and execute with a cheaper one. Aider's architect
  and editor modes, a reasoning model that plans the change and a second model
  that writes the edits, set a new top score on Aider's code-editing benchmark.
- Use a strong model for the orchestrator. Anthropic's research system, an Opus
  lead with Sonnet subagents, outperformed a single Opus agent by 90.2% on
  their research evaluation.
- Match the worker's model to the task's size. A cascade tries a cheap model
  first and escalates to a stronger one when a check fails.
- Independent evaluations of automatic model routers find smaller gains than
  vendors report, and several routers fail to beat the best single model.
  Measure any routing scheme on your own tasks.

## Verification

- A separate agent with a fresh context reviews each change before it merges.
  Anthropic's Claude Code guidance has one agent write code and another review
  it.
- The build and the tests are the final check. An evaluator-optimizer loop
  needs clear evaluation criteria.
- Bound each worker's iterations, time, and tokens.

## Sources

- [Anthropic: Building effective agents](https://www.anthropic.com/engineering/building-effective-agents)
- [Anthropic: How we built our multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system)
- [Anthropic: Claude Code best practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Cognition: Don't Build Multi-Agents](https://cognition.ai/blog/dont-build-multi-agents)
- [Aider: Separating code reasoning and editing](https://aider.chat/2024/09/26/architect.html)
- [Cemri et al.: Why Do Multi-Agent LLM Systems Fail?](https://arxiv.org/abs/2503.13657)
- [Chen et al.: FrugalGPT](https://arxiv.org/abs/2305.05176)
- [Addy Osmani: The Code Agent Orchestra](https://addyosmani.com/blog/code-agent-orchestra/)
- [Multi-Agent Orchestration: 4 Patterns That Actually Work](https://www.heyuan110.com/posts/ai/2026-02-26-multi-agent-orchestration/)
- [5 lessons from running AI coding agents in parallel](https://dev.to/battyterm/5-lessons-from-running-ai-coding-agents-in-parallel-53on)
- [From 1 to 3 parallel Claude Code agents](https://dev.to/0xandrewshu/from-1-to-3-parallel-claude-code-agents-how-i-broke-past-16-utilization-mee)
- [Parallel agentic development with Git worktrees](https://www.mindstudio.ai/blog/parallel-agentic-development-git-worktrees/)
- [Model routing and advisor patterns](https://codex.danielvaughan.com/2026/04/12/model-routing-advisor-patterns-cut-ai-coding-costs/)
- [Arize: How cheap models changed multi-agent economics](https://arize.com/blog/how-cheap-models-changed-multi-agent-economics/)
- [Multi-agent model routing: planner and executor pairs](https://www.morphllm.com/multi-agent-model-routing)
