package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/t4db/t4mem/memory"
)

func TestInitializeNegotiatesCurrentProtocol(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	result, err := server.handle(context.Background(), "initialize", json.RawMessage(`{
		"protocolVersion": "2025-06-18",
		"capabilities": {
			"roots": {
				"listChanged": true
			}
		},
		"clientInfo": {
			"name": "test-client",
			"version": "1.0.0"
		}
	}`))
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("initialize result type = %T, want map[string]any", result)
	}
	if got := payload["protocolVersion"]; got != protocolVersionLatest {
		t.Fatalf("protocolVersion = %v, want %q", got, protocolVersionLatest)
	}

	capabilities, ok := payload["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities type = %T", payload["capabilities"])
	}
	tools, ok := capabilities["tools"].(map[string]any)
	if !ok {
		t.Fatalf("tools capability type = %T", capabilities["tools"])
	}
	if got := tools["listChanged"]; got != false {
		t.Fatalf("tools.listChanged = %v, want false", got)
	}
}

func TestInitializeAcceptsPreviousCurrentProtocol(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	result, err := server.handle(context.Background(), "initialize", json.RawMessage(`{
		"protocolVersion": "2025-03-26"
	}`))
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	payload := result.(map[string]any)
	if got := payload["protocolVersion"]; got != protocolVersionCurrent {
		t.Fatalf("protocolVersion = %v, want %q", got, protocolVersionCurrent)
	}
}

func TestInitializeAcceptsLegacyProtocol(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	result, err := server.handle(context.Background(), "initialize", json.RawMessage(`{
		"protocolVersion": "2024-11-05"
	}`))
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	payload := result.(map[string]any)
	if got := payload["protocolVersion"]; got != protocolVersionLegacy {
		t.Fatalf("protocolVersion = %v, want %q", got, protocolVersionLegacy)
	}
}

func TestInitializeRejectsUnsupportedProtocol(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	_, err := server.handle(context.Background(), "initialize", json.RawMessage(`{
		"protocolVersion": "1.0.0"
	}`))
	if err == nil {
		t.Fatal("expected unsupported protocol version error")
	}
}

func TestEmptyListings(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	for _, method := range []string{"resources/list", "resources/templates/list", "prompts/list"} {
		if _, err := server.handle(context.Background(), method, nil); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
}

func TestHandleMessageSupportsBatchRequests(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	responses, handled, err := server.handleMessage(context.Background(), []byte(`[
		{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}},
		{"jsonrpc":"2.0","method":"notifications/initialized"},
		{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{}}
	]`))
	if err != nil {
		t.Fatalf("handleMessage: %v", err)
	}
	if !handled {
		t.Fatal("expected batch to be handled")
	}
	if len(responses) != 2 {
		t.Fatalf("response count = %d, want 2", len(responses))
	}
	if responses[0].ID != float64(1) && responses[0].ID != 1 {
		t.Fatalf("first response id = %#v, want 1", responses[0].ID)
	}
	if responses[1].ID != float64(2) && responses[1].ID != 2 {
		t.Fatalf("second response id = %#v, want 2", responses[1].ID)
	}
}

func TestTimelineQueryReturnsPagedEventsAndCursor(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	base := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	for _, event := range []memory.Event{
		{EventID: "evt-1", Timestamp: base, LogicalTS: "0001", ProjectID: "repo-123", BranchID: "main", Type: "command.executed"},
		{EventID: "evt-2", Timestamp: base.Add(time.Minute), LogicalTS: "0002", ProjectID: "repo-123", BranchID: "main", Type: "command.executed"},
		{EventID: "evt-3", Timestamp: base.Add(2 * time.Minute), LogicalTS: "0003", ProjectID: "repo-123", BranchID: "main", Type: "command.executed"},
	} {
		if _, err := server.store.Append(ctx, event); err != nil {
			t.Fatalf("append event %s: %v", event.EventID, err)
		}
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.timeline_query",
		"arguments": {
			"project_id": "repo-123",
			"limit": 2
		}
	}`))
	if err != nil {
		t.Fatalf("timeline_query: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	events := body["events"].([]memory.Event)
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].EventID != "evt-1" || events[1].EventID != "evt-2" {
		t.Fatalf("unexpected page events: %#v", events)
	}
	if got := body["next_cursor"]; got != "0002" {
		t.Fatalf("next_cursor = %#v, want %q", got, "0002")
	}

	result, err = server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.timeline_query",
		"arguments": {
			"project_id": "repo-123",
			"after_logical_ts": "0002",
			"limit": 2
		}
	}`))
	if err != nil {
		t.Fatalf("timeline_query after cursor: %v", err)
	}

	body = result.(map[string]any)["structuredContent"].(map[string]any)
	events = body["events"].([]memory.Event)
	if len(events) != 1 || events[0].EventID != "evt-3" {
		t.Fatalf("unexpected second page: %#v", events)
	}
	if got := body["next_cursor"]; got != "" {
		t.Fatalf("next_cursor = %#v, want empty", got)
	}
}

