package memory

import "context"

type Store interface {
	OpenProject(context.Context, OpenProjectInput) (Project, error)
	Append(context.Context, Event) (Event, error)
	GetState(context.Context, StateKey) (StateEntry, bool, error)
	SetState(context.Context, StateKey, any) (StateEntry, error)
	Facts(context.Context, FactsQuery) ([]Fact, error)
	PromoteFact(context.Context, Fact) (Fact, error)
	Events(context.Context, EventQuery) ([]Event, error)
	Checkpoint(context.Context, string, string, map[string]any) (Checkpoint, error)
	Branch(context.Context, BranchFrom) (Branch, error)
	CompareBranches(context.Context, string, string, string) (BranchComparison, error)
	AdoptBranch(context.Context, string, string) (Branch, error)
	GetBranch(context.Context, string, string) (Branch, error)
}
