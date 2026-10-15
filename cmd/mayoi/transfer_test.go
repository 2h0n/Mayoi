package main

import (
	"fmt"
	"net"
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

	// Upload… asks for files, and Download… for a folder.
	hs.h.chosen = []string{local}
	a.askUpload(hs.path("src"))
	hs.untilLong("Upload…", func() bool { return len(a.transfers) == 0 && read(dir+"/src/notes.txt") == "from the mac\n" })
	into := t.TempDir()
	hs.h.chosen = []string{into}
	a.askDownload(hs.path("src"))
	hs.untilLong("Download…", func() bool { return len(a.transfers) == 0 })
	if b, _ := os.ReadFile(filepath.Join(into, "src", "notes.txt")); string(b) != "from the mac\n" {
		t.Fatalf("downloaded %q", b)
	}

	// A transfer canceled says so, and leaves nothing.
	a.pasteFiles(a.root) // nothing: the clipboard has other text
	hs.h.clipboard = []string{local}
	a.pasteFiles(hs.path("src"))
	a.transfers[0].progress.Cancel()
	hs.untilLong("the cancel", func() bool { return len(a.transfers) == 0 })
	if got := read(dir + "/src/notes copy.txt"); !strings.HasPrefix(got, "error") {
		t.Errorf("a canceled paste arrived: %q", got)
	}
	hs.h.clipboard = nil

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

// TestShotTransfer shoots the status bar of a transfer running.
func TestShotTransfer(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	hs, _, _ := remoteHarness(t)
	a := hs.a
	x := a.startTransfer("Uploading 3 items to fake.invalid…")
	x.progress.Cancel()
	a.startTransfer("Downloading build…")
	hs.settle()
	hs.shot("transfer-status")
}

// TestForwardPorts: a link to a port of the host forwards it, and opens
// at the port of this machine that forwards to it; the status bar shows
// the ports forwarded.
func TestForwardPorts(t *testing.T) {
	hs, c, _ := remoteHarness(t)
	a := hs.a
	// A port in use here, as by the server itself when the host is this
	// machine: the forward takes another.
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	link := fmt.Sprintf("http://0.0.0.0:%d/app?x=1#top", port)
	got, ok := a.localLink(link)
	local := c.LocalPort(port)
	if !ok || local == 0 || local == port || got != fmt.Sprintf("http://localhost:%d/app?x=1#top", local) {
		t.Fatalf("localLink = %q, %v (local %d)", got, ok, local)
	}
	if again, _ := a.localLink(fmt.Sprintf("http://localhost:%d/", port)); again != fmt.Sprintf("http://localhost:%d/", local) {
		t.Errorf("again %q", again)
	}
	if len(c.Forwards()) != 1 {
		t.Errorf("forwards %d", len(c.Forwards()))
	}
	for _, other := range []string{"https://example.com:8080/", "http://localhost/", "file:///x", "http://10.0.0.1:3000/"} {
		if _, ok := a.localLink(other); ok {
			t.Errorf("%q forwarded", other)
		}
	}
	hs.settle()
	if want := fmt.Sprintf("%d → %d", port, local); !hs.tt.HasText(want) {
		t.Errorf("status bar has no %q", want)
	}
	hs.shot("ports-status")
	for _, f := range c.Forwards() {
		f.Close()
	}
	hs.settle()
	if hs.tt.HasText(fmt.Sprintf("%d → %d", port, local)) {
		t.Error("the status bar still shows the port")
	}
}

// TestRemoteReadAhead: the folders of a remote folder are read as it is,
// and open at once, with no round trip to the host.
func TestRemoteReadAhead(t *testing.T) {
	hs, c, dir := remoteHarness(t)
	a := hs.a
	src := hs.path("src")
	hs.untilLong("src read ahead", func() bool { return a.tree.ReadAhead(src) })
	a.tree.Load(src)
	if !a.tree.Loaded(src) || len(a.tree.Children(src)) != 1 {
		t.Fatalf("src did not open at once: %q", a.tree.Children(src))
	}
	// A folder read ahead a while ago shows at once, and is read again.
	if _, err := c.Run(nil, `mkdir -p "$1/src/deeper/x" && : > "$1/src/deeper/x/f"`, dir); err != nil {
		t.Fatal(err)
	}
	a.files.Open.Add(src)
	hs.untilLong("the new folder", func() bool { return hs.tt.HasText("deeper") })
	deeper := hs.path("src/deeper")
	hs.untilLong("deeper read ahead", func() bool { return a.tree.ReadAhead(deeper) })
	a.tree.Load(deeper)
	if got := a.tree.Children(deeper); len(got) != 1 || filepath.Base(got[0]) != "x" {
		t.Fatalf("deeper = %q", got)
	}
}
