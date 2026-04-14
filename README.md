# t4mem

`t4mem` is branchable, durable memory for agents that need to explain what they
did, compare alternatives, and recover work over time.

Most agent memory layers optimize for one of these:

- storing the latest state
- retrieving semantically similar text
- replaying chat or logs

`t4mem` is built for a different job: preserving structured work over time.
It treats memory as a combination of:

- timeline: what happened
- facts: what is believed
- state: what is currently true for active work
- branches: alternate lines of reasoning or execution

That makes it a better fit for coding agents, infrastructure automation, and
other workflows where reproducibility, auditability, and branchable execution
matter more than fuzzy recall.

## Product View

### What `t4mem` enables

- Branch an agent's memory before trying a risky approach
- Compare two lines of work without merging their histories together
- Trace a decision back to the observations and actions that led to it
- Promote facts from explicit evidence rather than vague summaries
- Snapshot active work at the task, session, or project level
- Recover long-running work after restarts with durable storage

### How an agent should use it

The best UX for `t4mem` is not making users think about tool names. The agent
should decide when to use the memory layer on the user's behalf.

A good default policy is:

- record events for meaningful milestones
- promote facts only when knowledge is stable
- use state for current working values
- use snapshots when the user asks for context
- use traces when the user asks "why"
- use branches when comparing alternatives

In practice that means:

- observations, decisions, plans, and adoptions should usually be events
- durable repo knowledge and stable workflow rules should become facts
- current plan, active branch, and active focus should live in state
- `task_snapshot`, `session_snapshot`, and `project_snapshot` should back broad summary requests
- `trace_decision` should back explanation requests

### Where it is strongest

- Coding and infrastructure agents
- Long-running automation
- Debugging and postmortems
- Migration and rollout workflows
- Multi-step planning with competing hypotheses

### What makes it different

#### Branchable reasoning

Most memory systems assume one mutable history. `t4mem` lets an agent fork
memory, explore alternatives, compare them, and adopt the winner.

Relevant tools:

- `memory.branch_create`
- `memory.branch_summary`
- `memory.branch_diff_summary`
- `memory.branch_compare`
- `memory.adopt_branch_with_reason`

#### Explainable decisions

`t4mem` can trace a decision back through prior events, causation links, and
supporting facts instead of forcing you to inspect a raw log manually.

Relevant tools:

- `memory.record_decision`
- `memory.trace_decision`
- `memory.recent_context`

#### Evidence-backed facts

Facts are first-class memory objects. They can carry provenance, branch
context, and explicit event evidence.

Relevant tools:

- `memory.promote_fact`
- `memory.promote_fact_from_events`
- `memory.list_facts`
- `memory.fact_summary`

#### Integrated working-memory snapshots

Instead of stitching together events, state, branch metadata, and facts across
many separate reads, `t4mem` can return focused snapshots for active work.

Relevant tools:

- `memory.task_snapshot`
- `memory.session_snapshot`
- `memory.project_snapshot`

## Bundled Skill

This repo also includes a skill at
[skills/t4mem-agent-memory/SKILL.md](/Users/amakhov/www/t4mem/skills/t4mem-agent-memory/SKILL.md).

Use it when you want an agent to apply `t4mem` as working memory automatically
instead of relying on the user to name individual MCP tools. The skill encodes:

- when to record observations, decisions, plans, and commands
- when to promote durable knowledge as facts
- when to use state instead of facts
- when to use snapshots for broad summaries
- when to use `trace_decision` for explanation requests
- when to use branches for alternative approaches

You can use the skill directly from the repo path or copy/install it into a
Codex skills directory for auto-discovery.

### Claude Equivalent

The bundled skill format is Codex-specific. For Claude, use the same policy as
project instructions in a `CLAUDE.md` file.

This repo includes a Claude-ready version at
[skills/t4mem-agent-memory/CLAUDE.md](/Users/amakhov/www/t4mem/skills/t4mem-agent-memory/CLAUDE.md).

You can copy it into your project root as `CLAUDE.md`, or paste the parts you
want into your existing Claude project instructions.

## Quick Start

Build and run the MCP server:

```bash
go build ./cmd/t4memd
./t4memd -root ./.t4mem
```

Or run directly:

```bash
go run ./cmd/t4memd -root ./.t4mem
```

By default, `t4memd` now acts as a stdio MCP adapter. It auto-starts a
background daemon that owns the local store lock and serves the same MCP
JSON-RPC over a Unix domain socket. This lets multiple Codex/Claude threads
share one `t4mem` root without tripping over Pebble's single-process lock.

To run only the shared daemon yourself:

```bash
./t4memd -daemon -root ./.t4mem
```

The daemon socket defaults to `ROOT/daemon.sock`. Override it with `-socket`
if you need to point multiple adapters at a different shared daemon.

The MCP surface supports:

- `initialize`
- `tools/list`
- `tools/call`

Verify the repo:

```bash
go test ./...
```

## Codex Plugin

This repo includes a local Codex plugin scaffold under
[plugins/t4mem](/Users/amakhov/www/t4mem/plugins/t4mem).

For distribution, prefer a release binary instead of `go run`. The plugin is
wired to launch a local `t4memd` executable from the plugin's `bin/` directory
and auto-installs that binary from GitHub Releases on first launch.

