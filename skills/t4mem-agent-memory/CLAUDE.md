# t4mem Agent Memory for Claude

Use `t4mem` as the working memory backend for this project. The user should not
need to think about MCP tool names.

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

## Intent Mapping

When the user asks for broad context:
- "summarize the current task" -> use the task snapshot flow
- "what's going on in this session" -> use the session snapshot flow
- "what do we know about this project" -> use the project snapshot flow and fact summaries when helpful

When the user asks why:
- "why did we choose this"
- "what led to that"
- "why did the agent do this"
Use decision tracing rather than scanning raw history manually.

When exploring alternatives:
- create candidate branches
- record branch-specific observations and decisions
- compare branches before adopting one
- record adoption with an explicit reason

When a stable conclusion follows from concrete evidence:
- record the evidence as events
- promote the conclusion into fact memory with provenance

## Recommended Write Behavior

During substantial work, prefer recording:
- observations for meaningful discoveries
- decisions for major choices
- plans and plan updates for active execution
- important commands worth preserving

Be selective. Do not write an event for every tiny action.

## Recommended Read Behavior

Before answering a broad context question, prefer snapshots over manual reconstruction.

Before adopting a branch, prefer a branch diff summary over raw branch history.

Before answering a "why" question, prefer decision tracing over scanning the timeline.

When the user asks what is known, prefer fact summaries or fact listings.

## UX Principle

Translate natural-language requests into the right memory operations automatically. Use `t4mem` as a backend capability, not as user-facing jargon, unless the user explicitly asks about the memory system itself.
