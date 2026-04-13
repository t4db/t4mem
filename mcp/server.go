package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/t4db/t4mem/memory"
)

type Server struct {
	store memory.Store
	in    *bufio.Reader
	out   io.Writer
}

const (
	protocolVersionLatest  = "2025-06-18"
	protocolVersionCurrent = "2025-03-26"
	protocolVersionLegacy  = "2024-11-05"
)

func New(store memory.Store, in io.Reader, out io.Writer) *Server {
	return &Server{
		store: store,
		in:    bufio.NewReader(in),
		out:   out,
	}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string     `json:"jsonrpc"`
	ID      any        `json:"id,omitempty"`
	Result  any        `json:"result,omitempty"`
	Error   *respError `json:"error,omitempty"`
}

type respError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type recordCommandInput struct {
	ProjectID     string         `json:"project_id"`
	Command       string         `json:"command"`
	BranchID      string         `json:"branch_id"`
	SessionID     string         `json:"session_id"`
	TaskID        string         `json:"task_id"`
	ExitCode      *int           `json:"exit_code"`
	DurationMS    *int           `json:"duration_ms"`
	StdoutSummary string         `json:"stdout_summary"`
	StderrSummary string         `json:"stderr_summary"`
	CausationID   string         `json:"causation_id"`
	CorrelationID string         `json:"correlation_id"`
	Metadata      map[string]any `json:"metadata"`
}

type recordObservationInput struct {
	ProjectID     string         `json:"project_id"`
	Summary       string         `json:"summary"`
	BranchID      string         `json:"branch_id"`
	SessionID     string         `json:"session_id"`
	TaskID        string         `json:"task_id"`
	Kind          string         `json:"kind"`
	Confidence    *float64       `json:"confidence"`
	Evidence      []string       `json:"evidence"`
	RelatedFiles  []string       `json:"related_files"`
	CausationID   string         `json:"causation_id"`
	CorrelationID string         `json:"correlation_id"`
	Metadata      map[string]any `json:"metadata"`
}

type recordDecisionInput struct {
	ProjectID       string         `json:"project_id"`
	Summary         string         `json:"summary"`
	BranchID        string         `json:"branch_id"`
	SessionID       string         `json:"session_id"`
	TaskID          string         `json:"task_id"`
	Rationale       string         `json:"rationale"`
	Alternatives    []string       `json:"alternatives"`
	ExpectedOutcome string         `json:"expected_outcome"`
	Confidence      *float64       `json:"confidence"`
	CausationID     string         `json:"causation_id"`
	CorrelationID   string         `json:"correlation_id"`
	Metadata        map[string]any `json:"metadata"`
}

type recordPlanInput struct {
	ProjectID     string         `json:"project_id"`
	Summary       string         `json:"summary"`
	Steps         []string       `json:"steps"`
	Status        string         `json:"status"`
	BranchID      string         `json:"branch_id"`
	SessionID     string         `json:"session_id"`
	TaskID        string         `json:"task_id"`
	CausationID   string         `json:"causation_id"`
	CorrelationID string         `json:"correlation_id"`
	Metadata      map[string]any `json:"metadata"`
}

type traceDecisionInput struct {
	ProjectID       string `json:"project_id"`
	DecisionEventID string `json:"decision_event_id"`
	BranchID        string `json:"branch_id"`
	SessionID       string `json:"session_id"`
	TaskID          string `json:"task_id"`
	Limit           int    `json:"limit"`
}

type promoteFactFromEventsInput struct {
	ProjectID        string   `json:"project_id"`
	Scope            string   `json:"scope"`
	ScopeID          string   `json:"scope_id"`
	Subject          string   `json:"subject"`
	Predicate        string   `json:"predicate"`
	Value            any      `json:"value"`
	Confidence       float64  `json:"confidence"`
	EvidenceEventIDs []string `json:"evidence_event_ids"`
	SourceBranch     string   `json:"source_branch"`
}

type adoptBranchWithReasonInput struct {
	ProjectID     string         `json:"project_id"`
	BranchID      string         `json:"branch_id"`
	Reason        string         `json:"reason"`
	Summary       string         `json:"summary"`
	SessionID     string         `json:"session_id"`
	TaskID        string         `json:"task_id"`
	CorrelationID string         `json:"correlation_id"`
	CausationID   string         `json:"causation_id"`
	Metadata      map[string]any `json:"metadata"`
}

type taskSnapshotInput struct {
	ProjectID   string   `json:"project_id"`
	TaskID      string   `json:"task_id"`
	SessionID   string   `json:"session_id"`
	BranchID    string   `json:"branch_id"`
	StateFields []string `json:"state_fields"`
	Limit       int      `json:"limit"`
}

