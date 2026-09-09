// Purpose: Validate a disposable checkpoint against its backlog file and
// replay only the appended tail.
// Exports: none; Repository.loadWithRead is the single integration boundary.
// Invariants: cache failures are misses, source-prefix bytes are never read on
// a hit, and authoritative errors are decided by the unchanged full loader.
package ergo

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func tryLoadBacklogCache(eventsPath string) (*Graph, eventLogRead, bool) {
	cachePath := filepath.Join(filepath.Dir(eventsPath), cacheFileName)
	cacheInfo, err := os.Lstat(cachePath)
	if err != nil || !cacheInfo.Mode().IsRegular() {
		return nil, eventLogRead{}, false
	}
	cacheFile, err := os.Open(cachePath)
	if err != nil {
		return nil, eventLogRead{}, false
	}
	decoded, err := decodeCache(cacheFile)
	closeErr := cacheFile.Close()
	if err != nil || closeErr != nil || decoded.source.Name != filepath.Base(eventsPath) {
		return nil, eventLogRead{}, false
	}

	sourceFile, err := os.Open(eventsPath)
	if err != nil {
		return nil, eventLogRead{}, false
	}
	defer sourceFile.Close()
	before, err := sourceFile.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return nil, eventLogRead{}, false
	}
	identity, ok := sourceFileIdentity(sourceFile, before)
	if !ok || identity != decoded.source.Identity || before.Size() < decoded.source.Bytes {
		return nil, eventLogRead{}, false
	}
	if before.Size() == decoded.source.Bytes && before.ModTime().UnixNano() != decoded.source.ModifiedNS {
		return nil, eventLogRead{}, false
	}
	if decoded.source.Bytes > 0 {
		boundary := []byte{0}
		if _, err := sourceFile.ReadAt(boundary, decoded.source.Bytes-1); err != nil || boundary[0] != '\n' {
			return nil, eventLogRead{}, false
		}
	}
	if _, err := sourceFile.Seek(decoded.source.Bytes, 0); err != nil {
		return nil, eventLogRead{}, false
	}
	read, err := inspectEventLogTail(sourceFile, eventsPath, decoded.source, before.Size())
	if err != nil {
		return nil, eventLogRead{}, false
	}
	after, err := sourceFile.Stat()
	if err != nil || after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		return nil, eventLogRead{}, false
	}
	afterIdentity, ok := sourceFileIdentity(sourceFile, after)
	if !ok || afterIdentity != identity {
		return nil, eventLogRead{}, false
	}

	raw, err := replayEventsOntoRaw(cloneGraph(decoded.graph), read.events)
	if err != nil {
		return nil, eventLogRead{}, false
	}
	final, err := replayEventsOnto(cloneGraph(raw), nil)
	if err != nil {
		return nil, eventLogRead{}, false
	}
	read.cacheGraph = raw
	read.cacheBase = decoded.source
	read.cacheHit = true
	if !read.truncatedTail && !read.needsSeparator && read.validBytes == after.Size() {
		read.cacheSource = cacheSource{
			Name: filepath.Base(eventsPath), Identity: identity, Bytes: after.Size(),
			Lines: read.lineCount, Records: read.recordCount, ModifiedNS: after.ModTime().UnixNano(),
		}
	}
	return final, read, true
}

func inspectEventLogTail(file *os.File, path string, prefix cacheSource, fileSize int64) (eventLogRead, error) {
	result := eventLogRead{recordCount: prefix.Records, lineCount: prefix.Lines, validBytes: prefix.Bytes}
	if fileSize == prefix.Bytes {
		return result, nil
	}
	endsWithNewline := false
	if fileSize > 0 {
		last := []byte{0}
		if _, err := file.ReadAt(last, fileSize-1); err == nil {
			endsWithNewline = last[0] == '\n'
		}
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogRecordBytes)
	var pending []byte
	pendingNo := 0
	currentNo := prefix.Lines
	processLine := func(lineNo int, line []byte) error {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			return nil
		}
		result.recordCount++
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(trimmed, &header); err != nil {
			return formatEventsParseError(path, lineNo, trimmed, err)
		}
		if snapshotKind(header.Type) {
			return fmt.Errorf("%s:%d: snapshot data record outside a snapshot block: %q", path, lineNo, header.Type)
		}
		events, err := decodeEventLogRecord(path, lineNo, trimmed)
		if err != nil {
			return err
		}
		result.events = append(result.events, events...)
		return nil
	}

	for scanner.Scan() {
		currentNo++
		line := append([]byte(nil), scanner.Bytes()...)
		if pending != nil {
			if err := processLine(pendingNo, pending); err != nil {
				return eventLogRead{}, err
			}
			result.validBytes += int64(len(pending) + 1)
		}
		pending = line
		pendingNo = currentNo
	}
	result.lineCount = currentNo
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return eventLogRead{}, fmt.Errorf("%s: event record too long (> %d bytes); file may be corrupted (e.g. missing newlines)", path, maxLogRecordBytes)
		}
		return eventLogRead{}, err
	}
	if pending != nil {
		if err := processLine(pendingNo, pending); err != nil {
			if !endsWithNewline && !json.Valid(bytes.TrimSpace(pending)) {
				result.truncatedTail = true
				return result, nil
			}
			return eventLogRead{}, err
		}
		result.validBytes += int64(len(pending))
		if endsWithNewline {
			result.validBytes++
		} else if len(pending) > 0 {
			result.needsSeparator = true
		}
	}
	return result, nil
}
