package auth

import (
	"os"
	"path/filepath"
	"testing"
)

// redirectPasswdFile points PasswdFile at a temp location for the duration of
// the test and ensures the parent directory exists. The original value is
// restored on cleanup.
func redirectPasswdFile(t *testing.T) {
	t.Helper()
	orig := PasswdFile
	t.Cleanup(func() { PasswdFile = orig })
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir temp dir: %v", err)
	}
	PasswdFile = filepath.Join(dir, ".playback_passwd")
}

func TestSetPasswordAndIsSet(t *testing.T) {
	redirectPasswdFile(t)

	if IsSet() {
		t.Fatal("IsSet() = true before any password set, want false")
	}
	if err := SetPassword("hunter2pass"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !IsSet() {
		t.Fatal("IsSet() = false after SetPassword, want true")
	}
}

func TestVerify(t *testing.T) {
	redirectPasswdFile(t)

	if err := SetPassword("correct-horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := Verify("correct-horse"); err != nil {
		t.Errorf("Verify(correct) = %v, want nil", err)
	}
	if err := Verify("wrong-horse"); err == nil {
		t.Error("Verify(wrong) = nil, want non-nil error")
	}
}

func TestRemove(t *testing.T) {
	redirectPasswdFile(t)

	if err := SetPassword("to-be-removed"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !IsSet() {
		t.Fatal("IsSet() = false after SetPassword, want true")
	}
	if err := Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if IsSet() {
		t.Fatal("IsSet() = true after Remove, want false")
	}
}

func TestRemoveAbsentIsNil(t *testing.T) {
	redirectPasswdFile(t)

	if IsSet() {
		t.Fatal("IsSet() = true with no password file, want false")
	}
	if err := Remove(); err != nil {
		t.Errorf("Remove() when absent = %v, want nil", err)
	}
}
