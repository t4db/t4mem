package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/t4db/t4"
)

const defaultBranchID = "main"

type T4Store struct {
	node *t4.Node
}

func Open(cfg Config) (*T4Store, error) {
	dataDir := strings.TrimSpace(cfg.DataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(cfg.RootDir)
	}
	if dataDir == "" {
		return nil, errors.New("memory data dir is required")
	}

	node, err := t4.Open(t4.Config{
		DataDir:     dataDir,
		ObjectStore: cfg.ObjectStore,
		Logger:      t4.NoopLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("open t4 node: %w", err)
	}
	return &T4Store{node: node}, nil
}

func (s *T4Store) Close() error {
	if s == nil || s.node == nil {
		return nil
	}
	return s.node.Close()
}

func (s *T4Store) OpenProject(ctx context.Context, input OpenProjectInput) (Project, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return Project{}, errors.New("project_id is required")
	}
	if existing, ok, err := getJSON[Project](s.node, s.projectKey(input.ProjectID)); err != nil {
		return Project{}, err
	} else if ok {
		return existing, nil
	}

	now := time.Now().UTC()
	project := Project{
		ProjectID:       input.ProjectID,
		RepoRef:         input.RepoRef,
		Language:        input.Language,
		Metadata:        input.Metadata,
		DefaultBranchID: fallback(input.DefaultBranchID, defaultBranchID),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.putJSON(ctx, s.projectKey(input.ProjectID), project); err != nil {
		return Project{}, err
	}
	branch := Branch{
		ProjectID: input.ProjectID,
		BranchID:  project.DefaultBranchID,
		Name:      project.DefaultBranchID,
		CreatedAt: now,
	}
	if err := s.putJSON(ctx, s.branchKey(input.ProjectID, project.DefaultBranchID), branch); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (s *T4Store) Append(ctx context.Context, event Event) (Event, error) {
	if err := s.ensureProjectExists(event.ProjectID); err != nil {
		return Event{}, err
	}
	if strings.TrimSpace(event.BranchID) == "" {
		event.BranchID = defaultBranchID
	}
	if err := s.ensureBranchExists(event.ProjectID, event.BranchID); err != nil {
		return Event{}, err
	}
	now := time.Now().UTC()
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = newID("evt")
	}
	if strings.TrimSpace(event.LogicalTS) == "" {
		event.LogicalTS = fmt.Sprintf("%020d", event.Timestamp.UnixNano())
	}
	if strings.TrimSpace(event.Type) == "" {
		return Event{}, errors.New("event type is required")
	}
	if err := s.putJSON(ctx, s.eventKey(event.ProjectID, event.BranchID, event.LogicalTS, event.EventID), event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *T4Store) GetState(_ context.Context, key StateKey) (StateEntry, bool, error) {
	if err := validateStateKey(key); err != nil {
		return StateEntry{}, false, err
	}
	return getJSON[StateEntry](s.node, s.stateKey(key))
}

func (s *T4Store) SetState(ctx context.Context, key StateKey, value any) (StateEntry, error) {
	if err := validateStateKey(key); err != nil {
		return StateEntry{}, err
	}
	if err := s.ensureProjectExists(key.ProjectID); err != nil {
		return StateEntry{}, err
	}
	entry := StateEntry{
		StateKey:  normalizeStateKey(key),
		Value:     value,
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.putJSON(ctx, s.stateKey(entry.StateKey), entry); err != nil {
		return StateEntry{}, err
	}
	return entry, nil
}

func (s *T4Store) Facts(_ context.Context, q FactsQuery) ([]Fact, error) {
	if strings.TrimSpace(q.ProjectID) == "" {
		return nil, errors.New("project_id is required")
	}
	kvs, err := s.node.List(s.factsPrefix(q.ProjectID))
	if err != nil {
		return nil, err
	}
	facts := make([]Fact, 0, len(kvs))
	for _, kv := range kvs {
		var fact Fact
		if err := json.Unmarshal(kv.Value, &fact); err != nil {
			return nil, err
		}
		if matchFact(q, fact) {
			facts = append(facts, fact)
		}
	}
	slices.SortFunc(facts, func(a, b Fact) int {
		switch {
		case a.FactID < b.FactID:
			return -1
		case a.FactID > b.FactID:
			return 1
		case a.UpdatedAt.Before(b.UpdatedAt):
			return -1
		case a.UpdatedAt.After(b.UpdatedAt):
			return 1
		default:
			return 0
		}
	})
	if q.AfterFactID != "" {
		idx := slices.IndexFunc(facts, func(f Fact) bool { return f.FactID == q.AfterFactID })
		if idx >= 0 {
			facts = facts[idx+1:]
		}
	}
	if q.Limit > 0 && len(facts) > q.Limit {
		facts = facts[:q.Limit]
	}
	return facts, nil
}

func (s *T4Store) PromoteFact(ctx context.Context, fact Fact) (Fact, error) {
	if strings.TrimSpace(fact.ProjectID) == "" {
		return Fact{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(fact.Scope) == "" || strings.TrimSpace(fact.ScopeID) == "" {
		return Fact{}, errors.New("fact scope and scope_id are required")
	}
	if strings.TrimSpace(fact.Subject) == "" || strings.TrimSpace(fact.Predicate) == "" {
		return Fact{}, errors.New("fact subject and predicate are required")
	}
	if err := s.ensureProjectExists(fact.ProjectID); err != nil {
		return Fact{}, err
	}
	if strings.TrimSpace(fact.FactID) == "" {
		fact.FactID = newID("fact")
	}
	fact.UpdatedAt = time.Now().UTC()
	if err := s.putJSON(ctx, s.factKey(fact.ProjectID, fact.Scope, fact.ScopeID, fact.Subject, fact.Predicate, fact.FactID), fact); err != nil {
		return Fact{}, err
	}
	return fact, nil
}

func (s *T4Store) Events(_ context.Context, q EventQuery) ([]Event, error) {
	if strings.TrimSpace(q.ProjectID) == "" {
		return nil, errors.New("project_id is required")
	}
	prefixes, err := s.eventPrefixes(q.ProjectID, q.BranchID)
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0)
	for _, prefix := range prefixes {
		kvs, err := s.node.List(prefix)
		if err != nil {
			return nil, err
		}
		for _, kv := range kvs {
			var event Event
			if err := json.Unmarshal(kv.Value, &event); err != nil {
				return nil, err
			}
			if matchEvent(q, event) {
				events = append(events, event)
			}
		}
	}
	slices.SortFunc(events, func(a, b Event) int {
		switch {
		case a.LogicalTS < b.LogicalTS:
			return -1
		case a.LogicalTS > b.LogicalTS:
			return 1
		case a.Timestamp.Before(b.Timestamp):
			return -1
		case a.Timestamp.After(b.Timestamp):
			return 1
		case a.EventID < b.EventID:
			return -1
		case a.EventID > b.EventID:
			return 1
		default:
			return 0
		}
	})
	if q.Limit > 0 && len(events) > q.Limit {
		events = events[:q.Limit]
	}
	return events, nil
}

func (s *T4Store) Checkpoint(ctx context.Context, projectID, branchID string, metadata map[string]any) (Checkpoint, error) {
	if err := s.ensureBranchExists(projectID, branchID); err != nil {
		return Checkpoint{}, err
	}
	latestLogicalTS, err := s.latestBranchLogicalTS(projectID, branchID)
	if err != nil {
		return Checkpoint{}, err
	}
	checkpoint := Checkpoint{
		CheckpointID: newID("chk"),
		ProjectID:    projectID,
		BranchID:     branchID,
		LogicalTS:    latestLogicalTS,
		CreatedAt:    time.Now().UTC(),
		Metadata:     metadata,
	}
	if err := s.putJSON(ctx, s.checkpointKey(projectID, branchID, checkpoint.CheckpointID), checkpoint); err != nil {
		return Checkpoint{}, err
	}
	return checkpoint, nil
}

func (s *T4Store) Branch(ctx context.Context, from BranchFrom) (Branch, error) {
	if strings.TrimSpace(from.ProjectID) == "" || strings.TrimSpace(from.From) == "" || strings.TrimSpace(from.Name) == "" {
		return Branch{}, errors.New("project_id, from, and name are required")
	}
	if err := s.ensureBranchExists(from.ProjectID, from.From); err != nil {
		return Branch{}, err
	}
	branchID := sanitizePathSegment(from.Name)
	if branchID == "" {
		branchID = newID("branch")
	}
	if _, ok, err := getJSON[Branch](s.node, s.branchKey(from.ProjectID, branchID)); err != nil {
		return Branch{}, err
	} else if ok {
		return Branch{}, fmt.Errorf("branch %q already exists", branchID)
	}
	baseLogicalTS, err := s.resolveBaseLogicalTS(from.ProjectID, from.From, from.FromCheckpointID)
	if err != nil {
		return Branch{}, err
	}
	branch := Branch{
		ProjectID:          from.ProjectID,
		BranchID:           branchID,
		Name:               from.Name,
		ParentBranchID:     from.From,
		BaseCheckpointID:   from.FromCheckpointID,
		BaseEventLogicalTS: baseLogicalTS,
		CreatedAt:          time.Now().UTC(),
	}
	if err := s.putJSON(ctx, s.branchKey(from.ProjectID, branchID), branch); err != nil {
		return Branch{}, err
	}
	return branch, nil
}

func (s *T4Store) CompareBranches(ctx context.Context, projectID, leftBranchID, rightBranchID string) (BranchComparison, error) {
	leftEvents, err := s.eventsForBranch(ctx, projectID, leftBranchID)
	if err != nil {
		return BranchComparison{}, err
	}
	rightEvents, err := s.eventsForBranch(ctx, projectID, rightBranchID)
	if err != nil {
		return BranchComparison{}, err
	}
	leftFacts, err := s.Facts(ctx, FactsQuery{ProjectID: projectID, BranchID: leftBranchID})
	if err != nil {
		return BranchComparison{}, err
	}
	rightFacts, err := s.Facts(ctx, FactsQuery{ProjectID: projectID, BranchID: rightBranchID})
	if err != nil {
		return BranchComparison{}, err
	}
	return BranchComparison{
		ProjectID:        projectID,
		LeftBranchID:     leftBranchID,
		RightBranchID:    rightBranchID,
		LeftOnlyEvents:   diffEvents(leftEvents, rightEvents),
		RightOnlyEvents:  diffEvents(rightEvents, leftEvents),
		LeftOnlyFacts:    diffFacts(leftFacts, rightFacts),
		RightOnlyFacts:   diffFacts(rightFacts, leftFacts),
		CommonBaseBranch: commonBase(leftBranchID, rightBranchID),
	}, nil
}

func (s *T4Store) AdoptBranch(ctx context.Context, projectID, branchID string) (Branch, error) {
	project, ok, err := getJSON[Project](s.node, s.projectKey(projectID))
	if err != nil {
		return Branch{}, err
	}
	if !ok {
		return Branch{}, fmt.Errorf("project %q does not exist", projectID)
	}
	branch, ok, err := getJSON[Branch](s.node, s.branchKey(projectID, branchID))
	if err != nil {
		return Branch{}, err
	}
	if !ok {
		return Branch{}, fmt.Errorf("branch %q does not exist", branchID)
	}
	now := time.Now().UTC()
	branch.AdoptedAt = &now
	project.DefaultBranchID = branchID
	project.UpdatedAt = now
	if err := s.putJSON(ctx, s.branchKey(projectID, branchID), branch); err != nil {
		return Branch{}, err
	}
	if err := s.putJSON(ctx, s.projectKey(projectID), project); err != nil {
		return Branch{}, err
	}
	return branch, nil
}

func (s *T4Store) GetBranch(_ context.Context, projectID, branchID string) (Branch, error) {
	branch, ok, err := getJSON[Branch](s.node, s.branchKey(projectID, branchID))
	if err != nil {
		return Branch{}, err
	}
	if !ok {
		return Branch{}, fmt.Errorf("branch %q does not exist", branchID)
	}
	return branch, nil
}

func (s *T4Store) eventsForBranch(ctx context.Context, projectID, branchID string) ([]Event, error) {
	return s.Events(ctx, EventQuery{ProjectID: projectID, BranchID: branchID})
}

func (s *T4Store) resolveBaseLogicalTS(projectID, branchID, checkpointID string) (string, error) {
	if checkpointID == "" {
		return s.latestBranchLogicalTS(projectID, branchID)
	}
	checkpoint, ok, err := getJSON[Checkpoint](s.node, s.checkpointKey(projectID, branchID, checkpointID))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("checkpoint %q does not exist", checkpointID)
	}
	return checkpoint.LogicalTS, nil
}

func (s *T4Store) latestBranchLogicalTS(projectID, branchID string) (string, error) {
	kvs, err := s.node.List(s.eventPrefix(projectID, branchID))
	if err != nil {
		return "", err
	}
	if len(kvs) == 0 {
		return "", nil
	}
	lastKey := kvs[len(kvs)-1].Key
	parts := strings.Split(lastKey, "/")
	if len(parts) < 2 {
		return "", nil
	}
	return parts[len(parts)-2], nil
}

func (s *T4Store) ensureProjectExists(projectID string) error {
	if strings.TrimSpace(projectID) == "" {
		return errors.New("project_id is required")
	}
	if _, ok, err := getJSON[Project](s.node, s.projectKey(projectID)); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("project %q does not exist", projectID)
	}
	return nil
}

func (s *T4Store) ensureBranchExists(projectID, branchID string) error {
	if err := s.ensureProjectExists(projectID); err != nil {
		return err
	}
	if strings.TrimSpace(branchID) == "" {
		return errors.New("branch_id is required")
	}
	if _, ok, err := getJSON[Branch](s.node, s.branchKey(projectID, branchID)); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("branch %q does not exist", branchID)
	}
	return nil
}

func (s *T4Store) eventPrefixes(projectID, branchID string) ([]string, error) {
	if strings.TrimSpace(branchID) != "" {
		return []string{s.eventPrefix(projectID, branchID)}, nil
	}
	branches, err := s.node.List(s.branchesPrefix(projectID))
	if err != nil {
		return nil, err
	}
	prefixes := make([]string, 0, len(branches))
	for _, kv := range branches {
		var branch Branch
		if err := json.Unmarshal(kv.Value, &branch); err != nil {
			return nil, err
		}
		prefixes = append(prefixes, s.eventPrefix(projectID, branch.BranchID))
	}
	return prefixes, nil
}

func (s *T4Store) putJSON(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.node.Put(ctx, key, data, 0)
	return err
}

func getJSON[T any](node *t4.Node, key string) (T, bool, error) {
	var zero T
	kv, err := node.Get(key)
	if err != nil {
		return zero, false, err
	}
	if kv == nil {
		return zero, false, nil
	}
	var value T
	if err := json.Unmarshal(kv.Value, &value); err != nil {
		return zero, false, err
	}
	return value, true, nil
}

func (s *T4Store) projectKey(projectID string) string {
	return fmt.Sprintf("/projects/%s/meta", sanitizePathSegment(projectID))
}

func (s *T4Store) branchesPrefix(projectID string) string {
	return fmt.Sprintf("/projects/%s/branches/", sanitizePathSegment(projectID))
}

func (s *T4Store) branchKey(projectID, branchID string) string {
	return fmt.Sprintf("%s%s/meta", s.branchesPrefix(projectID), sanitizePathSegment(branchID))
}

func (s *T4Store) eventPrefix(projectID, branchID string) string {
	return fmt.Sprintf("/projects/%s/events/%s/", sanitizePathSegment(projectID), sanitizePathSegment(branchID))
}

func (s *T4Store) eventKey(projectID, branchID, logicalTS, eventID string) string {
	return fmt.Sprintf("%s%s/%s", s.eventPrefix(projectID, branchID), sanitizePathSegment(logicalTS), sanitizePathSegment(eventID))
}

func (s *T4Store) stateKey(key StateKey) string {
	key = normalizeStateKey(key)
	return fmt.Sprintf("/projects/%s/state/%s/%s/%s/%s", key.ProjectID, key.Scope, key.ScopeID, fallback(key.BranchID, "_"), key.Field)
}

func (s *T4Store) factsPrefix(projectID string) string {
	return fmt.Sprintf("/projects/%s/facts/", sanitizePathSegment(projectID))
}

func (s *T4Store) factKey(projectID, scope, scopeID, subject, predicate, factID string) string {
	return fmt.Sprintf(
		"/projects/%s/facts/%s/%s/%s/%s/%s",
		sanitizePathSegment(projectID),
		sanitizePathSegment(scope),
		sanitizePathSegment(scopeID),
		sanitizePathSegment(subject),
		sanitizePathSegment(predicate),
		sanitizePathSegment(factID),
	)
}

func (s *T4Store) checkpointKey(projectID, branchID, checkpointID string) string {
	return fmt.Sprintf(
		"/projects/%s/checkpoints/%s/%s",
		sanitizePathSegment(projectID),
		sanitizePathSegment(branchID),
		sanitizePathSegment(checkpointID),
	)
}

func validateStateKey(key StateKey) error {
	if strings.TrimSpace(key.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	if strings.TrimSpace(key.Scope) == "" || strings.TrimSpace(key.ScopeID) == "" || strings.TrimSpace(key.Field) == "" {
		return errors.New("state scope, scope_id, and field are required")
	}
	return nil
}

func normalizeStateKey(key StateKey) StateKey {
	key.Scope = sanitizePathSegment(key.Scope)
	key.ScopeID = sanitizePathSegment(key.ScopeID)
	key.Field = sanitizePathSegment(key.Field)
	key.ProjectID = sanitizePathSegment(key.ProjectID)
	key.BranchID = sanitizePathSegment(key.BranchID)
	return key
}

func matchFact(q FactsQuery, fact Fact) bool {
	if q.Scope != "" && q.Scope != fact.Scope {
		return false
	}
	if q.ScopeID != "" && q.ScopeID != fact.ScopeID {
		return false
	}
	if q.Subject != "" && q.Subject != fact.Subject {
		return false
	}
	if q.Predicate != "" && q.Predicate != fact.Predicate {
		return false
	}
	if q.BranchID != "" && q.BranchID != fact.SourceBranch {
		return false
	}
	return true
}

func matchEvent(q EventQuery, event Event) bool {
	if q.TaskID != "" && q.TaskID != event.TaskID {
		return false
	}
	if q.SessionID != "" && q.SessionID != event.SessionID {
		return false
	}
	if q.Type != "" && q.Type != event.Type {
		return false
	}
	if q.AfterLogicalTS != "" && event.LogicalTS <= q.AfterLogicalTS {
		return false
	}
	if !q.Since.IsZero() && event.Timestamp.Before(q.Since) {
		return false
	}
	if !q.Until.IsZero() && event.Timestamp.After(q.Until) {
		return false
	}
	return true
}

func diffEvents(left, right []Event) []Event {
	rightByID := make(map[string]struct{}, len(right))
	for _, item := range right {
		rightByID[item.EventID] = struct{}{}
	}
	diff := make([]Event, 0)
	for _, item := range left {
		if _, ok := rightByID[item.EventID]; !ok {
			diff = append(diff, item)
		}
	}
	return diff
}

func diffFacts(left, right []Fact) []Fact {
	rightByID := make(map[string]struct{}, len(right))
	for _, item := range right {
		rightByID[item.FactID] = struct{}{}
	}
	diff := make([]Fact, 0)
	for _, item := range left {
		if _, ok := rightByID[item.FactID]; !ok {
			diff = append(diff, item)
		}
	}
	return diff
}

func commonBase(leftBranchID, rightBranchID string) string {
	if leftBranchID == rightBranchID {
		return leftBranchID
	}
	return ""
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func sanitizePathSegment(value string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", "..", "_")
	return replacer.Replace(strings.TrimSpace(value))
}

func newID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}
