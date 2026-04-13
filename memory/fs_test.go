package memory

import (
	"context"
	"testing"
	"time"
)

func TestProjectLifecycleAndState(t *testing.T) {
	t.Parallel()

	store, err := Open(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()

	project, err := store.OpenProject(ctx, OpenProjectInput{
		ProjectID: "repo-123",
		RepoRef:   "github.com/example/repo",
		Language:  "go",
	})
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	if project.DefaultBranchID != defaultBranchID {
		t.Fatalf("default branch = %q, want %q", project.DefaultBranchID, defaultBranchID)
	}

	entry, err := store.SetState(ctx, StateKey{
		ProjectID: "repo-123",
		Scope:     "task",
		ScopeID:   "task-77",
		Field:     "current_plan",
	}, map[string]any{"step": "run tests"})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}
	if entry.Scope != "task" {
		t.Fatalf("scope = %q, want task", entry.Scope)
	}

	got, ok, err := store.GetState(ctx, StateKey{
		ProjectID: "repo-123",
		Scope:     "task",
		ScopeID:   "task-77",
		Field:     "current_plan",
	})
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if !ok {
		t.Fatal("expected state entry to exist")
	}
	value, ok := got.Value.(map[string]any)
	if !ok || value["step"] != "run tests" {
		t.Fatalf("unexpected state value: %#v", got.Value)
	}
}

