---
name: t4mem-agent-memory
description: Use when working in a project that has t4mem available as an MCP memory backend and the agent should actively manage branchable, explainable working memory during the session. Trigger for requests to summarize the current task/session/project, explain why a decision was made, compare branches, preserve durable knowledge as facts, or keep a structured memory trail of observations, plans, decisions, and adoption events.
---

# t4mem Agent Memory

Use `t4mem` as the session memory layer without making the user think about tool names.

## Default Policy

- Record events for meaningful milestones.
- Promote facts only when knowledge is stable enough to matter again.
- Use state for current working values.
- Use snapshots when the user asks for broad context.
- Use traces when the user asks why something happened.
- Use branches when comparing alternatives.

## Choose The Right Memory Layer

Use events for things that happened.

Examples:
- observations
- decisions
- plans and plan updates
- branch adoption
- important commands when they matter to the work

Use facts for durable knowledge.

Examples:
- repo language
- module path
- stable architecture decisions
- persistent workflow rules
- product positioning that is unlikely to change soon

Do not promote facts for:
- temporary status
- speculative ideas
- noisy intermediate output
- one-off observations that are likely to change soon

Use state for current working values.

Examples:
- current plan
- active branch for a task
- current focus
- session-level working state

## Intent To Tool Mapping

When the user asks for broad context:
- "summarize the current task" -> `memory.task_snapshot`
- "what's going on in this session" -> `memory.session_snapshot`
- "what do we know about this project" -> `memory.project_snapshot` and optionally `memory.fact_summary`

When the user asks why:
- "why did we choose this"
- "what led to that"
- "why did the agent do this"
Use `memory.trace_decision`.

When exploring alternatives:
- create candidate branches with `memory.branch_create`
- record branch-specific observations and decisions
- compare with `memory.branch_summary`, `memory.branch_diff_summary`, or `memory.branch_compare`
- finalize with `memory.adopt_branch_with_reason`

When a stable conclusion follows from concrete evidence:
- record the evidence as events
- promote the conclusion with `memory.promote_fact_from_events`

## Recommended Write Behavior

During substantial work, prefer these writes:
- `memory.record_observation` for meaningful discoveries
- `memory.record_decision` for major choices
- `memory.record_plan` and `memory.update_plan` for active execution plans
- `memory.record_command` for important commands worth preserving

Be selective. Do not write an event for every tiny action.

## Recommended Read Behavior

Before answering a broad context question, prefer snapshots over manual reconstruction.

Before adopting a branch, prefer a diff summary over raw branch comparison output.

Before answering a "why" question, prefer `memory.trace_decision` over scanning the timeline manually.

When the user asks what is known, prefer `memory.fact_summary` or `memory.list_facts`.

## Example Workflows

### Explain a decision

1. Record observations.
2. Record the decision with useful context and, when possible, a `causation_id`.
3. Call `memory.trace_decision` when the user asks why.

### Compare alternatives

1. Keep `main` as the baseline.
2. Create one or more experiment branches.
3. Record branch-specific observations and decisions.
4. Compare branches.
5. Adopt the winning branch with a reason.

### Preserve durable repo knowledge

1. Confirm or repeat a stable piece of knowledge.
2. Promote it as a fact.
3. Reuse it later through fact summaries and snapshots.

## UX Principle

The user should not need to know MCP method names.

Translate natural-language requests into the right memory operations automatically. Use `t4mem` as a backend capability, not as user-facing jargon, unless the user explicitly asks about the memory system itself.
