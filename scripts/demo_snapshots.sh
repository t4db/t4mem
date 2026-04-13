#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${1:-/tmp/t4mem-demo}"
PROJECT_ID="${2:-demo-repo}"

cat <<EOF | go run ./cmd/t4memd -root "${ROOT_DIR}"
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"demo","version":"1.0.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"memory.open_project","arguments":{"project_id":"${PROJECT_ID}"}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"memory.set_state","arguments":{"key":{"project_id":"${PROJECT_ID}","scope":"task","scope_id":"task-1","field":"branch_id"},"value":"main"}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"memory.append_event","arguments":{"event_id":"evt-main-observation","logical_ts":"0001","project_id":"${PROJECT_ID}","branch_id":"main","task_id":"task-1","type":"observation.recorded","payload":{"summary":"Baseline looks healthy","kind":"health-check"}}}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"memory.append_event","arguments":{"event_id":"evt-main-decision","logical_ts":"0002","project_id":"${PROJECT_ID}","branch_id":"main","task_id":"task-1","type":"decision.made","causation_id":"evt-main-observation","payload":{"summary":"Try an alternate rollout branch","rationale":"Compare approaches before adopting"}}}}
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"memory.branch_create","arguments":{"project_id":"${PROJECT_ID}","from":"main","name":"experiment"}}}
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"memory.append_event","arguments":{"event_id":"evt-exp-observation","logical_ts":"0003","project_id":"${PROJECT_ID}","branch_id":"experiment","task_id":"task-1","type":"observation.recorded","payload":{"summary":"Experiment branch passes extra checks","kind":"verification"}}}}
{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"memory.promote_fact_from_events","arguments":{"project_id":"${PROJECT_ID}","scope":"project","scope_id":"${PROJECT_ID}","subject":"repo","predicate":"winning_branch_signal","value":"experiment branch passed verification","confidence":0.92,"evidence_event_ids":["evt-exp-observation"],"source_branch":"experiment"}}}
{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"memory.branch_diff_summary","arguments":{"project_id":"${PROJECT_ID}","left_branch_id":"main","right_branch_id":"experiment"}}}
{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"memory.adopt_branch_with_reason","arguments":{"project_id":"${PROJECT_ID}","branch_id":"experiment","reason":"Experiment branch showed stronger evidence","summary":"Adopt experiment as canonical"}}}
{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"memory.trace_decision","arguments":{"project_id":"${PROJECT_ID}","decision_event_id":"evt-main-decision"}}}
{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"memory.task_snapshot","arguments":{"project_id":"${PROJECT_ID}","task_id":"task-1","state_fields":["branch_id"],"limit":5}}}
{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"memory.project_snapshot","arguments":{"project_id":"${PROJECT_ID}","limit":5}}}
EOF
