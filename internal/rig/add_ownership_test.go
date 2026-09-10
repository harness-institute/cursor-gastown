package rig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveRigPathIfOwned_MatchingStampRemovesPath(t *testing.T) {
	rigPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rigPath, "some-file"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	stamp, err := newAddOwnershipStamp()
	if err != nil {
		t.Fatalf("new ownership stamp: %v", err)
	}
	if err := writeAddOwnershipStamp(rigPath, stamp); err != nil {
		t.Fatalf("write ownership stamp: %v", err)
	}

	removeRigPathIfOwned(rigPath, stamp)

	if _, err := os.Stat(rigPath); !os.IsNotExist(err) {
		t.Fatalf("expected rig path to be removed, stat err=%v", err)
	}
}

func TestRemoveRigPathIfOwned_MismatchedStampKeepsPath(t *testing.T) {
	rigPath := t.TempDir()
	preserved := filepath.Join(rigPath, "preserve-me")
	if err := os.WriteFile(preserved, []byte("important"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writeAddOwnershipStamp(rigPath, "newer-stamp"); err != nil {
		t.Fatalf("write ownership stamp: %v", err)
	}

	removeRigPathIfOwned(rigPath, "older-stamp")

	if _, err := os.Stat(preserved); err != nil {
		t.Fatalf("preserved file was deleted: %v", err)
	}
}

func TestRemoveRigPathIfOwned_MissingStampOnNonEmptyPathKeepsPath(t *testing.T) {
	rigPath := t.TempDir()
	preserved := filepath.Join(rigPath, "rig-content")
	if err := os.WriteFile(preserved, []byte("important"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	removeRigPathIfOwned(rigPath, "stale-stamp")

	if _, err := os.Stat(preserved); err != nil {
		t.Fatalf("preserved file was deleted: %v", err)
	}
}

func TestRemoveRigPathIfOwned_MissingStampOnEmptyPathRemovesPath(t *testing.T) {
	rigPath := t.TempDir()

	removeRigPathIfOwned(rigPath, "stale-stamp")

	if _, err := os.Stat(rigPath); !os.IsNotExist(err) {
		t.Fatalf("expected empty rig path to be removed, stat err=%v", err)
	}
}

func TestRemoveRigPathIfOwned_NoExpectedStampRemovesPath(t *testing.T) {
	rigPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rigPath, "x"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	removeRigPathIfOwned(rigPath, "")

	if _, err := os.Stat(rigPath); !os.IsNotExist(err) {
		t.Fatalf("expected rig path to be removed, stat err=%v", err)
	}
}
