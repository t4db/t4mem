package memory

import (
	"time"

	"github.com/t4db/t4/pkg/object"
)

type Config struct {
	// DataDir is the local t4 data directory.
	DataDir string

	// RootDir is kept as a backward-compatible alias for DataDir.
	RootDir string

	// ObjectStore enables t4's durable S3/object-store-backed mode when set.
	ObjectStore object.Store
}

type Agent struct {
	AgentID   string         `json:"agent_id"`
	Name      string         `json:"name,omitempty"`
	Model     string         `json:"model,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Project struct {
	ProjectID       string         `json:"project_id"`
	RepoRef         string         `json:"repo_ref,omitempty"`
	Language        string         `json:"language,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	DefaultBranchID string         `json:"default_branch_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type Session struct {
	SessionID string         `json:"session_id"`
	AgentID   string         `json:"agent_id,omitempty"`
	ProjectID string         `json:"project_id"`
	BranchID  string         `json:"branch_id"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Task struct {
	TaskID       string    `json:"task_id"`
	SessionID    string    `json:"session_id,omitempty"`
	ProjectID    string    `json:"project_id"`
	Title        string    `json:"title,omitempty"`
	Status       string    `json:"status,omitempty"`
	BranchID     string    `json:"branch_id"`
	ParentTaskID string    `json:"parent_task_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Event struct {
	EventID       string         `json:"event_id"`
	Timestamp     time.Time      `json:"timestamp"`
	LogicalTS     string         `json:"logical_ts"`
	ProjectID     string         `json:"project_id"`
	SessionID     string         `json:"session_id,omitempty"`
	TaskID        string         `json:"task_id,omitempty"`
	BranchID      string         `json:"branch_id"`
	Type          string         `json:"type"`
	Payload       map[string]any `json:"payload,omitempty"`
	CausationID   string         `json:"causation_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
}

type Fact struct {
	FactID       string    `json:"fact_id"`
	ProjectID    string    `json:"project_id"`
	Scope        string    `json:"scope"`
	ScopeID      string    `json:"scope_id"`
	Subject      string    `json:"subject"`
	Predicate    string    `json:"predicate"`
	Value        any       `json:"value"`
	Confidence   float64   `json:"confidence"`
	EvidenceRefs []string  `json:"evidence_refs,omitempty"`
	SourceBranch string    `json:"source_branch,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type StateKey struct {
	ProjectID string `json:"project_id"`
	Scope     string `json:"scope"`
	ScopeID   string `json:"scope_id"`
	Field     string `json:"field"`
	BranchID  string `json:"branch_id,omitempty"`
}

type StateEntry struct {
	StateKey
	Value     any       `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Branch struct {
	ProjectID          string     `json:"project_id"`
	BranchID           string     `json:"branch_id"`
	Name               string     `json:"name"`
	ParentBranchID     string     `json:"parent_branch_id,omitempty"`
	BaseCheckpointID   string     `json:"base_checkpoint_id,omitempty"`
	BaseEventLogicalTS string     `json:"base_event_logical_ts,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	AdoptedAt          *time.Time `json:"adopted_at,omitempty"`
}

type Checkpoint struct {
	CheckpointID string         `json:"checkpoint_id"`
	ProjectID    string         `json:"project_id"`
	BranchID     string         `json:"branch_id"`
	LogicalTS    string         `json:"logical_ts"`
	CreatedAt    time.Time      `json:"created_at"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type BranchFrom struct {
	ProjectID        string `json:"project_id"`
	From             string `json:"from"`
	Name             string `json:"name"`
	FromCheckpointID string `json:"from_checkpoint_id,omitempty"`
}

type BranchComparison struct {
	ProjectID        string  `json:"project_id"`
	LeftBranchID     string  `json:"left_branch_id"`
	RightBranchID    string  `json:"right_branch_id"`
	LeftOnlyEvents   []Event `json:"left_only_events"`
	RightOnlyEvents  []Event `json:"right_only_events"`
	LeftOnlyFacts    []Fact  `json:"left_only_facts"`
	RightOnlyFacts   []Fact  `json:"right_only_facts"`
	CommonBaseBranch string  `json:"common_base_branch,omitempty"`
}

type EventQuery struct {
	ProjectID      string    `json:"project_id"`
	BranchID       string    `json:"branch_id,omitempty"`
	TaskID         string    `json:"task_id,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	Type           string    `json:"type,omitempty"`
	AfterLogicalTS string    `json:"after_logical_ts,omitempty"`
	Since          time.Time `json:"since,omitempty"`
	Until          time.Time `json:"until,omitempty"`
	Limit          int       `json:"limit,omitempty"`
}

type FactsQuery struct {
	ProjectID   string `json:"project_id"`
	Scope       string `json:"scope,omitempty"`
	ScopeID     string `json:"scope_id,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Predicate   string `json:"predicate,omitempty"`
	BranchID    string `json:"branch_id,omitempty"`
	AfterFactID string `json:"after_fact_id,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type OpenProjectInput struct {
	ProjectID       string         `json:"project_id"`
	RepoRef         string         `json:"repo_ref,omitempty"`
	Language        string         `json:"language,omitempty"`
	DefaultBranchID string         `json:"default_branch_id,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}
