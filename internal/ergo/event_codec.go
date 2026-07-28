// Purpose: Decode supported event wire kinds into typed canonical events.
// Role: Exhaustive event-schema registry and legacy wire normalization.
// Invariants: Current and legacy kinds are disjoint; unknown kinds fail closed.
package ergo

import (
	"encoding/json"
	"fmt"
)

type Event struct {
	Type   string          `json:"type"`
	TS     string          `json:"ts"`
	Data   json.RawMessage `json:"data"`
	Source EventSource     `json:"-"`
}

type EventSource struct {
	Path             string
	Line             int
	TransactionIndex int
}

type NewTaskEvent struct {
	ID        string `json:"id"`
	UUID      string `json:"uuid"`
	EpicID    string `json:"epic_id"`
	State     string `json:"state"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type StateEvent struct {
	ID       string `json:"id"`
	NewState string `json:"state"`
	TS       string `json:"ts"`
}

type LinkEvent struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Type   string `json:"type"`
}

type ClaimEvent struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	TS      string `json:"ts"`
}

type TitleUpdateEvent struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	TS    string `json:"ts"`
}

type BodyUpdateEvent struct {
	ID   string `json:"id"`
	Body string `json:"body"`
	TS   string `json:"ts"`
}

type EpicAssignEvent struct {
	ID     string `json:"id"`
	EpicID string `json:"epic_id"`
	TS     string `json:"ts"`
}

type UnclaimEvent struct {
	ID string `json:"id"`
	TS string `json:"ts"`
}

type TombstoneEvent struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id,omitempty"`
	TS      string `json:"ts"`
}

type ResultEvent struct {
	TaskID            string `json:"task_id"`
	Summary           string `json:"summary"`
	Path              string `json:"path"`
	Sha256AtAttach    string `json:"sha256_at_attach"`
	MtimeAtAttach     string `json:"mtime_at_attach,omitempty"`
	GitCommitAtAttach string `json:"git_commit_at_attach,omitempty"`
	TS                string `json:"ts"`
}

type MessageEvent struct {
	TaskID string `json:"task_id"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	TS     string `json:"ts"`
}

const (
	eventNewTask   = "new_task"
	eventState     = "state"
	eventClaim     = "claim"
	eventUnclaim   = "unclaim"
	eventLink      = "link"
	eventUnlink    = "unlink"
	eventTitle     = "title"
	eventBody      = "body"
	eventEpic      = "epic"
	eventTombstone = "tombstone"
	eventResult    = "result"
	eventMessage   = "message"
)

var supportedEventKinds = []string{
	eventNewTask, eventState, eventClaim, eventUnclaim, eventLink, eventUnlink,
	eventTitle, eventBody, eventEpic, eventTombstone, eventResult, eventMessage,
}

var supportedLegacyEventKinds = []string{"new_epic"}

type decodedEvent struct {
	kind               string
	wireKind           string
	payload            any
	source             EventSource
	legacyExplicitEpic bool
}

type eventDecoder func(Event, string) (any, error)

var eventDecoders = map[string]eventDecoder{
	eventNewTask:   decodeEventPayload[NewTaskEvent],
	eventState:     decodeEventPayload[StateEvent],
	eventClaim:     decodeEventPayload[ClaimEvent],
	eventUnclaim:   decodeEventPayload[UnclaimEvent],
	eventLink:      decodeEventPayload[LinkEvent],
	eventUnlink:    decodeEventPayload[LinkEvent],
	eventTitle:     decodeEventPayload[TitleUpdateEvent],
	eventBody:      decodeEventPayload[BodyUpdateEvent],
	eventEpic:      decodeEventPayload[EpicAssignEvent],
	eventTombstone: decodeEventPayload[TombstoneEvent],
	eventResult:    decodeEventPayload[ResultEvent],
	eventMessage:   decodeEventPayload[MessageEvent],
}

var legacyEventDecoders = map[string]eventDecoder{
	"new_epic": decodeEventPayload[NewTaskEvent],
}

func decodeEventPayload[T any](event Event, context string) (any, error) {
	var payload T
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		return nil, replayDecodeError(context, event.Type, "", err)
	}
	return payload, nil
}

func decodeEvent(event Event, fallbackIndex int) (decodedEvent, error) {
	context := eventReplayContext(event, fallbackIndex)
	if event.Type == "" {
		return decodedEvent{}, replayInvariantError(context, "", "", "event kind is empty")
	}
	if _, err := parseTime(event.TS); err != nil {
		return decodedEvent{}, replayDecodeError(context, event.Type, "", fmt.Errorf("invalid event ts: %w", err))
	}
	if decoder := eventDecoders[event.Type]; decoder != nil {
		payload, err := decoder(event, context)
		return decodedEvent{kind: event.Type, wireKind: event.Type, payload: payload, source: event.Source}, err
	}
	if decoder := legacyEventDecoders[event.Type]; decoder != nil {
		payload, err := decoder(event, context)
		return decodedEvent{
			kind: eventNewTask, wireKind: event.Type, payload: payload, source: event.Source,
			legacyExplicitEpic: true,
		}, err
	}
	return decodedEvent{}, replayInvariantError(context, event.Type, "", "unknown event kind")
}