If you want to preinstall the current platform binary manually:

```bash
./plugins/t4mem/scripts/install_t4memd_release.sh
```

Then the plugin can launch `t4memd` through its local wrapper script:

```bash
/bin/sh ./plugins/t4mem/scripts/launch_t4memd.sh
```

The launcher resolves the repo root automatically and uses `./.t4mem` as the
memory store root for this checkout.

## Claude Plugin Marketplace

This repo also includes a Claude Code marketplace catalog at
[.claude-plugin/marketplace.json](/Users/amakhov/www/t4mem/.claude-plugin/marketplace.json)
and a Claude-compatible plugin manifest at
[plugins/t4mem/.claude-plugin/plugin.json](/Users/amakhov/www/t4mem/plugins/t4mem/.claude-plugin/plugin.json).

Once this repo is published on GitHub, the install flow is:

```bash
claude plugin marketplace add t4db/t4mem
claude plugin install t4mem@t4db-tools
```

For local testing from a checkout:

```bash
claude plugin marketplace add /absolute/path/to/t4mem
claude plugin install t4mem@t4db-tools
```

## MCP Setup

Example MCP server entry using the built binary:

```json
{
  "mcpServers": {
    "t4mem": {
      "command": "/absolute/path/to/t4memd",
      "args": ["-root", "/absolute/path/to/t4mem/.t4mem"]
    }
  }
}
```

The configured command stays the same even with the daemon architecture: the
MCP host talks to `t4memd` over stdio, and `t4memd` connects to or auto-starts
the shared local daemon behind the scenes.

Example using `go run` during development:

```json
{
  "mcpServers": {
    "t4mem": {
      "command": "go",
      "args": ["run", "./cmd/t4memd", "-root", "./.t4mem"],
      "cwd": "/absolute/path/to/t4mem"
    }
  }
}
```

### Claude Desktop Example

Add `t4mem` to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "t4mem": {
      "type": "stdio",
      "command": "/absolute/path/to/t4memd",
      "args": ["-root", "/absolute/path/to/t4mem/.t4mem"],
      "env": {}
    }
  }
}
```

You can also use `go run` during development by swapping the command and args
to:

```json
{
  "mcpServers": {
    "t4mem": {
      "type": "stdio",
      "command": "go",
      "args": ["run", "./cmd/t4memd", "-root", "./.t4mem"],
      "env": {}
    }
  }
}
```

If you want Claude to use `t4mem` proactively rather than waiting for explicit
tool names, pair the MCP config above with a project `CLAUDE.md` based on
[skills/t4mem-agent-memory/CLAUDE.md](/Users/amakhov/www/t4mem/skills/t4mem-agent-memory/CLAUDE.md).

### S3-Backed Durability

If `T4MEM_S3_BUCKET` is set, `t4memd` uses a `t4` object store for durable
WAL/checkpoint storage. If it is unset, the server runs in local embedded mode.

Supported environment variables:

- `T4MEM_S3_BUCKET`
- `T4MEM_S3_PREFIX`
- `T4MEM_S3_ENDPOINT`
- `T4MEM_S3_REGION`
- `T4MEM_AWS_PROFILE`
- `T4MEM_AWS_ACCESS_KEY_ID`
- `T4MEM_AWS_SECRET_ACCESS_KEY`

Example:

```json
{
  "mcpServers": {
    "t4mem": {
      "command": "/absolute/path/to/t4memd",
      "args": ["-root", "/absolute/path/to/t4mem/.t4mem"],
      "env": {
        "T4MEM_S3_BUCKET": "my-agent-memory",
        "T4MEM_S3_PREFIX": "t4mem/dev",
        "T4MEM_S3_REGION": "us-east-1"
      }
    }
  }
}
```

## MCP Surface

### Writes and structured events

- `memory.open_project`
- `memory.append_event`
- `memory.record_command`
- `memory.record_observation`
- `memory.record_decision`
- `memory.record_plan`
- `memory.update_plan`
- `memory.promote_fact`
- `memory.promote_fact_from_events`
- `memory.branch_create`
- `memory.branch_adopt`
- `memory.adopt_branch_with_reason`
- `memory.checkpoint`
- `memory.set_state`

### Reads and summaries

- `memory.get_state`
- `memory.timeline_query`
- `memory.recent_context`
- `memory.list_facts`
- `memory.fact_summary`
- `memory.trace_decision`
- `memory.branch_summary`
- `memory.branch_compare`
- `memory.branch_diff_summary`
- `memory.task_snapshot`
- `memory.session_snapshot`
- `memory.project_snapshot`

## Storage Model

t4mem uses [T4](https://github.com/t4db/t4) as a storage.

Data is laid out under prefixes like:

- `/projects/<project_id>/meta`
- `/projects/<project_id>/branches/<branch_id>/meta`
- `/projects/<project_id>/events/<branch_id>/<logical_ts>/<event_id>`
- `/projects/<project_id>/state/<scope>/<scope_id>/<branch_id>/<field>`
- `/projects/<project_id>/facts/<scope>/<scope_id>/<subject>/<predicate>/<fact_id>`
- `/projects/<project_id>/checkpoints/<branch_id>/<checkpoint_id>`