func TestRecordCommandAppendsStandardEvent(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.record_command",
		"arguments": {
			"project_id": "repo-123",
			"command": "go test ./...",
			"task_id": "task-1",
			"session_id": "session-1",
			"exit_code": 0,
			"duration_ms": 1250,
			"stdout_summary": "all packages passed",
			"metadata": {
				"source": "agent"
			}
		}
	}`))
	if err != nil {
		t.Fatalf("record_command: %v", err)
	}

	event := result.(map[string]any)["structuredContent"].(memory.Event)
	if event.Type != "command.executed" {
		t.Fatalf("event type = %q, want command.executed", event.Type)
	}
	if event.TaskID != "task-1" || event.SessionID != "session-1" {
		t.Fatalf("unexpected event context: %#v", event)
	}
	if got := event.Payload["command"]; got != "go test ./..." {
		t.Fatalf("payload command = %#v, want go test ./...", got)
	}
	if got := event.Payload["exit_code"]; got != float64(0) && got != 0 {
		t.Fatalf("payload exit_code = %#v, want 0", got)
	}
	if got := event.Payload["duration_ms"]; got != float64(1250) && got != 1250 {
		t.Fatalf("payload duration_ms = %#v, want 1250", got)
	}
	if got := event.Payload["stdout_summary"]; got != "all packages passed" {
		t.Fatalf("payload stdout_summary = %#v", got)
	}

	events, err := server.store.Events(ctx, memory.EventQuery{ProjectID: "repo-123"})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "command.executed" {
		t.Fatalf("unexpected stored events: %#v", events)
	}
}

func TestRecordObservationAppendsStandardEvent(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.record_observation",
		"arguments": {
			"project_id": "repo-123",
			"summary": "go test is currently green",
			"kind": "test-status",
			"confidence": 0.9,
			"evidence": ["turn-12", "test-log-1"],
			"related_files": ["/Users/amakhov/www/t4mem/go.mod"]
		}
	}`))
	if err != nil {
		t.Fatalf("record_observation: %v", err)
	}

	event := result.(map[string]any)["structuredContent"].(memory.Event)
	if event.Type != "observation.recorded" {
		t.Fatalf("event type = %q, want observation.recorded", event.Type)
	}
	if got := event.Payload["summary"]; got != "go test is currently green" {
		t.Fatalf("payload summary = %#v", got)
	}
	if got := event.Payload["kind"]; got != "test-status" {
		t.Fatalf("payload kind = %#v", got)
	}
	if got := event.Payload["confidence"]; got != 0.9 {
		t.Fatalf("payload confidence = %#v", got)
	}

	events, err := server.store.Events(ctx, memory.EventQuery{ProjectID: "repo-123"})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "observation.recorded" {
		t.Fatalf("unexpected stored events: %#v", events)
	}
}

