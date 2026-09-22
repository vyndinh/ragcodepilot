package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLeaseExclusiveAndDurableFailureState(t *testing.T) {
	stateDir := t.TempDir()
	first, err := acquireRunLease(stateDir, "code_chunks")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	fingerprint := inputFingerprint("code_chunks", "/tmp/repo", "representation-v1", []string{"go", "rust"}, 40, 5, map[string]string{"main.go": "abc"})
	if err := first.begin("code_chunks", "/tmp/repo", "representation-v1", fingerprint, 1); err != nil {
		t.Fatalf("begin: %v", err)
	}

	if _, err := acquireRunLease(stateDir, "code_chunks"); err == nil || !strings.Contains(err.Error(), "already being indexed") {
		t.Fatalf("second acquire error = %v, want active-writer error", err)
	}
	if err := first.fail(os.ErrClosed); err != nil {
		t.Fatalf("fail: %v", err)
	}
	stateBytes, err := os.ReadFile(filepath.Join(stateDir, collectionKey("code_chunks")+".json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state RunState
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.Status != "failed" || state.InputFingerprint != fingerprint || state.LastError == "" {
		t.Fatalf("state = %+v, want failed state with fingerprint and error", state)
	}
	if err := first.release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	second, err := acquireRunLease(stateDir, "code_chunks")
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	if err := second.complete(); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := second.release(); err != nil {
		t.Fatalf("second release: %v", err)
	}
}

func TestInputFingerprintIsStableAcrossMapAndLanguageOrder(t *testing.T) {
	left := inputFingerprint("collection", "/repo", "v1", []string{"rust", "go"}, 40, 5, map[string]string{"b.go": "2", "a.go": "1"})
	right := inputFingerprint("collection", "/repo", "v1", []string{"go", "rust"}, 40, 5, map[string]string{"a.go": "1", "b.go": "2"})
	if left != right {
		t.Fatalf("fingerprints differ: %q vs %q", left, right)
	}
}