func TestEventsFactsBranchesAndCheckpoint(t *testing.T) {
	t.Parallel()

	store, err := Open(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	if _, err := store.OpenProject(ctx, OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	mainEvent, err := store.Append(ctx, Event{
		ProjectID: "repo-123",
		BranchID:  "main",
		TaskID:    "task-1",
		Type:      "command.executed",
		Payload:   map[string]any{"command": "go test ./..."},
	})
	if err != nil {
		t.Fatalf("append event: %v", err)
	}
	if mainEvent.EventID == "" || mainEvent.LogicalTS == "" {
		t.Fatalf("expected event IDs to be populated: %#v", mainEvent)
	}

	checkpoint, err := store.Checkpoint(ctx, "repo-123", "main", map[string]any{"label": "baseline"})
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if checkpoint.LogicalTS == "" {
		t.Fatal("expected checkpoint logical ts")
	}

	branch, err := store.Branch(ctx, BranchFrom{
		ProjectID:        "repo-123",
		From:             "main",
		Name:             "debug-race",
		FromCheckpointID: checkpoint.CheckpointID,
	})
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if branch.ParentBranchID != "main" {
		t.Fatalf("parent branch = %q, want main", branch.ParentBranchID)
	}

	if _, err := store.Append(ctx, Event{
		ProjectID: "repo-123",
		BranchID:  branch.BranchID,
		TaskID:    "task-1",
		Type:      "error.observed",
		Payload:   map[string]any{"error": "race detected"},
	}); err != nil {
		t.Fatalf("append branch event: %v", err)
	}

	fact, err := store.PromoteFact(ctx, Fact{
		ProjectID:    "repo-123",
		Scope:        "project",
		ScopeID:      "repo-123",
		Subject:      "repo",
		Predicate:    "test_command",
		Value:        "go test ./...",
		Confidence:   0.95,
		SourceBranch: branch.BranchID,
	})
	if err != nil {
		t.Fatalf("promote fact: %v", err)
	}
	if fact.FactID == "" {
		t.Fatal("expected fact ID")
	}

	events, err := store.Events(ctx, EventQuery{
		ProjectID: "repo-123",
		BranchID:  branch.BranchID,
		TaskID:    "task-1",
	})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "error.observed" {
		t.Fatalf("unexpected branch events: %#v", events)
	}

	facts, err := store.Facts(ctx, FactsQuery{
		ProjectID: "repo-123",
		BranchID:  branch.BranchID,
	})
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	if len(facts) != 1 || facts[0].Predicate != "test_command" {
		t.Fatalf("unexpected facts: %#v", facts)
	}

	comparison, err := store.CompareBranches(ctx, "repo-123", "main", branch.BranchID)
	if err != nil {
		t.Fatalf("compare branches: %v", err)
	}
	if len(comparison.RightOnlyEvents) != 1 {
		t.Fatalf("expected one right-only event, got %#v", comparison.RightOnlyEvents)
	}
	if len(comparison.RightOnlyFacts) != 1 {
		t.Fatalf("expected one right-only fact, got %#v", comparison.RightOnlyFacts)
	}

	adopted, err := store.AdoptBranch(ctx, "repo-123", branch.BranchID)
	if err != nil {
		t.Fatalf("adopt branch: %v", err)
	}
	if adopted.AdoptedAt == nil {
		t.Fatal("expected adopted timestamp")
	}
}

func TestEventsAreSortedAndRespectTimeWindow(t *testing.T) {
	t.Parallel()

	store, err := Open(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	if _, err := store.OpenProject(ctx, OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	base := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	inputs := []Event{
		{
			EventID:   "evt-late",
			Timestamp: base.Add(2 * time.Minute),
			LogicalTS: "00000000000000000003",
			ProjectID: "repo-123",
			BranchID:  "main",
			Type:      "command.executed",
			Payload:   map[string]any{"command": "late"},
		},
		{
			EventID:   "evt-early",
			Timestamp: base,
			LogicalTS: "00000000000000000001",
			ProjectID: "repo-123",
			BranchID:  "main",
			Type:      "command.executed",
			Payload:   map[string]any{"command": "early"},
		},
		{
			EventID:   "evt-middle",
			Timestamp: base.Add(1 * time.Minute),
			LogicalTS: "00000000000000000002",
			ProjectID: "repo-123",
			BranchID:  "main",
			Type:      "command.executed",
			Payload:   map[string]any{"command": "middle"},
		},
	}
	for _, event := range inputs {
		if _, err := store.Append(ctx, event); err != nil {
			t.Fatalf("append event %s: %v", event.EventID, err)
		}
	}

	events, err := store.Events(ctx, EventQuery{
		ProjectID: "repo-123",
		Since:     base.Add(30 * time.Second),
		Until:     base.Add(90 * time.Second),
	})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].EventID != "evt-middle" {
		t.Fatalf("event id = %q, want evt-middle", events[0].EventID)
	}

	allEvents, err := store.Events(ctx, EventQuery{ProjectID: "repo-123"})
	if err != nil {
		t.Fatalf("all events: %v", err)
	}
	if len(allEvents) != 3 {
		t.Fatalf("all event count = %d, want 3", len(allEvents))
	}
	if allEvents[0].EventID != "evt-early" || allEvents[1].EventID != "evt-middle" || allEvents[2].EventID != "evt-late" {
		t.Fatalf("unexpected order: %#v", allEvents)
	}

	limited, err := store.Events(ctx, EventQuery{ProjectID: "repo-123", Limit: 2})
	if err != nil {
		t.Fatalf("limited events: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("limited count = %d, want 2", len(limited))
	}
	if limited[0].EventID != "evt-early" || limited[1].EventID != "evt-middle" {
		t.Fatalf("unexpected limited order: %#v", limited)
	}

	afterMiddle, err := store.Events(ctx, EventQuery{
		ProjectID:      "repo-123",
		AfterLogicalTS: "00000000000000000002",
	})
	if err != nil {
		t.Fatalf("events after cursor: %v", err)
	}
	if len(afterMiddle) != 1 || afterMiddle[0].EventID != "evt-late" {
		t.Fatalf("unexpected events after cursor: %#v", afterMiddle)
	}
}

func TestFactsAreSortedAndRespectCursor(t *testing.T) {
	t.Parallel()

	store, err := Open(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	if _, err := store.OpenProject(ctx, OpenProjectInput{ProjectID: "repo-123"}); err != nil {
		t.Fatalf("open project: %v", err)
	}

	base := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	inputs := []Fact{
		{
			FactID:       "fact-03",
			ProjectID:    "repo-123",
			Scope:        "project",
			ScopeID:      "repo-123",
			Subject:      "repo",
			Predicate:    "uses_feature",
			Value:        "pagination",
			Confidence:   0.8,
			UpdatedAt:    base.Add(2 * time.Minute),
			SourceBranch: "main",
		},
		{
			FactID:       "fact-01",
			ProjectID:    "repo-123",
			Scope:        "project",
			ScopeID:      "repo-123",
			Subject:      "repo",
			Predicate:    "uses_feature",
			Value:        "mcp",
			Confidence:   0.9,
			UpdatedAt:    base,
			SourceBranch: "main",
		},
		{
			FactID:       "fact-02",
			ProjectID:    "repo-123",
			Scope:        "project",
			ScopeID:      "repo-123",
			Subject:      "repo",
			Predicate:    "uses_feature",
			Value:        "memory",
			Confidence:   0.85,
			UpdatedAt:    base.Add(1 * time.Minute),
			SourceBranch: "main",
		},
	}
	for _, fact := range inputs {
		if _, err := store.PromoteFact(ctx, fact); err != nil {
			t.Fatalf("promote fact %s: %v", fact.FactID, err)
		}
	}

	facts, err := store.Facts(ctx, FactsQuery{ProjectID: "repo-123"})
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	if len(facts) != 3 {
		t.Fatalf("fact count = %d, want 3", len(facts))
	}
	if facts[0].FactID != "fact-01" || facts[1].FactID != "fact-02" || facts[2].FactID != "fact-03" {
		t.Fatalf("unexpected fact order: %#v", facts)
	}

	limited, err := store.Facts(ctx, FactsQuery{ProjectID: "repo-123", Limit: 2})
	if err != nil {
		t.Fatalf("limited facts: %v", err)
	}
	if len(limited) != 2 || limited[0].FactID != "fact-01" || limited[1].FactID != "fact-02" {
		t.Fatalf("unexpected limited facts: %#v", limited)
	}

	afterMiddle, err := store.Facts(ctx, FactsQuery{ProjectID: "repo-123", AfterFactID: "fact-02"})
	if err != nil {
		t.Fatalf("facts after cursor: %v", err)
	}
	if len(afterMiddle) != 1 || afterMiddle[0].FactID != "fact-03" {
		t.Fatalf("unexpected facts after cursor: %#v", afterMiddle)
	}
}
