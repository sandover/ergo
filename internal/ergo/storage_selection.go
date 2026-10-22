// Purpose: Resolve source projects, personal backlogs, and external storage.
// Invariants: Storage selection never creates files or performs Git synchronization.
package ergo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type storageConfig struct {
	Version     int              `json:"version"`
	DefaultUser string           `json:"default_user,omitempty"`
	Projects    []projectStorage `json:"projects,omitempty"`
}

type projectStorage struct {
	Root           string `json:"root"`
	DataDir        string `json:"ergo_data,omitempty"`
	DefaultBacklog string `json:"default_backlog,omitempty"`
}

type storageSelection struct {
	Project string
	Dir     string
	Owner   string
	Source  string
}

// canonicalPath also handles a not-yet-created storage directory by resolving
// its nearest existing ancestor. It does not expand shell syntax.
func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ergo", "config.json"), nil
}

func loadStorageConfig() (storageConfig, error) {
	var config storageConfig
	path, err := configPath()
	if err != nil {
		return config, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return config, fmt.Errorf("read Ergo config %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&config); err == nil {
		var extra any
		if next := decoder.Decode(&extra); next != io.EOF {
			err = errors.New("expected one JSON object")
		}
	}
	if err != nil {
		return config, fmt.Errorf("invalid Ergo config %s: %w", path, err)
	}
	if config.Version != 1 {
		return config, fmt.Errorf("unsupported Ergo config version %d in %s (expected 1)", config.Version, path)
	}
	if config.DefaultUser != "" {
		if err := validateUserID(config.DefaultUser); err != nil {
			return config, fmt.Errorf("%s: default_user: %w", path, err)
		}
	}
	seen := make(map[string]bool)
	for i := range config.Projects {
		project := &config.Projects[i]
		if project.Root == "" {
			return config, fmt.Errorf("%s: projects[%d].root is required", path, i)
		}
		for _, field := range []*string{&project.Root, &project.DataDir} {
			if *field == "" {
				continue
			}
			value := *field
			if !filepath.IsAbs(value) {
				value = filepath.Join(filepath.Dir(path), value)
			}
			*field, err = canonicalPath(value)
			if err != nil {
				return config, fmt.Errorf("%s: %w", path, err)
			}
		}
		key := project.Root
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return config, fmt.Errorf("%s: duplicate project root %s", path, project.Root)
		}
		seen[key] = true
		switch project.DefaultBacklog {
		case "", "shared", "personal":
		default:
			return config, fmt.Errorf("%s: default_backlog must be shared or personal", path)
		}
	}
	return config, nil
}

// Lowercase portable identifiers avoid case-folding collisions across platforms.
func validateUserID(id string) error {
	if len(id) == 0 || len(id) > 64 {
		return errors.New("user ID must be 1-64 lowercase ASCII letters, digits, hyphens, or underscores")
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return errors.New("user ID must contain only lowercase ASCII letters, digits, hyphens, or underscores")
		}
	}
	switch id {
	case "con", "prn", "aux", "nul", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return fmt.Errorf("user ID %q is reserved on Windows", id)
	}
	return nil
}

func userDataDir() (string, error) {
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return dir, nil
		}
		return "", errors.New("LOCALAPPDATA is not set")
	}
	if runtime.GOOS == "darwin" {
		return os.UserConfigDir()
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", errors.New("XDG_DATA_HOME must be absolute")
		}
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}

func pathContains(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func resolveStorage(opts RepositoryOptions, initialize bool) (storageSelection, error) {
	var selected storageSelection
	if opts.Shared && opts.User != "" {
		return selected, errors.New("--shared and --user conflict")
	}
	if opts.User != "" {
		if err := validateUserID(opts.User); err != nil {
			return selected, err
		}
	}
	config, err := loadStorageConfig()
	if err != nil {
		return selected, err
	}
	start := opts.StartDir
	if start == "" {
		start = "."
	}
	start, err = canonicalPath(start)
	if err != nil {
		return selected, err
	}
	var mapping *projectStorage
	for i := range config.Projects {
		candidate := &config.Projects[i]
		if pathContains(candidate.Root, start) && (mapping == nil || len(candidate.Root) > len(mapping.Root)) {
			mapping = candidate
		}
	}
	owner := opts.User
	if owner == "" && !opts.Shared && mapping != nil && mapping.DefaultBacklog == "personal" {
		owner = config.DefaultUser
		if owner == "" {
			return selected, errors.New("personal default backlog requires default_user in Ergo config or --user <id>")
		}
	}
	selected.Owner = "shared"
	if owner != "" {
		selected.Owner = owner
	}
	base := opts.DataDir
	selected.Source = "repository"
	if base != "" {
		selected.Source = "--data-dir"
	} else if mapping != nil && mapping.DataDir != "" {
		base = mapping.DataDir
		selected.Source = "config"
	}

	if mapping != nil {
		selected.Project = mapping.Root
	} else if initialize && owner == "" && base == "" {
		// Legacy init creates in the requested directory, not an ancestor.
		selected.Project = start
		if filepath.Base(start) == dataDirName {
			selected.Project = filepath.Dir(start)
		}
	} else {
		local, discoverErr := resolveErgoDir(start)
		if discoverErr == nil {
			selected.Project = filepath.Dir(local)
		} else if !errors.Is(discoverErr, ErrNoErgoDir) {
			return selected, discoverErr
		} else if owner != "" || base != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			output, gitErr := exec.CommandContext(ctx, "git", "-C", start, "rev-parse", "--show-toplevel").Output()
			cancel()
			if gitErr == nil {
				selected.Project, err = canonicalPath(strings.TrimSpace(string(output)))
			} else if opts.StartDir != "" {
				selected.Project = start
			} else {
				return selected, errors.New("cannot identify source project; use --dir <project-root> or configure a project mapping")
			}
			if err != nil {
				return selected, err
			}
		} else {
			return selected, discoverErr
		}
	}
	if base == "" {
		if owner == "" {
			base = filepath.Join(selected.Project, dataDirName)
		} else {
			data, err := userDataDir()
			if err != nil {
				return selected, err
			}
			key := selected.Project
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			hash := sha256.Sum256([]byte(key))
			base = filepath.Join(data, "ergo", "projects", fmt.Sprintf("%x", hash[:16]))
			selected.Source = "personal default"
		}
	}
	if owner != "" {
		base = filepath.Join(base, "users", owner)
	}
	selected.Dir, err = canonicalPath(base)
	if err != nil {
		return selected, err
	}
	if !initialize {
		info, err := os.Stat(selected.Dir)
		if errors.Is(err, os.ErrNotExist) {
			return selected, fmt.Errorf("%w: %s (run ergo init with the same selection)", ErrNoErgoDir, selected.Dir)
		}
		if err != nil {
			return selected, err
		}
		if !info.IsDir() {
			return selected, fmt.Errorf("%s exists but is not a directory", selected.Dir)
		}
		// An existing external parent is not itself an initialized backlog.
		if selected.Source != "repository" || owner != "" {
			log, err := selectEventsPath(selected.Dir)
			if err != nil {
				return selected, err
			}
			if _, err = os.Stat(log); errors.Is(err, os.ErrNotExist) {
				return selected, fmt.Errorf("%w: %s (run ergo init with the same selection)", ErrNoErgoDir, selected.Dir)
			} else if err != nil {
				return selected, err
			}
		}
	}
	return selected, nil
}
