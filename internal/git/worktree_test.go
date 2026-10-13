package git

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeForBranch(t *testing.T) {
	r, git := repo(t)
	path := filepath.Join(t.TempDir(), "work tree\nquoted\"name")
	git("worktree", "add", "-b", "topic", path)
	// Git canonicalizes the temporary directory's symlink on macOS.
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.WorktreeForBranch("topic")
	if err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
	got, err = r.WorktreeForBranch("missing")
	if err != nil || got != "" {
		t.Fatalf("missing: %q, %v", got, err)
	}
	var occupied *WorktreeInUse
	if switchErr := r.Switch("topic", false); !errors.As(switchErr, &occupied) || occupied.Path != want {
		t.Fatalf("switch occupied branch: %v", switchErr)
	}
	current, err := r.run(nil, "symbolic-ref", "--short", "HEAD")
	if err != nil || strings.TrimSpace(string(current)) != "main" {
		t.Fatalf("original branch changed: %q, %v", current, err)
	}
}