type sessionSnapshotInput struct {
	ProjectID   string   `json:"project_id"`
	SessionID   string   `json:"session_id"`
	BranchID    string   `json:"branch_id"`
	StateFields []string `json:"state_fields"`
	Limit       int      `json:"limit"`
}

func (s *Server) Serve(ctx context.Context) error {
	for {
		line, err := s.in.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		line = bytesTrimSpace(line)
		if len(line) == 0 {
			continue
		}

		responses, handled, err := s.handleMessage(ctx, line)
		if err != nil {
			if err := s.write(map[string]any{
				"jsonrpc": "2.0",
				"error": map[string]any{
					"code":    -32700,
					"message": "parse error",
				},
			}); err != nil {
				return err
			}
			continue
		}
		if !handled || len(responses) == 0 {
			continue
		}
		var payload any = responses[0]
		if len(responses) > 1 {
			payload = responses
		}
		if err := s.write(payload); err != nil {
			return err
		}
	}
}

func (s *Server) handleMessage(ctx context.Context, line []byte) ([]response, bool, error) {
	if len(line) == 0 {
		return nil, false, nil
	}
	if line[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(line, &batch); err != nil {
			return nil, false, err
		}
		responses := make([]response, 0, len(batch))
		for _, item := range batch {
			resp, ok, err := s.handleSingleMessage(ctx, item)
			if err != nil {
				return nil, false, err
			}
			if ok {
				responses = append(responses, resp)
			}
		}
		return responses, true, nil
	}

	resp, ok, err := s.handleSingleMessage(ctx, line)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, true, nil
	}
	return []response{resp}, true, nil
}

func (s *Server) handleSingleMessage(ctx context.Context, line []byte) (response, bool, error) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return response{}, false, err
	}

	if req.Method == "notifications/initialized" {
		return response{}, false, nil
	}

	resp := response{JSONRPC: "2.0", ID: req.ID}
	result, err := s.handle(ctx, req.Method, req.Params)
	if err != nil {
		resp.Error = &respError{Code: -32000, Message: err.Error()}
	} else {
		resp.Result = result
	}
	return resp, true, nil
}

func (s *Server) handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		return s.handleInitialize(params)
	case "ping":
		return map[string]any{}, nil
	case "resources/list":
		return map[string]any{
			"resources": []any{},
		}, nil
	case "resources/templates/list":
		return map[string]any{
			"resourceTemplates": []any{},
		}, nil
	case "prompts/list":
		return map[string]any{
			"prompts": []any{},
		}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		return s.handleToolCall(ctx, params)
	default:
		return nil, fmt.Errorf("unsupported method %q", method)
	}
}

func (s *Server) handleInitialize(params json.RawMessage) (any, error) {
	var input struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, fmt.Errorf("decode initialize params: %w", err)
		}
	}

	version := negotiateProtocolVersion(input.ProtocolVersion)
	if version == "" {
		return nil, fmt.Errorf("unsupported protocol version %q", input.ProtocolVersion)
	}

	return map[string]any{
		"protocolVersion": version,
		"serverInfo": map[string]any{
			"name":    "t4mem",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"prompts": map[string]any{
				"listChanged": false,
			},
			"resources": map[string]any{
				"subscribe":   false,
				"listChanged": false,
			},
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"instructions": "Use the memory.* tools to open projects, append events, and query timelines.",
	}, nil
}

func negotiateProtocolVersion(requested string) string {
	switch strings.TrimSpace(requested) {
	case "", protocolVersionLatest:
		return protocolVersionLatest
	case protocolVersionCurrent:
		return protocolVersionCurrent
	case protocolVersionLegacy:
		return protocolVersionLegacy
	default:
		return ""
	}
}

