package vfs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanVault(t *testing.T) {
	tempDir := t.TempDir()

	// Create a mock vault structure
	files := []string{
		"note1.md",
		"folder1/note2.md",
		".hidden/note3.md",
		"image.png",
	}

	for _, f := range files {
		fullPath := filepath.Join(tempDir, f)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		err = os.WriteFile(fullPath, []byte("content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	scannedFiles, err := ScanVault(tempDir)
	if err != nil {
		t.Fatalf("ScanVault returned error: %v", err)
	}

	// note3.md is hidden, image.png is not .md
	expectedFiles := 2
	if len(scannedFiles) != expectedFiles {
		t.Errorf("Expected %d files, got %d", expectedFiles, len(scannedFiles))
	}

	foundNote1 := false
	foundNote2 := false
	for _, f := range scannedFiles {
		// filepath.Rel could use either / or \ depending on OS. We convert to / for testing.
		f = strings.ReplaceAll(f, "\\", "/")
		if f == "note1.md" {
			foundNote1 = true
		}
		if f == "folder1/note2.md" {
			foundNote2 = true
		}
	}

	if !foundNote1 {
		t.Errorf("Expected to find note1.md")
	}
	if !foundNote2 {
		t.Errorf("Expected to find folder1/note2.md")
	}
}

func TestAtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test_write.md")
	content := []byte("hello world")

	err := AtomicWrite(targetPath, content)
	if err != nil {
		t.Fatalf("AtomicWrite returned error: %v", err)
	}

	written, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if !bytes.Equal(written, content) {
		t.Errorf("Expected content %s, got %s", content, written)
	}
}

func TestSafeMove(t *testing.T) {
	tempDir := t.TempDir()
	oldPath := filepath.Join(tempDir, "old.md")
	newPath := filepath.Join(tempDir, "folder", "new.md")
	content := []byte("move me")

	err := os.WriteFile(oldPath, content, 0644)
	if err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}

	err = SafeMove(oldPath, newPath)
	if err != nil {
		t.Fatalf("SafeMove returned error: %v", err)
	}

	// Verify old file doesn't exist
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("Expected old file to not exist")
	}

	// Verify new file exists and has correct content
	written, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("Failed to read new file: %v", err)
	}

	if !bytes.Equal(written, content) {
		t.Errorf("Expected content %s, got %s", content, written)
	}
}

func TestWithRetry_Success(t *testing.T) {
	attempts := 0
	err := withRetry(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestWithRetry_Failure(t *testing.T) {
	attempts := 0
	expectedErr := errors.New("permanent error")
	err := withRetry(func() error {
		attempts++
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("Expected %v, got %v", expectedErr, err)
	}
	if attempts != maxRetries {
		t.Errorf("Expected %d attempts, got %d", maxRetries, attempts)
	}
}
