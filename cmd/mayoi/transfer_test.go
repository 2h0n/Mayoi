package main

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemotePaste: files of the Finder paste into a remote folder, a
// remote file copied in the explorer pastes on its host and down to this
// machine, and the status bar shows the transfer while it runs.
func TestRemotePaste(t *testing.T) {
	hs, c, dir := remoteHarness(t)
	dir = path.Clean(dir)
	a := hs.a
	read := func(rp string) string {
		b, err := c.ReadFile(rp, 0)
		if err != nil {
			return "error: " + err.Error()
		}
		return string(b)
	}

	// Up from the Finder, into the workspace.
	local := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(local, []byte("from the mac\n"), 0o644)
	hs.h.clipboard = []string{local}
	a.pasteFiles(a.root)
	if !strings.HasPrefix(a.transferText(), "Uploading notes.txt to "+c.Host) {
		t.Errorf("status %q", a.transferText())
	}
	hs.untilLong("the upload", func() bool { return len(a.transfers) == 0 && hs.tt.HasText("notes.txt") })
	if got := read(dir + "/notes.txt"); got != "from the mac\n" {
		t.Fatalf("on the host: %q", got)
	}
	if got := a.files.Item(a.fileRow); got != hs.path("notes.txt") {
		t.Errorf("chosen %q", got)
	}

	// A remote file copied in the explorer: its path on the clipboard, and
	// pasted into a folder of the host, a copy there.
	hs.h.clipboard = nil
	a.copyFiles(hs.path("main.go"))
	if got := hs.tt.Clipboard(); got != dir+"/main.go" {
		t.Errorf("clipboard text %q", got)
	}
	a.pasteFiles(a.root)
	hs.untilLong("the copy on the host", func() bool { return len(a.transfers) == 0 && hs.tt.HasText("main copy.go") })
	if got := read(dir + "/main copy.go"); !strings.HasPrefix(got, "package main") {
		t.Fatalf("copy: %q", got)
	}

	// Down to a folder of this machine.
	down := t.TempDir()
	got, err := transferInto(a.clipboardSources(), down, nil)
	if err != nil || len(got) != 1 || got[0] != filepath.Join(down, "main.go") {
		t.Fatalf("downloaded to %q, %v", got, err)
	}
	if b, _ := os.ReadFile(got[0]); !strings.HasPrefix(string(b), "package main") {
		t.Fatalf("downloaded %q", b)
	}
	// A folder, twice: named as a copy the second time.
	for range 2 {
		if _, err := transferInto([]string{hs.path("src")}, down, nil); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(down, "src copy", "a.txt")); string(b) != "needle here\n" {
		t.Fatalf("folder copy: %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(down, ".mayoi-paste-*")); len(left) > 0 {
		t.Errorf("left %q", left)
	}

	// Once the clipboard has other text, nothing pastes.
	hs.tt.SetClipboard("something else")
	if srcs := a.clipboardSources(); srcs != nil {
		t.Errorf("sources %q", srcs)
	}

	// For a terminal of the host: copies in a temporary folder of its
	// own, and the host's own files where they are.
	got, err = toHost(c, []string{local, hs.path("main.go")}, "", nil)
	if err != nil || len(got) != 2 || got[1] != dir+"/main.go" || read(got[0]) != "from the mac\n" {
		t.Fatalf("toHost = %q, %v", got, err)
	}
	c.Remove(filepath.Dir(got[0]))
}

func TestTransferLabel(t *testing.T) {
	for _, tc := range []struct {
		srcs []string
		dir  string
		want string
	}{
		{[]string{"/a/x.txt"}, "/ssh:box/srv", "Uploading x.txt to box…"},
		{[]string{"/a/x.txt", "/a/y"}, "/ssh:box/srv", "Uploading 2 items to box…"},
		{[]string{"/ssh:box/srv/x.txt"}, "/ssh:box/srv/b", "Copying x.txt on box…"},
		{[]string{"/ssh:other/x.txt"}, "/ssh:box/srv", "Uploading x.txt to box…"},
		{[]string{"/ssh:box/srv/x.txt"}, "/Users/me", "Downloading x.txt…"},
	} {
		if got := transferLabel(tc.srcs, tc.dir); got != tc.want {
			t.Errorf("transferLabel(%q, %q) = %q", tc.srcs, tc.dir, got)
		}
	}
	if byteSize(999) != "999 B" || byteSize(12_345) != "12 KB" || byteSize(3_400_000) != "3.4 MB" {
		t.Errorf("byteSize: %s %s %s", byteSize(999), byteSize(12_345), byteSize(3_400_000))
	}
}