func TestRecordDecisionAppendsStandardEvent(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.record_decision",
		"arguments": {
			"project_id": "repo-123",
			"summary": "Use cursor-based timeline pagination",
			"rationale": "Keeps large timelines manageable",
			"alternatives": ["offset pagination", "full scans"],
			"expected_outcome": "stable incremental reads",
			"confidence": 0.95
		}
	}`))
	if err != nil {
		t.Fatalf("record_decision: %v", err)
	}

	event := result.(map[string]any)["structuredContent"].(memory.Event)
	if event.Type != "decision.made" {
		t.Fatalf("event type = %q, want decision.made", event.Type)
	}
	if got := event.Payload["summary"]; got != "Use cursor-based timeline pagination" {
		t.Fatalf("payload summary = %#v", got)
	}
	if got := event.Payload["rationale"]; got != "Keeps large timelines manageable" {
		t.Fatalf("payload rationale = %#v", got)
	}
	if got := event.Payload["expected_outcome"]; got != "stable incremental reads" {
		t.Fatalf("payload expected_outcome = %#v", got)
	}

	events, err := server.store.Events(ctx, memory.EventQuery{ProjectID: "repo-123"})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "decision.made" {
		t.Fatalf("unexpected stored events: %#v", events)
	}
}

func TestTraceDecisionReturnsContextAndFacts(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	obs, err := server.store.Append(ctx, memory.Event{
		EventID:   "evt-observation",
		Timestamp: time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		LogicalTS: "0001",
		ProjectID: "repo-123",
		BranchID:  "main",
		TaskID:    "task-1",
		Type:      "observation.recorded",
		Payload:   map[string]any{"summary": "tests are green"},
	})
	if err != nil {
		t.Fatalf("append observation: %v", err)
	}
	if _, err := server.store.Append(ctx, memory.Event{
		EventID:     "evt-decision",
		Timestamp:   time.Date(2026, 4, 13, 10, 1, 0, 0, time.UTC),
		LogicalTS:   "0002",
		ProjectID:   "repo-123",
		BranchID:    "main",
		TaskID:      "task-1",
		Type:        "decision.made",
		Payload:     map[string]any{"summary": "ship it"},
		CausationID: obs.EventID,
	}); err != nil {
		t.Fatalf("append decision: %v", err)
	}
	_, err = server.store.PromoteFact(ctx, memory.Fact{
		FactID:       "fact-1",
		ProjectID:    "repo-123",
		Scope:        "project",
		ScopeID:      "repo-123",
		Subject:      "repo",
		Predicate:    "status",
		Value:        "green",
		SourceBranch: "main",
	})
	if err != nil {
		t.Fatalf("promote fact: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.trace_decision",
		"arguments": {
			"project_id": "repo-123",
			"decision_event_id": "evt-decision",
			"limit": 5
		}
	}`))
	if err != nil {
		t.Fatalf("trace_decision: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	decision := body["decision"].(memory.Event)
	if decision.EventID != "evt-decision" {
		t.Fatalf("decision event = %#v", decision)
	}
	causationChain := body["causation_chain"].([]memory.Event)
	if len(causationChain) != 1 || causationChain[0].EventID != "evt-observation" {
		t.Fatalf("unexpected causation chain: %#v", causationChain)
	}
	contextual := body["contextual_events"].([]memory.Event)
	if len(contextual) != 1 || contextual[0].EventID != "evt-observation" {
		t.Fatalf("unexpected contextual events: %#v", contextual)
	}
	facts := body["supporting_facts"].([]memory.Fact)
	if len(facts) != 1 || facts[0].FactID != "fact-1" {
		t.Fatalf("unexpected supporting facts: %#v", facts)
	}
}

func TestRecordPlanAppendsStandardEvent(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.record_plan",
		"arguments": {
			"project_id": "repo-123",
			"summary": "Stabilize MCP compatibility",
			"steps": ["support current protocol", "add compatibility tests"],
			"status": "in_progress"
		}
	}`))
	if err != nil {
		t.Fatalf("record_plan: %v", err)
	}

	event := result.(map[string]any)["structuredContent"].(memory.Event)
	if event.Type != "plan.created" {
		t.Fatalf("event type = %q, want plan.created", event.Type)
	}
	if got := event.Payload["summary"]; got != "Stabilize MCP compatibility" {
		t.Fatalf("payload summary = %#v", got)
	}
	if got := event.Payload["status"]; got != "in_progress" {
		t.Fatalf("payload status = %#v", got)
	}

	events, err := server.store.Events(ctx, memory.EventQuery{ProjectID: "repo-123"})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "plan.created" {
		t.Fatalf("unexpected stored events: %#v", events)
	}
}

func TestUpdatePlanAppendsStandardEvent(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.update_plan",
		"arguments": {
			"project_id": "repo-123",
			"summary": "MCP compatibility work complete",
			"steps": ["support current protocol", "add compatibility tests", "verify restart path"],
			"status": "completed"
		}
	}`))
	if err != nil {
		t.Fatalf("update_plan: %v", err)
	}

	event := result.(map[string]any)["structuredContent"].(memory.Event)
	if event.Type != "plan.updated" {
		t.Fatalf("event type = %q, want plan.updated", event.Type)
	}
	if got := event.Payload["summary"]; got != "MCP compatibility work complete" {
		t.Fatalf("payload summary = %#v", got)
	}
	if got := event.Payload["status"]; got != "completed" {
		t.Fatalf("payload status = %#v", got)
	}

	events, err := server.store.Events(ctx, memory.EventQuery{ProjectID: "repo-123"})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "plan.updated" {
		t.Fatalf("unexpected stored events: %#v", events)
	}
}

