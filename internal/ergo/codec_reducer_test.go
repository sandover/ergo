// Purpose: Prove event-registry exhaustiveness and pure transaction reduction.
package ergo

import (
	"reflect"
	"testing"
	"time"
)

func TestSupportedEventKindRegistryIsExhaustive(t *testing.T) {
	current := make(map[string]struct{}, len(supportedEventKinds))
	for _, kind := range supportedEventKinds {
		if _, duplicate := current[kind]; duplicate {
			t.Fatalf("duplicate current event kind %q", kind)
		}
		current[kind] = struct{}{}
		if eventDecoders[kind] == nil {
			t.Fatalf("current event kind %q has no decoder", kind)
		}
	}
	if len(current) != len(eventDecoders) {
		t.Fatalf("registry lists %d current kinds but has %d decoders", len(current), len(eventDecoders))
	}
	for _, kind := range supportedLegacyEventKinds {
		if _, conflict := current[kind]; conflict {
			t.Fatalf("legacy event kind %q also appears as current", kind)
		}
		if legacyEventDecoders[kind] == nil {
			t.Fatalf("legacy event kind %q has no decoder", kind)
		}
	}
	if len(supportedLegacyEventKinds) != len(legacyEventDecoders) {
		t.Fatalf("legacy registry/list cardinality differs")
	}
}

func TestLegacyNewEpicNormalizesBeforeReduction(t *testing.T) {
	now := time.Now().UTC()
	event := mustNewEvent("new_epic", now, NewTaskEvent{
		ID: "EPIC01", UUID: "legacy", State: stateTodo, Title: "Legacy", CreatedAt: formatTime(now),
	})
	decoded, err := decodeEvent(event, 0)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.kind != eventNewTask || !decoded.legacyExplicitEpic {
		t.Fatalf("legacy normalization = %#v", decoded)
	}
	graph, err := replayEvents([]Event{event})
	if err != nil {
		t.Fatal(err)
	}
	if !graph.IsEpic("EPIC01") || len(graph.Children("EPIC01")) != 0 {
		t.Fatal("normalized legacy empty epic was not preserved")
	}
}

func TestEverySupportedEventKindDecodesAndReduces(t *testing.T) {
	now := time.Now().UTC()
	create := func(id string) Event {
		return mustNewEvent(eventNewTask, now, NewTaskEvent{
			ID: id, UUID: "uuid-" + id, State: stateTodo, Title: id, CreatedAt: formatTime(now),
		})
	}
	tests := []struct {
		kind   string
		base   []Event
		events []Event
	}{
		{eventNewTask, nil, []Event{create("T1")}},
		{eventState, []Event{create("T1")}, []Event{mustNewEvent(eventState, now, StateEvent{ID: "T1", NewState: stateDone, TS: formatTime(now)})}},
		{eventClaim, []Event{create("T1")}, []Event{
			mustNewEvent(eventClaim, now, ClaimEvent{ID: "T1", AgentID: "agent", TS: formatTime(now)}),
			mustNewEvent(eventState, now, StateEvent{ID: "T1", NewState: stateDoing, TS: formatTime(now)}),
		}},
		{eventUnclaim, []Event{
			create("T1"),
			mustNewEvent(eventClaim, now, ClaimEvent{ID: "T1", AgentID: "agent", TS: formatTime(now)}),
			mustNewEvent(eventState, now, StateEvent{ID: "T1", NewState: stateDoing, TS: formatTime(now)}),
		}, []Event{
			mustNewEvent(eventUnclaim, now, UnclaimEvent{ID: "T1", TS: formatTime(now)}),
			mustNewEvent(eventState, now, StateEvent{ID: "T1", NewState: stateTodo, TS: formatTime(now)}),
		}},
		{eventLink, []Event{create("T1"), create("T2")}, []Event{mustNewEvent(eventLink, now, LinkEvent{FromID: "T1", ToID: "T2", Type: dependsLinkType})}},
		{eventUnlink, []Event{create("T1"), create("T2"), mustNewEvent(eventLink, now, LinkEvent{FromID: "T1", ToID: "T2", Type: dependsLinkType})},
			[]Event{mustNewEvent(eventUnlink, now, LinkEvent{FromID: "T1", ToID: "T2", Type: dependsLinkType})}},
		{eventTitle, []Event{create("T1")}, []Event{mustNewEvent(eventTitle, now, TitleUpdateEvent{ID: "T1", Title: "changed", TS: formatTime(now)})}},
		{eventBody, []Event{create("T1")}, []Event{mustNewEvent(eventBody, now, BodyUpdateEvent{ID: "T1", Body: "changed", TS: formatTime(now)})}},
		{eventEpic, []Event{create("E1"), create("T1")}, []Event{mustNewEvent(eventEpic, now, EpicAssignEvent{ID: "T1", EpicID: "E1", TS: formatTime(now)})}},
		{eventTombstone, []Event{create("T1")}, []Event{mustNewEvent(eventTombstone, now, TombstoneEvent{ID: "T1", TS: formatTime(now)})}},
		{eventResult, []Event{create("T1")}, []Event{mustNewEvent(eventResult, now, ResultEvent{TaskID: "T1", Summary: "result", Path: "result.txt", TS: formatTime(now)})}},
		{eventMessage, []Event{create("T1")}, []Event{mustNewEvent(eventMessage, now, MessageEvent{TaskID: "T1", Kind: "done", Text: "note", TS: formatTime(now)})}},
	}
	if len(tests) != len(supportedEventKinds) {
		t.Fatalf("reducer fixtures=%d supported kinds=%d", len(tests), len(supportedEventKinds))
	}
	fixtures := make(map[string]struct{}, len(tests))
	for _, test := range tests {
		if _, duplicate := fixtures[test.kind]; duplicate {
			t.Fatalf("duplicate reducer fixture for %q", test.kind)
		}
		fixtures[test.kind] = struct{}{}
		t.Run(test.kind, func(t *testing.T) {
			base, err := replayEvents(test.base)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := applyTransaction(base, test.events); err != nil {
				t.Fatalf("supported kind did not reduce: %v", err)
			}
		})
	}
	for _, kind := range supportedEventKinds {
		if _, ok := fixtures[kind]; !ok {
			t.Fatalf("supported kind %q has no reducer fixture", kind)
		}
	}
}

