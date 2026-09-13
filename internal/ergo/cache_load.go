// Purpose: Validate a disposable checkpoint against its backlog file and
// replay only the appended tail.
// Exports: none; Repository.loadWithRead is the single integration boundary.
// Invariants: cache failures are misses; a hit reads only a fixed boundary
// probe and the tail. The full loader decides authoritative errors.
package ergo

import (
	"os"
	"path/filepath"
)

func tryLoadBacklogCache(eventsPath string) (*Graph, backlogRead, bool) {
	cachePath := filepath.Join(filepath.Dir(eventsPath), cacheFileName)
	cacheInfo, err := os.Lstat(cachePath)
	if err != nil || !cacheInfo.Mode().IsRegular() {
		return nil, backlogRead{}, false
	}
	cacheFile, err := os.Open(cachePath)
	if err != nil {
		return nil, backlogRead{}, false
	}
	decoded, err := decodeCache(cacheFile)
	closeErr := cacheFile.Close()
	if err != nil || closeErr != nil || decoded.source.Name != filepath.Base(eventsPath) {
		return nil, backlogRead{}, false
	}

	sourceFile, err := os.Open(eventsPath)
	if err != nil {
		return nil, backlogRead{}, false
	}
	defer sourceFile.Close()
	before, err := sourceFile.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return nil, backlogRead{}, false
	}
	identity, ok := sourceFileIdentity(sourceFile, before)
	if !ok || identity != decoded.source.Identity || before.Size() < decoded.source.Bytes {
		return nil, backlogRead{}, false
	}
	if before.Size() == decoded.source.Bytes && before.ModTime().UnixNano() != decoded.source.ModifiedNS {
		return nil, backlogRead{}, false
	}
	if decoded.source.Bytes > 0 {
		boundary := []byte{0}
		if _, err := sourceFile.ReadAt(boundary, decoded.source.Bytes-1); err != nil || boundary[0] != '\n' {
			return nil, backlogRead{}, false
		}
	}
	if _, err := sourceFile.Seek(decoded.source.Bytes, 0); err != nil {
		return nil, backlogRead{}, false
	}
	read, err := inspectEventLogTail(sourceFile, eventsPath, decoded.source, before.Size())
	if err != nil {
		return nil, backlogRead{}, false
	}
	after, err := sourceFile.Stat()
	if err != nil || after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		return nil, backlogRead{}, false
	}
	afterIdentity, ok := sourceFileIdentity(sourceFile, after)
	if !ok || afterIdentity != identity {
		return nil, backlogRead{}, false
	}

	raw := decoded.graph
	if len(read.events) > 0 {
		raw, err = replayEventsOntoRaw(raw, read.events)
		if err != nil {
			return nil, backlogRead{}, false
		}
	}
	if !read.truncatedTail && !read.needsSeparator && read.validBytes == after.Size() {
		read.source = backlogSource{
			Name: filepath.Base(eventsPath), Identity: identity, Bytes: after.Size(),
			Lines: read.lineCount, Records: read.recordCount, ModifiedNS: after.ModTime().UnixNano(),
		}
	}
	graph, loaded := finishBacklogLoad(raw, read, &decoded.source)
	return graph, loaded, true
}

func inspectEventLogTail(file *os.File, path string, prefix backlogSource, fileSize int64) (eventLogRead, error) {
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

	return scanEventLog(file, path, prefix, endsWithNewline, false)
}
