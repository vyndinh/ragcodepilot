package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hello.go")
	if err := os.WriteFile(f, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := HashFile(f)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}

	if len(hash) != 64 { // SHA-256 hex is 64 chars
		t.Fatalf("expected 64-char hex hash, got %d: %s", len(hash), hash)
	}

	// Same content should produce the same hash.
	hash2, err := HashFile(f)
	if err != nil {
		t.Fatalf("HashFile second call: %v", err)
	}
	if hash != hash2 {
		t.Fatalf("expected identical hashes, got %s and %s", hash, hash2)
	}
}

func TestHashFile_NotFound(t *testing.T) {
	_, err := HashFile("/nonexistent/path/file.go")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestHashFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.go")
	f2 := filepath.Join(dir, "b.go")
	if err := os.WriteFile(f1, []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("package b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	hashes, err := HashFiles([]string{f1, f2})
	if err != nil {
		t.Fatalf("HashFiles: %v", err)
	}

	if len(hashes) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(hashes))
	}
	if hashes[f1] == hashes[f2] {
		t.Fatal("different files should have different hashes")
	}
}