func TestRecentContextReturnsNewestFirstWithSummary(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	base := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	for _, event := range []memory.Event{
		{EventID: "evt-1", Timestamp: base, LogicalTS: "0001", ProjectID: "repo-123", BranchID: "main", Type: "plan.created"},
		{EventID: "evt-2", Timestamp: base.Add(time.Minute), LogicalTS: "0002", ProjectID: "repo-123", BranchID: "main", Type: "command.executed"},
		{EventID: "evt-3", Timestamp: base.Add(2 * time.Minute), LogicalTS: "0003", ProjectID: "repo-123", BranchID: "main", Type: "decision.made"},
	} {
		if _, err := server.store.Append(ctx, event); err != nil {
			t.Fatalf("append event %s: %v", event.EventID, err)
		}
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.recent_context",
		"arguments": {
			"project_id": "repo-123",
			"limit": 2
		}
	}`))
	if err != nil {
		t.Fatalf("recent_context: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	events := body["events"].([]memory.Event)
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].EventID != "evt-3" || events[1].EventID != "evt-2" {
		t.Fatalf("unexpected recent order: %#v", events)
	}

	summary := body["summary"].(map[string]any)
	if got := summary["event_count"]; got != 2 {
		t.Fatalf("summary event_count = %#v, want 2", got)
	}
	typeCounts := summary["type_counts"].(map[string]int)
	if typeCounts["decision.made"] != 1 || typeCounts["command.executed"] != 1 {
		t.Fatalf("unexpected type_counts: %#v", typeCounts)
	}
}

func TestListFactsReturnsPagedFactsAndCursor(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	base := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	for _, fact := range []memory.Fact{
		{FactID: "fact-1", ProjectID: "repo-123", Scope: "project", ScopeID: "repo-123", Subject: "repo", Predicate: "feature", Value: "mcp", UpdatedAt: base},
		{FactID: "fact-2", ProjectID: "repo-123", Scope: "project", ScopeID: "repo-123", Subject: "repo", Predicate: "feature", Value: "memory", UpdatedAt: base.Add(time.Minute)},
		{FactID: "fact-3", ProjectID: "repo-123", Scope: "project", ScopeID: "repo-123", Subject: "repo", Predicate: "feature", Value: "pagination", UpdatedAt: base.Add(2 * time.Minute)},
	} {
		if _, err := server.store.PromoteFact(ctx, fact); err != nil {
			t.Fatalf("promote fact %s: %v", fact.FactID, err)
		}
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.list_facts",
		"arguments": {
			"project_id": "repo-123",
			"limit": 2
		}
	}`))
	if err != nil {
		t.Fatalf("list_facts: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	facts := body["facts"].([]memory.Fact)
	if len(facts) != 2 {
		t.Fatalf("fact count = %d, want 2", len(facts))
	}
	if facts[0].FactID != "fact-1" || facts[1].FactID != "fact-2" {
		t.Fatalf("unexpected first page facts: %#v", facts)
	}
	if got := body["next_cursor"]; got != "fact-2" {
		t.Fatalf("next_cursor = %#v, want fact-2", got)
	}

	result, err = server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.list_facts",
		"arguments": {
			"project_id": "repo-123",
			"after_fact_id": "fact-2",
			"limit": 2
		}
	}`))
	if err != nil {
		t.Fatalf("list_facts after cursor: %v", err)
	}

	body = result.(map[string]any)["structuredContent"].(map[string]any)
	facts = body["facts"].([]memory.Fact)
	if len(facts) != 1 || facts[0].FactID != "fact-3" {
		t.Fatalf("unexpected second page facts: %#v", facts)
	}
	if got := body["next_cursor"]; got != "" {
		t.Fatalf("next_cursor = %#v, want empty", got)
	}
}

func TestPromoteFactFromEventsUsesEvidenceRefs(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}
	for _, event := range []memory.Event{
		{EventID: "evt-1", Timestamp: time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC), LogicalTS: "0001", ProjectID: "repo-123", BranchID: "main", Type: "observation.recorded"},
		{EventID: "evt-2", Timestamp: time.Date(2026, 4, 13, 10, 1, 0, 0, time.UTC), LogicalTS: "0002", ProjectID: "repo-123", BranchID: "main", Type: "decision.made"},
	} {
		if _, err := server.store.Append(ctx, event); err != nil {
			t.Fatalf("append event %s: %v", event.EventID, err)
		}
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.promote_fact_from_events",
		"arguments": {
			"project_id": "repo-123",
			"scope": "project",
			"scope_id": "repo-123",
			"subject": "repo",
			"predicate": "decision_state",
			"value": "ready",
			"confidence": 0.8,
			"evidence_event_ids": ["evt-1", "evt-2"]
		}
	}`))
	if err != nil {
		t.Fatalf("promote_fact_from_events: %v", err)
	}

	fact := result.(map[string]any)["structuredContent"].(memory.Fact)
	if len(fact.EvidenceRefs) != 2 || fact.EvidenceRefs[0] != "evt-1" || fact.SourceBranch != "main" {
		t.Fatalf("unexpected fact evidence: %#v", fact)
	}
}