func (s *Server) handleToolCall(ctx context.Context, raw json.RawMessage) (any, error) {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return nil, err
	}

	var result any
	var err error
	switch call.Name {
	case "memory.open_project":
		var input memory.OpenProjectInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.store.OpenProject(ctx, input)
		}
	case "memory.append_event":
		var input memory.Event
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.store.Append(ctx, input)
		}
	case "memory.record_command":
		var input recordCommandInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			var event memory.Event
			event, err = buildCommandEvent(input)
			if err == nil {
				result, err = s.store.Append(ctx, event)
			}
		}
	case "memory.record_observation":
		var input recordObservationInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			var event memory.Event
			event, err = buildObservationEvent(input)
			if err == nil {
				result, err = s.store.Append(ctx, event)
			}
		}
	case "memory.record_decision":
		var input recordDecisionInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			var event memory.Event
			event, err = buildDecisionEvent(input)
			if err == nil {
				result, err = s.store.Append(ctx, event)
			}
		}
	case "memory.trace_decision":
		var input traceDecisionInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.traceDecision(ctx, input)
		}
	case "memory.record_plan":
		var input recordPlanInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			var event memory.Event
			event, err = buildPlanEvent("plan.created", input)
			if err == nil {
				result, err = s.store.Append(ctx, event)
			}
		}
	case "memory.update_plan":
		var input recordPlanInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			var event memory.Event
			event, err = buildPlanEvent("plan.updated", input)
			if err == nil {
				result, err = s.store.Append(ctx, event)
			}
		}
	case "memory.get_state":
		var key memory.StateKey
		err = decodeArgs(call.Arguments, &key)
		if err == nil {
			var entry memory.StateEntry
			var ok bool
			entry, ok, err = s.store.GetState(ctx, key)
			result = map[string]any{"found": ok, "entry": entry}
		}
	case "memory.set_state":
		var input struct {
			Key   memory.StateKey `json:"key"`
			Value any             `json:"value"`
		}
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.store.SetState(ctx, input.Key, input.Value)
		}
	case "memory.list_facts":
		var q memory.FactsQuery
		err = decodeArgs(call.Arguments, &q)
		if err == nil {
			query := q
			if query.Limit > 0 {
				query.Limit++
			}
			var facts []memory.Fact
			facts, err = s.store.Facts(ctx, query)
			if err == nil {
				result = buildFactsPage(facts, q.Limit)
			}
		}
	case "memory.promote_fact":
		var fact memory.Fact
		err = decodeArgs(call.Arguments, &fact)
		if err == nil {
			result, err = s.store.PromoteFact(ctx, fact)
		}
	case "memory.promote_fact_from_events":
		var input promoteFactFromEventsInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.promoteFactFromEvents(ctx, input)
		}
	case "memory.branch_create":
		var from memory.BranchFrom
		err = decodeArgs(call.Arguments, &from)
		if err == nil {
			result, err = s.store.Branch(ctx, from)
		}
	case "memory.branch_summary":
		var input struct {
			ProjectID string `json:"project_id"`
			BranchID  string `json:"branch_id"`
			Limit     int    `json:"limit"`
		}
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.branchSummary(ctx, input.ProjectID, input.BranchID, input.Limit)
		}
	case "memory.branch_compare":
		var input struct {
			ProjectID     string `json:"project_id"`
			LeftBranchID  string `json:"left_branch_id"`
			RightBranchID string `json:"right_branch_id"`
		}
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.store.CompareBranches(ctx, input.ProjectID, input.LeftBranchID, input.RightBranchID)
		}
	case "memory.branch_diff_summary":
		var input struct {
			ProjectID     string `json:"project_id"`
			LeftBranchID  string `json:"left_branch_id"`
			RightBranchID string `json:"right_branch_id"`
			Limit         int    `json:"limit"`
		}
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.branchDiffSummary(ctx, input.ProjectID, input.LeftBranchID, input.RightBranchID, input.Limit)
		}
	case "memory.branch_adopt":
		var input struct {
			ProjectID string `json:"project_id"`
			BranchID  string `json:"branch_id"`
		}
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.store.AdoptBranch(ctx, input.ProjectID, input.BranchID)
		}
	case "memory.adopt_branch_with_reason":
		var input adoptBranchWithReasonInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.adoptBranchWithReason(ctx, input)
		}
	case "memory.timeline_query":
		var q memory.EventQuery
		err = decodeArgs(call.Arguments, &q)
		if err == nil {
			query := q
			if query.Limit > 0 {
				query.Limit++
			}
			var events []memory.Event
			events, err = s.store.Events(ctx, query)
			if err == nil {
				result = buildTimelinePage(events, q.Limit)
			}
		}
	case "memory.recent_context":
		var q memory.EventQuery
		err = decodeArgs(call.Arguments, &q)
		if err == nil {
			limit := q.Limit
			if limit <= 0 {
				limit = 10
			}
			q.Limit = 0
			var events []memory.Event
			events, err = s.store.Events(ctx, q)
			if err == nil {
				result = buildRecentContext(events, limit)
			}
		}
	case "memory.task_snapshot":
		var input taskSnapshotInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.taskSnapshot(ctx, input)
		}
	case "memory.session_snapshot":
		var input sessionSnapshotInput
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.sessionSnapshot(ctx, input)
		}
	case "memory.project_snapshot":
		var input struct {
			ProjectID string `json:"project_id"`
			BranchID  string `json:"branch_id"`
			Limit     int    `json:"limit"`
		}
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.projectSnapshot(ctx, input.ProjectID, input.BranchID, input.Limit)
		}
	case "memory.fact_summary":
		var q memory.FactsQuery
		err = decodeArgs(call.Arguments, &q)
		if err == nil {
			limit := q.Limit
			if limit <= 0 {
				limit = 10
			}
			q.Limit = limit
			var facts []memory.Fact
			facts, err = s.store.Facts(ctx, q)
			if err == nil {
				result = buildFactSummary(facts)
			}
		}
	case "memory.checkpoint":
		var input struct {
			ProjectID string         `json:"project_id"`
			BranchID  string         `json:"branch_id"`
			Metadata  map[string]any `json:"metadata"`
		}
		err = decodeArgs(call.Arguments, &input)
		if err == nil {
			result, err = s.store.Checkpoint(ctx, input.ProjectID, input.BranchID, input.Metadata)
		}
	default:
		return nil, fmt.Errorf("unsupported tool %q", call.Name)
	}
	if err != nil {
		return nil, err
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(content),
			},
		},
		"structuredContent": result,
	}, nil
}