func TestApplyTransactionIsPureAndRejectsAtomically(t *testing.T) {
	now := time.Now().UTC()
	base, err := replayEvents([]Event{mustNewEvent(eventNewTask, now, NewTaskEvent{
		ID: "T1", UUID: "uuid", State: stateTodo, Title: "original", CreatedAt: formatTime(now),
	})})
	if err != nil {
		t.Fatal(err)
	}
	before := cloneGraph(base)
	title := mustNewEvent(eventTitle, now, TitleUpdateEvent{ID: "T1", Title: "changed", TS: formatTime(now)})

	changed, err := applyTransaction(base, []Event{title})
	if err != nil {
		t.Fatal(err)
	}
	if base.Tasks["T1"].Title != "original" || !reflect.DeepEqual(base, before) {
		t.Fatal("successful transaction mutated its input graph")
	}
	if changed.Tasks["T1"].Title != "changed" {
		t.Fatal("successful transaction did not return the changed graph")
	}

	orphan := mustNewEvent(eventBody, now, BodyUpdateEvent{ID: "MISSING", Body: "bad", TS: formatTime(now)})
	if _, err := applyTransaction(base, []Event{title, orphan}); err == nil {
		t.Fatal("invalid later event did not reject the transaction")
	}
	if base.Tasks["T1"].Title != "original" || !reflect.DeepEqual(base, before) {
		t.Fatal("rejected transaction leaked a partial mutation")
	}
}

func TestApplyTransactionMatchesFullReplay(t *testing.T) {
	now := time.Now().UTC()
	history := []Event{mustNewEvent(eventNewTask, now, NewTaskEvent{
		ID: "T1", UUID: "uuid", State: stateTodo, Title: "original", CreatedAt: formatTime(now),
	})}
	proposed := []Event{
		mustNewEvent(eventTitle, now, TitleUpdateEvent{ID: "T1", Title: "changed", TS: formatTime(now)}),
		mustNewEvent(eventBody, now, BodyUpdateEvent{ID: "T1", Body: "body", TS: formatTime(now)}),
	}
	base, err := replayEvents(history)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := applyTransaction(base, proposed)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := replayEvents(append(append([]Event(nil), history...), proposed...))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(applied.Tasks, replayed.Tasks) || !reflect.DeepEqual(applied.Deps, replayed.Deps) {
		t.Fatal("proposed transaction application differs from full replay")
	}
}

func TestSupportedRecordKindRegistryIncludesSnapshotCodec(t *testing.T) {
	got := make(map[string]struct{}, len(supportedRecordKinds))
	for _, kind := range supportedRecordKinds {
		if _, duplicate := got[kind]; duplicate {
			t.Fatalf("duplicate record kind %q", kind)
		}
		got[kind] = struct{}{}
	}
	if _, ok := got[transactionRecordType]; !ok {
		t.Fatal("transaction record kind missing")
	}
	for _, kind := range []string{snapshotRecordType, snapshotTaskRecordType, snapshotResultRecordType, snapshotMessageRecordType, snapshotDependencyRecordType} {
		if _, ok := got[kind]; !ok || !snapshotKind(kind) {
			t.Fatalf("snapshot record kind %q is not registered consistently", kind)
		}
	}
}