func TestBranchSummaryAndDiffSummary(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}
	if _, err := server.store.Append(ctx, memory.Event{EventID: "evt-main", Timestamp: time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC), LogicalTS: "0001", ProjectID: "repo-123", BranchID: "main", Type: "command.executed"}); err != nil {
		t.Fatalf("append main event: %v", err)
	}
	branch, err := server.store.Branch(ctx, memory.BranchFrom{ProjectID: "repo-123", From: "main", Name: "alt"})
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := server.store.Append(ctx, memory.Event{EventID: "evt-alt", Timestamp: time.Date(2026, 4, 13, 10, 1, 0, 0, time.UTC), LogicalTS: "0002", ProjectID: "repo-123", BranchID: branch.BranchID, Type: "decision.made"}); err != nil {
		t.Fatalf("append alt event: %v", err)
	}
	if _, err := server.store.PromoteFact(ctx, memory.Fact{FactID: "fact-alt", ProjectID: "repo-123", Scope: "project", ScopeID: "repo-123", Subject: "repo", Predicate: "mode", Value: "alt", SourceBranch: branch.BranchID}); err != nil {
		t.Fatalf("promote alt fact: %v", err)
	}

	summaryResult, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.branch_summary",
		"arguments": {
			"project_id": "repo-123",
			"branch_id": "alt"
		}
	}`))
	if err != nil {
		t.Fatalf("branch_summary: %v", err)
	}
	summaryBody := summaryResult.(map[string]any)["structuredContent"].(map[string]any)
	branchSummary := summaryBody["summary"].(map[string]any)
	if branchSummary["event_count"] != 1 || branchSummary["fact_count"] != 1 {
		t.Fatalf("unexpected branch summary: %#v", branchSummary)
	}

	diffResult, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.branch_diff_summary",
		"arguments": {
			"project_id": "repo-123",
			"left_branch_id": "main",
			"right_branch_id": "alt"
		}
	}`))
	if err != nil {
		t.Fatalf("branch_diff_summary: %v", err)
	}
	diffBody := diffResult.(map[string]any)["structuredContent"].(map[string]any)
	diffSummary := diffBody["summary"].(map[string]any)
	if diffSummary["right_only_event_count"] != 1 || diffSummary["right_only_fact_count"] != 1 {
		t.Fatalf("unexpected diff summary: %#v", diffSummary)
	}
}