func buildTimelinePage(events []memory.Event, limit int) map[string]any {
	page := events
	nextCursor := ""
	if limit > 0 && len(events) > limit {
		page = events[:limit]
		nextCursor = page[len(page)-1].LogicalTS
	}
	return map[string]any{
		"events":      page,
		"next_cursor": nextCursor,
	}
}

func buildRecentContext(events []memory.Event, limit int) map[string]any {
	if limit > 0 && len(events) > limit {
		events = events[len(events)-limit:]
	}
	recent := make([]memory.Event, len(events))
	copy(recent, events)
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}

	types := make([]string, 0)
	typeCounts := make(map[string]int)
	for _, event := range recent {
		if _, ok := typeCounts[event.Type]; !ok {
			types = append(types, event.Type)
		}
		typeCounts[event.Type]++
	}

	summary := map[string]any{
		"event_count": len(recent),
		"event_types": types,
		"type_counts": typeCounts,
	}
	if len(recent) > 0 {
		summary["latest_timestamp"] = recent[0].Timestamp
		summary["oldest_timestamp"] = recent[len(recent)-1].Timestamp
	}

	return map[string]any{
		"summary": summary,
		"events":  recent,
	}
}

func buildFactsPage(facts []memory.Fact, limit int) map[string]any {
	page := facts
	nextCursor := ""
	if limit > 0 && len(facts) > limit {
		page = facts[:limit]
		nextCursor = page[len(page)-1].FactID
	}
	return map[string]any{
		"facts":       page,
		"next_cursor": nextCursor,
	}
}

func buildFactSummary(facts []memory.Fact) map[string]any {
	bySubject := make(map[string]int)
	byPredicate := make(map[string]int)
	subjects := make([]string, 0)
	predicates := make([]string, 0)

	for _, fact := range facts {
		if _, ok := bySubject[fact.Subject]; !ok {
			subjects = append(subjects, fact.Subject)
		}
		bySubject[fact.Subject]++

		if _, ok := byPredicate[fact.Predicate]; !ok {
			predicates = append(predicates, fact.Predicate)
		}
		byPredicate[fact.Predicate]++
	}

	summary := map[string]any{
		"fact_count":   len(facts),
		"subjects":     subjects,
		"predicates":   predicates,
		"by_subject":   bySubject,
		"by_predicate": byPredicate,
	}

	return map[string]any{
		"summary": summary,
		"facts":   facts,
	}
}

