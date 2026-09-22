package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"
)

const runStateSchemaVersion = 1

// DefaultRunStateDir returns the durable per-user directory used for index run
// state and collection writer locks. It is outside the repository so indexing
// does not create files that can be accidentally committed or indexed.
func DefaultRunStateDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache directory: %w", err)
	}
	return filepath.Join(cacheDir, "ragcodepilot", "index-state"), nil
}

// RunState records the durable lifecycle of the most recent index run for a
// collection. A failed or interrupted run remains visible as incomplete work.
type RunState struct {
	SchemaVersion    int    `json:"schema_version"`
	Status           string `json:"status"`
	Collection       string `json:"collection"`
	Repository       string `json:"repository"`
	Representation   string `json:"representation"`
	InputFingerprint string `json:"input_fingerprint"`
	StartedAt        string `json:"started_at"`
	UpdatedAt        string `json:"updated_at"`
	ProcessID        int    `json:"process_id"`
	Host             string `json:"host"`
	FileCount        int    `json:"file_count"`
	LastError        string `json:"last_error,omitempty"`
}

type runLease struct {
	stateDir  string
	statePath string
	lockPath  string
	lockFile  *os.File
	state     RunState
	started   bool
}

// acquireRunLease obtains exclusive ownership for a collection. The lock is
// created atomically. If a prior process died, its PID is checked and its lock
// is archived before retrying; a live owner is never interrupted.
func acquireRunLease(stateDir, collection string) (*runLease, error) {
	if stateDir == "" {
		return &runLease{}, nil
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating run-state directory: %w", err)
	}
	key := collectionKey(collection)
	lease := &runLease{
		stateDir:  stateDir,
		statePath: filepath.Join(stateDir, key+".json"),
		lockPath:  filepath.Join(stateDir, key+".lock"),
	}
	for attempt := 0; attempt < 2; attempt++ {
		lockFile, err := os.OpenFile(lease.lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			lease.lockFile = lockFile
			payload := map[string]any{
				"collection": collection,
				"process_id": os.Getpid(),
				"host":       hostname(),
				"started_at": time.Now().UTC().Format(time.RFC3339Nano),
			}
			data, marshalErr := json.MarshalIndent(payload, "", "  ")
			if marshalErr == nil {
				_, marshalErr = lockFile.Write(append(data, '\n'))
			}
			if marshalErr != nil {
				_ = lockFile.Close()
				_ = os.Remove(lease.lockPath)
				return nil, fmt.Errorf("writing writer lock: %w", marshalErr)
			}
			return lease, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("creating writer lock: %w", err)
		}
		if !reclaimDeadLock(lease.lockPath) {
			owner := readLockOwner(lease.lockPath)
			return nil, fmt.Errorf("collection %q is already being indexed%s; lock: %s", collection, owner, lease.lockPath)
		}
	}
	return nil, fmt.Errorf("could not acquire writer lock for collection %q", collection)
}

func (l *runLease) begin(collection, repository, representation, fingerprint string, fileCount int) error {
	if l == nil || l.lockFile == nil {
		return nil
	}
	l.state = RunState{
		SchemaVersion:    runStateSchemaVersion,
		Status:           "in_progress",
		Collection:       collection,
		Repository:       repository,
		Representation:   representation,
		InputFingerprint: fingerprint,
		StartedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		ProcessID:        os.Getpid(),
		Host:             hostname(),
		FileCount:        fileCount,
	}
	l.started = true
	return l.writeState()
}

func (l *runLease) complete() error {
	if l == nil || l.lockFile == nil || !l.started {
		return nil
	}
	l.state.Status = "completed"
	l.state.LastError = ""
	l.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return l.writeState()
}

func (l *runLease) fail(runErr error) error {
	if l == nil || l.lockFile == nil || !l.started {
		return nil
	}
	l.state.Status = "failed"
	if runErr != nil {
		l.state.LastError = runErr.Error()
	}
	l.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return l.writeState()
}

func (l *runLease) release() error {
	if l == nil || l.lockFile == nil {
		return nil
	}
	closeErr := l.lockFile.Close()
	removeErr := os.Remove(l.lockPath)
	l.lockFile = nil
	if closeErr != nil {
		return fmt.Errorf("closing writer lock: %w", closeErr)
	}
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return fmt.Errorf("removing writer lock: %w", removeErr)
	}
	return nil
}

func (l *runLease) writeState() error {
	data, err := json.MarshalIndent(l.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding run state: %w", err)
	}
	tmp, err := os.CreateTemp(l.stateDir, ".run-state-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary run state: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting run-state permissions: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing run state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing run state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing run state: %w", err)
	}
	if err := os.Rename(tmpName, l.statePath); err != nil {
		return fmt.Errorf("publishing run state: %w", err)
	}
	return nil
}

func collectionKey(collection string) string {
	digest := sha256.Sum256([]byte(collection))
	return hex.EncodeToString(digest[:])
}

func inputFingerprint(collection, repository, representation string, languages []string, chunkSize, chunkOverlap int, fileHashes map[string]string) string {
	langs := append([]string(nil), languages...)
	sort.Strings(langs)
	keys := make([]string, 0, len(fileHashes))
	for path := range fileHashes {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	h := sha256.New()
	fmt.Fprintf(h, "collection=%s\nrepository=%s\nrepresentation=%s\nchunk_size=%d\nchunk_overlap=%d\nlanguages=%v\n", collection, repository, representation, chunkSize, chunkOverlap, langs)
	for _, path := range keys {
		fmt.Fprintf(h, "%s=%s\n", path, fileHashes[path])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func readLockOwner(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var owner struct {
		ProcessID int    `json:"process_id"`
		Host      string `json:"host"`
	}
	if json.Unmarshal(data, &owner) != nil || owner.ProcessID == 0 {
		return ""
	}
	return fmt.Sprintf(" (pid %d on %s)", owner.ProcessID, owner.Host)
}

func reclaimDeadLock(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var owner struct {
		ProcessID int `json:"process_id"`
	}
	if json.Unmarshal(data, &owner) != nil || owner.ProcessID <= 0 {
		return false
	}
	process, err := os.FindProcess(owner.ProcessID)
	if err != nil {
		return false
	}
	if signalErr := process.Signal(syscall.Signal(0)); signalErr == nil || errors.Is(signalErr, syscall.EPERM) {
		return false
	}
	stalePath := path + ".stale." + strconv.FormatInt(time.Now().UnixNano(), 10)
	return os.Rename(path, stalePath) == nil
}

func hostname() string {
	host, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return host
}
