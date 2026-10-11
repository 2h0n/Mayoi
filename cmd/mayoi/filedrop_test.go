package main

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func tabNames(col *column) string {
	var out []string
	for _, t := range col.tabs {
		out = append(out, t.name())
	}
	return strings.Join(out, " ")
}

func TestDroppingFiles(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.openFile(hs.path("go.mod"), true)
	hs.settle()
	col := a.cols[0]

	// Between tabs, in the order they come, the last shown; the folder
	// opens as `mayoi <dir>` opens it.
	a.dropFiles([]string{hs.path("README.md"), hs.path("internal"), hs.path("web/style.css"), hs.path("missing.txt")}, col, 1)
	hs.settle()
	if got := tabNames(col); got != "main.go README.md style.css go.mod" {
		t.Fatalf("tabs %q", got)
	}
	if col.tab().name() != "style.css" {
		t.Errorf("shown %q", col.tab().name())
	}
	if !slices.Equal(hs.h.folders, []string{hs.path("internal")}) {
		t.Errorf("folders %q", hs.h.folders)
	}
	// One open already comes forward, and those after it follow it.
	a.dropFiles([]string{hs.path("main.go"), hs.path("web/index.html")}, col, 4)
	hs.settle()
	if got := tabNames(col); got != "main.go index.html README.md style.css go.mod" {
		t.Fatalf("tabs %q", got)
	}

	// Beside the column: a column of their own.
	a.dropFilesBeside([]string{hs.path("internal/a/a.go")}, col, true)
	hs.settle()
	if len(a.cols) != 2 || tabNames(a.cols[1]) != "a.go" || a.col != 1 {
		t.Fatalf("split: %d columns, col %d", len(a.cols), a.col)
	}
	// None that opens leaves no column behind.
	bin := hs.path("blob.bin")
	os.WriteFile(bin, []byte{0, 1, 2, 0, 0, 3}, 0o644)
	a.dropFilesBeside([]string{bin}, a.cols[1], true)
	hs.settle()
	if len(a.cols) != 2 {
		t.Errorf("%d columns after dropping a binary file beside", len(a.cols))
	}
	// Only folders beside a column split nothing.
	a.dropFilesBeside([]string{hs.path("web")}, a.cols[1], false)
	hs.settle()
	if len(a.cols) != 2 || hs.h.folders[len(hs.h.folders)-1] != hs.path("web") {
		t.Errorf("%d columns, folders %q", len(a.cols), hs.h.folders)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/me/a.go":     "/Users/me/a.go",
		"/tmp/my file.txt":   "'/tmp/my file.txt'",
		"/tmp/it's":          `'/tmp/it'\''s'`,
		"/tmp/$HOME;rm":      "'/tmp/$HOME;rm'",
		"":                   "''",
		"/tmp/a-b_c.d@e%f+g": "/tmp/a-b_c.d@e%f+g",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
