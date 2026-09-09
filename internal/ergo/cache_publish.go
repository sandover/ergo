// Purpose: Publish and refresh the disposable backlog cache automatically.
// Exports: none; repository operations invoke it under their existing lock.
// Invariants: publication uses a unique complete temporary file and atomic
// replacement; every cache or ignore failure is silent and non-authoritative.
package ergo

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	cacheCreateBytes    = 4 * 1024 * 1024
	cacheRefreshBytes   = 1 * 1024 * 1024
	cacheRefreshRecords = 256
	cacheIgnoreComment  = "# Ergo performance cache"
	cacheIgnoreFileRule = "/cache.jsonl"
	cacheIgnoreTempRule = "/cache.tmp-*"
)

func (r *Repository) publishCache(read eventLogRead) {
	if r == nil || !cacheRefreshNeeded(read) {
		return
	}
	data, err := marshalCache(read.cacheGraph, read.cacheSource)
	if err != nil {
		return
	}
	_ = ensureCacheIgnored(r.dir)
	_ = publishCacheFile(filepath.Join(r.dir, cacheFileName), data)
}

func cacheRefreshNeeded(read eventLogRead) bool {
	if read.cacheGraph == nil || read.cacheSource.Identity == "" || read.cacheSource.Bytes < 0 {
		return false
	}
	if !read.cacheHit {
		return read.cacheSource.Bytes >= cacheCreateBytes
	}
	if read.cacheSource.Bytes <= read.cacheBase.Bytes {
		return false
	}
	return read.cacheSource.Bytes-read.cacheBase.Bytes >= cacheRefreshBytes ||
		read.cacheSource.Records-read.cacheBase.Records >= cacheRefreshRecords
}

func publishCacheFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "cache.tmp-*")
	if err != nil {
		return err
	}
	tmpPath := file.Name()
	defer os.Remove(tmpPath)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if err := writeAllWith(file, data, func(file *os.File, chunk []byte) (int, error) {
		return file.Write(chunk)
	}); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return replaceCacheFile(tmpPath, path)
}

func ensureCacheIgnored(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	info, err := os.Lstat(path)
	switch {
	case err == nil && !info.Mode().IsRegular():
		return errors.New(".ergo/.gitignore is not a regular file")
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	hasFile := hasIgnoreLine(existing, cacheIgnoreFileRule)
	hasTemp := hasIgnoreLine(existing, cacheIgnoreTempRule)
	if hasFile && hasTemp {
		return nil
	}
	var addition bytes.Buffer
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		addition.WriteByte('\n')
	}
	if !bytes.Contains(existing, []byte(cacheIgnoreComment)) {
		addition.WriteString(cacheIgnoreComment)
		addition.WriteByte('\n')
	}
	if !hasFile {
		addition.WriteString(cacheIgnoreFileRule)
		addition.WriteByte('\n')
	}
	if !hasTemp {
		addition.WriteString(cacheIgnoreTempRule)
		addition.WriteByte('\n')
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	file, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return err
	}
	if _, err := file.Write(addition.Bytes()); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func hasIgnoreLine(data []byte, target string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}
