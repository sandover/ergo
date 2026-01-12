// Data directory discovery and event storage helpers.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	dataDirName = ".ergo"
)

func findDataDir(start string) (string, bool) {
	current := start
	for {
		candidate := filepath.Join(current, dataDirName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, true
		}
		if current == filepath.Dir(current) {
			return "", false
		}
		current = filepath.Dir(current)
	}
}

func resolveErgoDir(start string) (string, error) {
	ergoDir, ok := findDataDir(start)
	if ok {
		return ergoDir, nil
	}
	if filepath.Base(start) == dataDirName {
		if info, err := os.Stat(start); err == nil && info.IsDir() {
			return start, nil
		}
	}
	return "", fmt.Errorf("%w (run ergo init)", errNoErgoDir)
}

func ergoDir(opts GlobalOptions) (string, error) {
	start := opts.StartDir
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = wd
	}
	debugf(opts, "discover start=%s", start)
	return resolveErgoDir(start)
}

func loadGraph(dir string) (*Graph, error) {
	eventsPath := filepath.Join(dir, "events.jsonl")
	events, err := readEvents(eventsPath)
	if err != nil {
		return nil, err
	}
	return replayEvents(events)
}

func readEvents(path string) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var events []Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func appendEvents(path string, events []Event) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeEventsFile(path string, events []Event) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeLinkEvent(dir string, opts GlobalOptions, eventType, from, to string) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		fromItem, ok := graph.Tasks[from]
		if !ok {
			return fmt.Errorf("unknown id %s", from)
		}
		toItem, ok := graph.Tasks[to]
		if !ok {
			return fmt.Errorf("unknown id %s", to)
		}
		// Validate dependency rules
		if err := validateDepSelf(from, to); err != nil {
			return err
		}
		if err := validateDepKinds(isEpic(fromItem), isEpic(toItem)); err != nil {
			return err
		}
		// Cycle detection for new links
		if eventType == "link" {
			if hasCycle(graph, from, to) {
				return errors.New("dependency would create a cycle")
			}
		}
		now := time.Now().UTC()
		event, err := newEvent(eventType, now, LinkEvent{
			FromID: from,
			ToID:   to,
			Type:   dependsLinkType,
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}

func writeWorkerEvent(dir string, opts GlobalOptions, id string, worker Worker) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if _, ok := graph.Tasks[id]; !ok {
			return fmt.Errorf("unknown task id %s", id)
		}
		now := time.Now().UTC()
		event, err := newEvent("worker", now, WorkerEvent{
			ID:     id,
			Worker: string(worker),
			TS:     formatTime(now),
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}

func createTask(dir string, opts GlobalOptions, epicID string, isEpic bool, body string, worker Worker) (createOutput, error) {
	eventsPath := filepath.Join(dir, "events.jsonl")
	lockPath := filepath.Join(dir, "lock")
	return createTaskWithDir(dir, opts, lockPath, eventsPath, epicID, isEpic, body, worker)
}

func createTaskWithDir(dir string, opts GlobalOptions, lockPath, eventsPath, epicID string, isEpic bool, body string, worker Worker) (createOutput, error) {
	if worker == "" {
		worker = workerAny
	}
	var output createOutput
	err := withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if !isEpic && epicID != "" {
			epic, ok := graph.Tasks[epicID]
			if !ok {
				return fmt.Errorf("unknown epic id %s", epicID)
			}
			if epic.EpicID != "" {
				return fmt.Errorf("task %s is not an epic", epicID)
			}
		}
		id, err := newShortID(graph.Tasks)
		if err != nil {
			return err
		}
		uuid, err := newUUID()
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		payload := NewTaskEvent{
			ID:        id,
			UUID:      uuid,
			EpicID:    epicID,
			State:     stateTodo,
			Body:      body,
			Worker:    string(worker),
			CreatedAt: formatTime(now),
		}
		if isEpic {
			payload.EpicID = ""
		}
		eventType := "new_task"
		if isEpic {
			eventType = "new_epic"
		}
		event, err := newEvent(eventType, now, payload)
		if err != nil {
			return err
		}
		if err := appendEvents(eventsPath, []Event{event}); err != nil {
			return err
		}
		kind := "task"
		if isEpic {
			kind = "epic"
		}
		output = createOutput{
			Kind:      kind,
			ID:        id,
			UUID:      uuid,
			EpicID:    payload.EpicID,
			State:     stateTodo,
			Worker:    string(worker),
			CreatedAt: payload.CreatedAt,
		}
		return nil
	})
	if err != nil {
		return createOutput{}, err
	}
	return output, nil
}
// writeResultEvent attaches a result to a task.
// If content is non-empty, it writes an artifact file and uses its ref.
// If content is empty, ref must be a URL.
func writeResultEvent(dir string, opts GlobalOptions, taskID, summary, content, urlRef string) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		task, ok := graph.Tasks[taskID]
		if !ok {
			return fmt.Errorf("unknown task id %s", taskID)
		}
		if isEpic(task) {
			return errors.New("cannot attach result to epic")
		}
		if err := validateResultSummary(summary); err != nil {
			return err
		}

		var ref string
		if content != "" {
			// Write artifact file
			artifactRef, err := writeArtifact(dir, content)
			if err != nil {
				return err
			}
			ref = artifactRef
		} else if urlRef != "" {
			ref = urlRef
		} else {
			return errors.New("result requires content or URL")
		}

		now := time.Now().UTC()
		event, err := newEvent("result", now, ResultEvent{
			TaskID:  taskID,
			Summary: strings.TrimSpace(summary),
			Ref:     ref,
			TS:      formatTime(now),
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}

// writeArtifact writes content to .ergo/artifacts/ with content-addressed naming.
// Returns the relative ref (e.g., "artifacts/a1b2c3d4.txt").
func writeArtifact(dir, content string) (string, error) {
	artifactsDir := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		return "", err
	}

	// Content-addressed filename using SHA256 prefix
	hash := sha256.Sum256([]byte(content))
	filename := fmt.Sprintf("%x.txt", hash[:8]) // 16 hex chars
	ref := filepath.Join("artifacts", filename)
	fullPath := filepath.Join(dir, ref)

	// Write atomically (write to temp, rename)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", err
	}
	return ref, nil
}

// readArtifact reads an artifact file from .ergo/.
func readArtifact(dir, ref string) (string, error) {
	fullPath := filepath.Join(dir, ref)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}