func (s *Server) traceDecision(ctx context.Context, input traceDecisionInput) (map[string]any, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return nil, errors.New("project_id is required")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	decisionCandidates, err := s.store.Events(ctx, memory.EventQuery{
		ProjectID: input.ProjectID,
		BranchID:  input.BranchID,
		SessionID: input.SessionID,
		TaskID:    input.TaskID,
		Type:      "decision.made",
	})
	if err != nil {
		return nil, err
	}
	if len(decisionCandidates) == 0 {
		return nil, errors.New("no matching decision.made events found")
	}

	decision := decisionCandidates[len(decisionCandidates)-1]
	if strings.TrimSpace(input.DecisionEventID) != "" {
		found := false
		for _, event := range decisionCandidates {
			if event.EventID == input.DecisionEventID {
				decision = event
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("decision event %q not found", input.DecisionEventID)
		}
	}

	scope := memory.EventQuery{
		ProjectID: input.ProjectID,
		BranchID:  decision.BranchID,
	}
	if decision.TaskID != "" {
		scope.TaskID = decision.TaskID
	} else if decision.SessionID != "" {
		scope.SessionID = decision.SessionID
	}
	contextEvents, err := s.store.Events(ctx, scope)
	if err != nil {
		return nil, err
	}
	priorEvents := make([]memory.Event, 0)
	for _, event := range contextEvents {
		if event.LogicalTS < decision.LogicalTS {
			priorEvents = append(priorEvents, event)
		}
	}
	if len(priorEvents) > limit {
		priorEvents = priorEvents[len(priorEvents)-limit:]
	}

	allProjectEvents, err := s.store.Events(ctx, memory.EventQuery{ProjectID: input.ProjectID})
	if err != nil {
		return nil, err
	}
	eventByID := make(map[string]memory.Event, len(allProjectEvents))
	for _, event := range allProjectEvents {
		eventByID[event.EventID] = event
	}
	causationChain := make([]memory.Event, 0)
	seen := map[string]struct{}{}
	currentID := decision.CausationID
	for currentID != "" {
		if _, ok := seen[currentID]; ok {
			break
		}
		seen[currentID] = struct{}{}
		event, ok := eventByID[currentID]
		if !ok {
			break
		}
		causationChain = append(causationChain, event)
		currentID = event.CausationID
	}

	facts, err := s.store.Facts(ctx, memory.FactsQuery{
		ProjectID: input.ProjectID,
		BranchID:  decision.BranchID,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"decision":          decision,
		"causation_chain":   causationChain,
		"contextual_events": priorEvents,
		"supporting_facts":  facts,
		"summary": map[string]any{
			"decision_event_id":      decision.EventID,
			"decision_branch_id":     decision.BranchID,
			"contextual_event_count": len(priorEvents),
			"causation_depth":        len(causationChain),
			"supporting_fact_count":  len(facts),
		},
	}, nil
}

func (s *Server) promoteFactFromEvents(ctx context.Context, input promoteFactFromEventsInput) (memory.Fact, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return memory.Fact{}, errors.New("project_id is required")
	}
	if len(input.EvidenceEventIDs) == 0 {
		return memory.Fact{}, errors.New("evidence_event_ids are required")
	}
	events, err := s.store.Events(ctx, memory.EventQuery{ProjectID: input.ProjectID})
	if err != nil {
		return memory.Fact{}, err
	}
	eventByID := make(map[string]memory.Event, len(events))
	for _, event := range events {
		eventByID[event.EventID] = event
	}
	sourceBranch := strings.TrimSpace(input.SourceBranch)
	for _, eventID := range input.EvidenceEventIDs {
		event, ok := eventByID[eventID]
		if !ok {
			return memory.Fact{}, fmt.Errorf("evidence event %q not found", eventID)
		}
		if sourceBranch == "" {
			sourceBranch = event.BranchID
		}
	}
	return s.store.PromoteFact(ctx, memory.Fact{
		ProjectID:    input.ProjectID,
		Scope:        input.Scope,
		ScopeID:      input.ScopeID,
		Subject:      input.Subject,
		Predicate:    input.Predicate,
		Value:        input.Value,
		Confidence:   input.Confidence,
		EvidenceRefs: input.EvidenceEventIDs,
		SourceBranch: sourceBranch,
	})
}

func (s *Server) branchSummary(ctx context.Context, projectID, branchID string, limit int) (map[string]any, error) {
	if limit <= 0 {
		limit = 5
	}
	branch, err := s.store.GetBranch(ctx, projectID, branchID)
	if err != nil {
		return nil, err
	}
	events, err := s.store.Events(ctx, memory.EventQuery{ProjectID: projectID, BranchID: branchID})
	if err != nil {
		return nil, err
	}
	facts, err := s.store.Facts(ctx, memory.FactsQuery{ProjectID: projectID, BranchID: branchID})
	if err != nil {
		return nil, err
	}
	recent := buildRecentContext(events, limit)
	return map[string]any{
		"branch": branch,
		"summary": map[string]any{
			"event_count": len(events),
			"fact_count":  len(facts),
		},
		"recent_events":        recent["events"],
		"recent_event_summary": recent["summary"],
	}, nil
}

func (s *Server) branchDiffSummary(ctx context.Context, projectID, leftBranchID, rightBranchID string, limit int) (map[string]any, error) {
	if limit <= 0 {
		limit = 5
	}
	comparison, err := s.store.CompareBranches(ctx, projectID, leftBranchID, rightBranchID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"summary": map[string]any{
			"left_only_event_count":  len(comparison.LeftOnlyEvents),
			"right_only_event_count": len(comparison.RightOnlyEvents),
			"left_only_fact_count":   len(comparison.LeftOnlyFacts),
			"right_only_fact_count":  len(comparison.RightOnlyFacts),
			"common_base_branch":     comparison.CommonBaseBranch,
			"left_only_event_types":  eventTypes(comparison.LeftOnlyEvents),
			"right_only_event_types": eventTypes(comparison.RightOnlyEvents),
		},
		"left_only_events":  trimEvents(comparison.LeftOnlyEvents, limit),
		"right_only_events": trimEvents(comparison.RightOnlyEvents, limit),
		"left_only_facts":   trimFacts(comparison.LeftOnlyFacts, limit),
		"right_only_facts":  trimFacts(comparison.RightOnlyFacts, limit),
	}, nil
}

func (s *Server) adoptBranchWithReason(ctx context.Context, input adoptBranchWithReasonInput) (map[string]any, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return nil, errors.New("project_id is required")
	}
	if strings.TrimSpace(input.BranchID) == "" {
		return nil, errors.New("branch_id is required")
	}
	if strings.TrimSpace(input.Reason) == "" {
		return nil, errors.New("reason is required")
	}
	branch, err := s.store.AdoptBranch(ctx, input.ProjectID, input.BranchID)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"reason": input.Reason,
	}
	if strings.TrimSpace(input.Summary) != "" {
		payload["summary"] = strings.TrimSpace(input.Summary)
	}
	if len(input.Metadata) > 0 {
		payload["metadata"] = input.Metadata
	}
	event, err := s.store.Append(ctx, memory.Event{
		ProjectID:     input.ProjectID,
		BranchID:      input.BranchID,
		SessionID:     input.SessionID,
		TaskID:        input.TaskID,
		Type:          "branch.adopted",
		Payload:       payload,
		CausationID:   input.CausationID,
		CorrelationID: input.CorrelationID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"branch":         branch,
		"adoption_event": event,
	}, nil
}

