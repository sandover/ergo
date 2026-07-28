// Purpose: Encode and decode non-snapshot physical JSONL records.
// Role: Versioned transaction codec and released standalone-event compatibility.
package ergo

import (
	"encoding/json"
	"fmt"
)

const transactionRecordType = "transaction"

var supportedRecordKinds = []string{
	transactionRecordType,
	snapshotRecordType,
	snapshotTaskRecordType,
	snapshotResultRecordType,
	snapshotMessageRecordType,
	snapshotDependencyRecordType,
}

type transactionRecord struct {
	Type    string  `json:"type"`
	Version int     `json:"version"`
	Events  []Event `json:"events"`
}

func decodeEventLogRecord(path string, line int, raw []byte) ([]Event, error) {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, formatEventsParseError(path, line, raw, err)
	}
	if header.Type != transactionRecordType {
		var event Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, formatEventsParseError(path, line, raw, err)
		}
		event.Source = EventSource{Path: path, Line: line}
		return []Event{event}, nil
	}
	var record transactionRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, formatEventsParseError(path, line, raw, err)
	}
	if record.Version != 1 {
		return nil, fmt.Errorf("%s:%d: unsupported transaction record version %d", path, line, record.Version)
	}
	if len(record.Events) == 0 {
		return nil, fmt.Errorf("%s:%d: transaction record contains no events", path, line)
	}
	for i := range record.Events {
		record.Events[i].Source = EventSource{Path: path, Line: line, TransactionIndex: i + 1}
	}
	return record.Events, nil
}

func marshalTransaction(events []Event) ([]byte, error) {
	if len(events) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(transactionRecord{Type: transactionRecordType, Version: 1, Events: events})
	if err != nil {
		return nil, err
	}
	if len(data) > maxLogRecordBytes {
		return nil, fmt.Errorf("transaction record is too long: %d bytes exceeds the %d-byte limit", len(data), maxLogRecordBytes)
	}
	return append(data, '\n'), nil
}