func TestAdoptBranchWithReasonAppendsAdoptionEvent(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}
	branch, err := server.store.Branch(ctx, memory.BranchFrom{ProjectID: "repo-123", From: "main", Name: "winner"})
	if err != nil {
		t.Fatalf("branch: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.adopt_branch_with_reason",
		"arguments": {
			"project_id": "repo-123",
			"branch_id": "winner",
			"reason": "Passed the experiment",
			"summary": "winner branch outperformed baseline"
		}
	}`))
	if err != nil {
		t.Fatalf("adopt_branch_with_reason: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	adoptionEvent := body["adoption_event"].(memory.Event)
	if adoptionEvent.Type != "branch.adopted" || adoptionEvent.BranchID != branch.BranchID {
		t.Fatalf("unexpected adoption event: %#v", adoptionEvent)
	}
	if got := adoptionEvent.Payload["reason"]; got != "Passed the experiment" {
		t.Fatalf("unexpected adoption payload: %#v", adoptionEvent.Payload)
	}
}

func TestFactSummaryReturnsFactsAndGroupingSummary(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	for _, fact := range []memory.Fact{
		{FactID: "fact-1", ProjectID: "repo-123", Scope: "project", ScopeID: "repo-123", Subject: "repo", Predicate: "uses", Value: "mcp"},
		{FactID: "fact-2", ProjectID: "repo-123", Scope: "project", ScopeID: "repo-123", Subject: "repo", Predicate: "uses", Value: "memory"},
		{FactID: "fact-3", ProjectID: "repo-123", Scope: "project", ScopeID: "repo-123", Subject: "agent", Predicate: "prefers", Value: "small patches"},
	} {
		if _, err := server.store.PromoteFact(ctx, fact); err != nil {
			t.Fatalf("promote fact %s: %v", fact.FactID, err)
		}
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.fact_summary",
		"arguments": {
			"project_id": "repo-123",
			"limit": 10
		}
	}`))
	if err != nil {
		t.Fatalf("fact_summary: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	facts := body["facts"].([]memory.Fact)
	if len(facts) != 3 {
		t.Fatalf("fact count = %d, want 3", len(facts))
	}

	summary := body["summary"].(map[string]any)
	if got := summary["fact_count"]; got != 3 {
		t.Fatalf("summary fact_count = %#v, want 3", got)
	}
	bySubject := summary["by_subject"].(map[string]int)
	if bySubject["repo"] != 2 || bySubject["agent"] != 1 {
		t.Fatalf("unexpected by_subject: %#v", bySubject)
	}
	byPredicate := summary["by_predicate"].(map[string]int)
	if byPredicate["uses"] != 2 || byPredicate["prefers"] != 1 {
		t.Fatalf("unexpected by_predicate: %#v", byPredicate)
	}
}

func TestTaskSnapshotCombinesBranchEventsFactsStateAndDecision(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}
	if _, err := server.store.SetState(ctx, memory.StateKey{
		ProjectID: "repo-123",
		Scope:     "task",
		ScopeID:   "task-1",
		Field:     "branch_id",
	}, "main"); err != nil {
		t.Fatalf("set branch state: %v", err)
	}
	if _, err := server.store.SetState(ctx, memory.StateKey{
		ProjectID: "repo-123",
		Scope:     "task",
		ScopeID:   "task-1",
		Field:     "current_plan",
	}, map[string]any{"step": "verify output"}); err != nil {
		t.Fatalf("set plan state: %v", err)
	}

	base := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	if _, err := server.store.Append(ctx, memory.Event{
		EventID:   "evt-obs",
		Timestamp: base,
		LogicalTS: "0001",
		ProjectID: "repo-123",
		BranchID:  "main",
		TaskID:    "task-1",
		Type:      "observation.recorded",
	}); err != nil {
		t.Fatalf("append observation: %v", err)
	}
	if _, err := server.store.Append(ctx, memory.Event{
		EventID:     "evt-decision",
		Timestamp:   base.Add(time.Minute),
		LogicalTS:   "0002",
		ProjectID:   "repo-123",
		BranchID:    "main",
		TaskID:      "task-1",
		Type:        "decision.made",
		CausationID: "evt-obs",
		Payload:     map[string]any{"summary": "continue rollout"},
	}); err != nil {
		t.Fatalf("append decision: %v", err)
	}
	if _, err := server.store.PromoteFact(ctx, memory.Fact{
		FactID:       "fact-1",
		ProjectID:    "repo-123",
		Scope:        "project",
		ScopeID:      "repo-123",
		Subject:      "repo",
		Predicate:    "status",
		Value:        "healthy",
		SourceBranch: "main",
	}); err != nil {
		t.Fatalf("promote fact: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.task_snapshot",
		"arguments": {
			"project_id": "repo-123",
			"task_id": "task-1",
			"state_fields": ["current_plan"],
			"limit": 5
		}
	}`))
	if err != nil {
		t.Fatalf("task_snapshot: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	summary := body["summary"].(map[string]any)
	if summary["branch_id"] != "main" {
		t.Fatalf("unexpected snapshot branch: %#v", summary)
	}
	if summary["has_decision_trace"] != true {
		t.Fatalf("expected decision trace: %#v", summary)
	}

	state := body["state"].(map[string]memory.StateEntry)
	if _, ok := state["current_plan"]; !ok {
		t.Fatalf("expected current_plan state: %#v", state)
	}

	recentContext := body["recent_context"].(map[string]any)
	events := recentContext["events"].([]memory.Event)
	if len(events) != 2 || events[0].EventID != "evt-decision" {
		t.Fatalf("unexpected snapshot events: %#v", events)
	}

	factSummary := body["fact_summary"].(map[string]any)
	facts := factSummary["facts"].([]memory.Fact)
	if len(facts) != 1 || facts[0].FactID != "fact-1" {
		t.Fatalf("unexpected snapshot facts: %#v", facts)
	}
}

func TestSessionSnapshotCombinesSessionStateAndDecision(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}
	if _, err := server.store.SetState(ctx, memory.StateKey{
		ProjectID: "repo-123",
		Scope:     "session",
		ScopeID:   "session-1",
		Field:     "current_focus",
	}, "verify branch adoption"); err != nil {
		t.Fatalf("set session state: %v", err)
	}
	base := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	if _, err := server.store.Append(ctx, memory.Event{
		EventID:   "evt-session-obs",
		Timestamp: base,
		LogicalTS: "0001",
		ProjectID: "repo-123",
		BranchID:  "main",
		SessionID: "session-1",
		Type:      "observation.recorded",
	}); err != nil {
		t.Fatalf("append session observation: %v", err)
	}
	if _, err := server.store.Append(ctx, memory.Event{
		EventID:     "evt-session-decision",
		Timestamp:   base.Add(time.Minute),
		LogicalTS:   "0002",
		ProjectID:   "repo-123",
		BranchID:    "main",
		SessionID:   "session-1",
		Type:        "decision.made",
		CausationID: "evt-session-obs",
	}); err != nil {
		t.Fatalf("append session decision: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.session_snapshot",
		"arguments": {
			"project_id": "repo-123",
			"session_id": "session-1",
			"state_fields": ["current_focus"],
			"limit": 5
		}
	}`))
	if err != nil {
		t.Fatalf("session_snapshot: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	summary := body["summary"].(map[string]any)
	if summary["session_id"] != "session-1" || summary["has_decision_trace"] != true {
		t.Fatalf("unexpected session summary: %#v", summary)
	}
	state := body["state"].(map[string]memory.StateEntry)
	if _, ok := state["current_focus"]; !ok {
		t.Fatalf("expected session state: %#v", state)
	}
}

func TestProjectSnapshotCombinesProjectContext(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()
	if _, err := server.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}
	base := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	for _, event := range []memory.Event{
		{EventID: "evt-1", Timestamp: base, LogicalTS: "0001", ProjectID: "repo-123", BranchID: "main", Type: "command.executed"},
		{EventID: "evt-2", Timestamp: base.Add(time.Minute), LogicalTS: "0002", ProjectID: "repo-123", BranchID: "main", Type: "decision.made"},
	} {
		if _, err := server.store.Append(ctx, event); err != nil {
			t.Fatalf("append event %s: %v", event.EventID, err)
		}
	}
	if _, err := server.store.PromoteFact(ctx, memory.Fact{
		FactID:    "fact-1",
		ProjectID: "repo-123",
		Scope:     "project",
		ScopeID:   "repo-123",
		Subject:   "repo",
		Predicate: "health",
		Value:     "good",
	}); err != nil {
		t.Fatalf("promote fact: %v", err)
	}

	result, err := server.handleToolCall(ctx, json.RawMessage(`{
		"name": "memory.project_snapshot",
		"arguments": {
			"project_id": "repo-123",
			"limit": 5
		}
	}`))
	if err != nil {
		t.Fatalf("project_snapshot: %v", err)
	}

	body := result.(map[string]any)["structuredContent"].(map[string]any)
	summary := body["summary"].(map[string]any)
	if summary["default_branch_id"] != "main" || summary["fact_count"] != 1 {
		t.Fatalf("unexpected project summary: %#v", summary)
	}
	recentContext := body["recent_context"].(map[string]any)
	events := recentContext["events"].([]memory.Event)
	if len(events) != 2 || events[0].EventID != "evt-2" {
		t.Fatalf("unexpected project events: %#v", events)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	store, err := memory.Open(memory.Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	return New(store, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
}