func trimEvents(events []memory.Event, limit int) []memory.Event {
	if limit > 0 && len(events) > limit {
		return events[:limit]
	}
	return events
}

func trimFacts(facts []memory.Fact, limit int) []memory.Fact {
	if limit > 0 && len(facts) > limit {
		return facts[:limit]
	}
	return facts
}

func eventTypes(events []memory.Event) []string {
	seen := map[string]struct{}{}
	types := make([]string, 0)
	for _, event := range events {
		if _, ok := seen[event.Type]; ok {
			continue
		}
		seen[event.Type] = struct{}{}
		types = append(types, event.Type)
	}
	return types
}

func (s *Server) taskSnapshot(ctx context.Context, input taskSnapshotInput) (map[string]any, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return nil, errors.New("project_id is required")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	branchID, project, err := s.resolveSnapshotBranch(ctx, input)
	if err != nil {
		return nil, err
	}

	branchSummary, err := s.branchSummary(ctx, input.ProjectID, branchID, limit)
	if err != nil {
		return nil, err
	}

	recentEvents, err := s.store.Events(ctx, memory.EventQuery{
		ProjectID: input.ProjectID,
		BranchID:  branchID,
		TaskID:    input.TaskID,
		SessionID: input.SessionID,
	})
	if err != nil {
		return nil, err
	}
	recentContext := buildRecentContext(recentEvents, limit)

	facts, err := s.store.Facts(ctx, memory.FactsQuery{
		ProjectID: input.ProjectID,
		BranchID:  branchID,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	factSummary := buildFactSummary(facts)

	state := make(map[string]memory.StateEntry)
	for _, field := range input.StateFields {
		entry, ok, err := s.store.GetState(ctx, memory.StateKey{
			ProjectID: input.ProjectID,
			Scope:     "task",
			ScopeID:   input.TaskID,
			Field:     field,
			BranchID:  branchID,
		})
		if err != nil {
			return nil, err
		}
		if !ok {
			entry, ok, err = s.store.GetState(ctx, memory.StateKey{
				ProjectID: input.ProjectID,
				Scope:     "task",
				ScopeID:   input.TaskID,
				Field:     field,
			})
			if err != nil {
				return nil, err
			}
		}
		if ok {
			state[field] = entry
		}
	}

	var latestDecisionTrace any = nil
	if input.TaskID != "" || input.SessionID != "" {
		trace, err := s.traceDecision(ctx, traceDecisionInput{
			ProjectID: input.ProjectID,
			BranchID:  branchID,
			TaskID:    input.TaskID,
			SessionID: input.SessionID,
			Limit:     limit,
		})
		if err == nil {
			latestDecisionTrace = trace
		}
	}

	return map[string]any{
		"project":         project,
		"branch":          branchSummary["branch"],
		"branch_summary":  branchSummary["summary"],
		"recent_context":  recentContext,
		"fact_summary":    factSummary,
		"state":           state,
		"latest_decision": latestDecisionTrace,
		"summary": map[string]any{
			"project_id":         input.ProjectID,
			"task_id":            input.TaskID,
			"session_id":         input.SessionID,
			"branch_id":          branchID,
			"state_field_count":  len(state),
			"recent_event_count": recentContext["summary"].(map[string]any)["event_count"],
			"fact_count":         factSummary["summary"].(map[string]any)["fact_count"],
			"has_decision_trace": latestDecisionTrace != nil,
		},
	}, nil
}

func (s *Server) sessionSnapshot(ctx context.Context, input sessionSnapshotInput) (map[string]any, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return nil, errors.New("project_id is required")
	}
	if strings.TrimSpace(input.SessionID) == "" {
		return nil, errors.New("session_id is required")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	project, err := s.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: input.ProjectID})
	if err != nil {
		return nil, err
	}
	branchID := strings.TrimSpace(input.BranchID)
	if branchID == "" {
		branchID = project.DefaultBranchID
	}

	branchSummary, err := s.branchSummary(ctx, input.ProjectID, branchID, limit)
	if err != nil {
		return nil, err
	}
	events, err := s.store.Events(ctx, memory.EventQuery{
		ProjectID: input.ProjectID,
		BranchID:  branchID,
		SessionID: input.SessionID,
	})
	if err != nil {
		return nil, err
	}
	recentContext := buildRecentContext(events, limit)
	facts, err := s.store.Facts(ctx, memory.FactsQuery{
		ProjectID: input.ProjectID,
		BranchID:  branchID,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	factSummary := buildFactSummary(facts)

	state := make(map[string]memory.StateEntry)
	for _, field := range input.StateFields {
		entry, ok, err := s.store.GetState(ctx, memory.StateKey{
			ProjectID: input.ProjectID,
			Scope:     "session",
			ScopeID:   input.SessionID,
			Field:     field,
			BranchID:  branchID,
		})
		if err != nil {
			return nil, err
		}
		if !ok {
			entry, ok, err = s.store.GetState(ctx, memory.StateKey{
				ProjectID: input.ProjectID,
				Scope:     "session",
				ScopeID:   input.SessionID,
				Field:     field,
			})
			if err != nil {
				return nil, err
			}
		}
		if ok {
			state[field] = entry
		}
	}

	var latestDecisionTrace any
	trace, err := s.traceDecision(ctx, traceDecisionInput{
		ProjectID: input.ProjectID,
		BranchID:  branchID,
		SessionID: input.SessionID,
		Limit:     limit,
	})
	if err == nil {
		latestDecisionTrace = trace
	}

	return map[string]any{
		"project":         project,
		"branch":          branchSummary["branch"],
		"branch_summary":  branchSummary["summary"],
		"recent_context":  recentContext,
		"fact_summary":    factSummary,
		"state":           state,
		"latest_decision": latestDecisionTrace,
		"summary": map[string]any{
			"project_id":         input.ProjectID,
			"session_id":         input.SessionID,
			"branch_id":          branchID,
			"state_field_count":  len(state),
			"recent_event_count": recentContext["summary"].(map[string]any)["event_count"],
			"fact_count":         factSummary["summary"].(map[string]any)["fact_count"],
			"has_decision_trace": latestDecisionTrace != nil,
		},
	}, nil
}

func (s *Server) projectSnapshot(ctx context.Context, projectID, branchID string, limit int) (map[string]any, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, errors.New("project_id is required")
	}
	if limit <= 0 {
		limit = 10
	}
	project, err := s.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(branchID) == "" {
		branchID = project.DefaultBranchID
	}
	branchSummary, err := s.branchSummary(ctx, projectID, branchID, limit)
	if err != nil {
		return nil, err
	}
	events, err := s.store.Events(ctx, memory.EventQuery{ProjectID: projectID, Limit: 0})
	if err != nil {
		return nil, err
	}
	recentContext := buildRecentContext(events, limit)
	facts, err := s.store.Facts(ctx, memory.FactsQuery{ProjectID: projectID, Limit: limit})
	if err != nil {
		return nil, err
	}
	factSummary := buildFactSummary(facts)

	return map[string]any{
		"project":        project,
		"default_branch": branchSummary["branch"],
		"branch_summary": branchSummary["summary"],
		"recent_context": recentContext,
		"fact_summary":   factSummary,
		"summary": map[string]any{
			"project_id":         projectID,
			"default_branch_id":  branchID,
			"recent_event_count": recentContext["summary"].(map[string]any)["event_count"],
			"fact_count":         factSummary["summary"].(map[string]any)["fact_count"],
		},
	}, nil
}

func (s *Server) resolveSnapshotBranch(ctx context.Context, input taskSnapshotInput) (string, memory.Project, error) {
	project, err := s.store.OpenProject(ctx, memory.OpenProjectInput{ProjectID: input.ProjectID})
	if err != nil {
		return "", memory.Project{}, err
	}
	if strings.TrimSpace(input.BranchID) != "" {
		return input.BranchID, project, nil
	}

	if input.TaskID != "" {
		entry, ok, err := s.store.GetState(ctx, memory.StateKey{
			ProjectID: input.ProjectID,
			Scope:     "task",
			ScopeID:   input.TaskID,
			Field:     "branch_id",
		})
		if err != nil {
			return "", memory.Project{}, err
		}
		if ok {
			if branchID, ok := entry.Value.(string); ok && strings.TrimSpace(branchID) != "" {
				return branchID, project, nil
			}
		}
	}

	return project.DefaultBranchID, project, nil
}

func buildCommandEvent(input recordCommandInput) (memory.Event, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return memory.Event{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(input.Command) == "" {
		return memory.Event{}, errors.New("command is required")
	}

	payload := map[string]any{
		"command": strings.TrimSpace(input.Command),
	}
	if input.ExitCode != nil {
		payload["exit_code"] = *input.ExitCode
	}
	if input.DurationMS != nil {
		payload["duration_ms"] = *input.DurationMS
	}
	if strings.TrimSpace(input.StdoutSummary) != "" {
		payload["stdout_summary"] = strings.TrimSpace(input.StdoutSummary)
	}
	if strings.TrimSpace(input.StderrSummary) != "" {
		payload["stderr_summary"] = strings.TrimSpace(input.StderrSummary)
	}
	if len(input.Metadata) > 0 {
		payload["metadata"] = input.Metadata
	}

	return memory.Event{
		ProjectID:     input.ProjectID,
		SessionID:     input.SessionID,
		TaskID:        input.TaskID,
		BranchID:      input.BranchID,
		Type:          "command.executed",
		Payload:       payload,
		CausationID:   input.CausationID,
		CorrelationID: input.CorrelationID,
	}, nil
}

func buildObservationEvent(input recordObservationInput) (memory.Event, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return memory.Event{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(input.Summary) == "" {
		return memory.Event{}, errors.New("summary is required")
	}

	payload := map[string]any{
		"summary": strings.TrimSpace(input.Summary),
	}
	if strings.TrimSpace(input.Kind) != "" {
		payload["kind"] = strings.TrimSpace(input.Kind)
	}
	if input.Confidence != nil {
		payload["confidence"] = *input.Confidence
	}
	if len(input.Evidence) > 0 {
		payload["evidence"] = input.Evidence
	}
	if len(input.RelatedFiles) > 0 {
		payload["related_files"] = input.RelatedFiles
	}
	if len(input.Metadata) > 0 {
		payload["metadata"] = input.Metadata
	}

	return memory.Event{
		ProjectID:     input.ProjectID,
		SessionID:     input.SessionID,
		TaskID:        input.TaskID,
		BranchID:      input.BranchID,
		Type:          "observation.recorded",
		Payload:       payload,
		CausationID:   input.CausationID,
		CorrelationID: input.CorrelationID,
	}, nil
}

func buildDecisionEvent(input recordDecisionInput) (memory.Event, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return memory.Event{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(input.Summary) == "" {
		return memory.Event{}, errors.New("summary is required")
	}

	payload := map[string]any{
		"summary": strings.TrimSpace(input.Summary),
	}
	if strings.TrimSpace(input.Rationale) != "" {
		payload["rationale"] = strings.TrimSpace(input.Rationale)
	}
	if len(input.Alternatives) > 0 {
		payload["alternatives"] = input.Alternatives
	}
	if strings.TrimSpace(input.ExpectedOutcome) != "" {
		payload["expected_outcome"] = strings.TrimSpace(input.ExpectedOutcome)
	}
	if input.Confidence != nil {
		payload["confidence"] = *input.Confidence
	}
	if len(input.Metadata) > 0 {
		payload["metadata"] = input.Metadata
	}

	return memory.Event{
		ProjectID:     input.ProjectID,
		SessionID:     input.SessionID,
		TaskID:        input.TaskID,
		BranchID:      input.BranchID,
		Type:          "decision.made",
		Payload:       payload,
		CausationID:   input.CausationID,
		CorrelationID: input.CorrelationID,
	}, nil
}

func buildPlanEvent(eventType string, input recordPlanInput) (memory.Event, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return memory.Event{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(input.Summary) == "" {
		return memory.Event{}, errors.New("summary is required")
	}

	payload := map[string]any{
		"summary": strings.TrimSpace(input.Summary),
	}
	if len(input.Steps) > 0 {
		payload["steps"] = input.Steps
	}
	if strings.TrimSpace(input.Status) != "" {
		payload["status"] = strings.TrimSpace(input.Status)
	}
	if len(input.Metadata) > 0 {
		payload["metadata"] = input.Metadata
	}

	return memory.Event{
		ProjectID:     input.ProjectID,
		SessionID:     input.SessionID,
		TaskID:        input.TaskID,
		BranchID:      input.BranchID,
		Type:          eventType,
		Payload:       payload,
		CausationID:   input.CausationID,
		CorrelationID: input.CorrelationID,
	}, nil
}

func decodeArgs(input map[string]any, out any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func (s *Server) write(resp any) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.out, "%s\n", payload)
	return err
}

func bytesTrimSpace(input []byte) []byte {
	return []byte(strings.TrimSpace(string(input)))
}
