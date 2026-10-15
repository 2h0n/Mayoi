package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// connectFake connects to a host by an ssh that runs the commands on
// this machine.
func connectFake(t *testing.T) *Conn {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(fakeSSH), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := For("fake.invalid")
	t.Cleanup(Shutdown)
	c.Connect()
	deadline := time.Now().Add(30 * time.Second)
	for !c.Connected() {
		if s, err := c.State(); s == Offline && err != nil || time.Now().After(deadline) {
			t.Fatalf("could not connect: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return c
}

func TestTransfer(t *testing.T) {
	c := connectFake(t)
	local := t.TempDir()
	write := func(rel, data string, mode os.FileMode) {
		p := filepath.Join(local, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(data), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("it's a.txt", "one", 0o644)
	write("tool.sh", "#!/bin/sh\n", 0o755)
	write("dir/sub/deep.txt", "deep", 0o600)
	os.Symlink("sub/deep.txt", filepath.Join(local, "dir", "link"))

	// The host's folder, which has a.txt's name already.
	host := t.TempDir()
	os.WriteFile(filepath.Join(host, "it's a.txt"), []byte("there"), 0o644)

	var p Progress
	got, err := c.Upload([]string{filepath.Join(local, "it's a.txt"), filepath.Join(local, "tool.sh"), filepath.Join(local, "dir")}, host, &p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{host + "/it's a copy.txt", host + "/tool.sh", host + "/dir"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("uploaded to %q", got)
	}
	if p.Total() != 3+10+4 || p.Done() < p.Total() {
		t.Errorf("progress %d of %d", p.Done(), p.Total())
	}
	read := func(p string) string {
		b, _ := os.ReadFile(p)
		return string(b)
	}
	if read(host+"/it's a.txt") != "there" || read(host+"/it's a copy.txt") != "one" || read(host+"/dir/sub/deep.txt") != "deep" {
		t.Fatal("the files did not arrive")
	}
	if fi, _ := os.Stat(host + "/tool.sh"); fi == nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("tool.sh lost its mode: %v", fi)
	}
	if l, _ := os.Readlink(host + "/dir/link"); l != "sub/deep.txt" {
		t.Errorf("link = %q", l)
	}
	if left, _ := filepath.Glob(host + "/.mayoi-paste.*"); len(left) > 0 {
		t.Errorf("left %q", left)
	}

	// To a folder of its own, as for a terminal of the host.
	tmp, err := c.Upload([]string{filepath.Join(local, "tool.sh")}, "", nil)
	if err != nil || len(tmp) != 1 || read(tmp[0]) != "#!/bin/sh\n" {
		t.Fatalf("to a temporary folder: %q, %v", tmp, err)
	}
	os.RemoveAll(filepath.Dir(tmp[0]))

	// On the host, a folder twice, then into itself.
	got, err = c.CopyOnHost([]string{host + "/dir", host + "/dir"}, host, nil)
	if err != nil || len(got) != 2 || got[0] != host+"/dir copy" || got[1] != host+"/dir copy 2" {
		t.Fatalf("copied to %q, %v", got, err)
	}
	if read(host+"/dir copy 2/sub/deep.txt") != "deep" {
		t.Fatal("the copy is not whole")
	}
	if _, err := c.CopyOnHost([]string{host + "/dir"}, host+"/dir/sub", nil); err == nil {
		t.Fatal("pasted a folder into itself")
	}

	// Back to this machine.
	back := t.TempDir()
	var dp Progress
	if err := c.Download(host+"/dir", filepath.Join(back, "dir"), &dp); err != nil {
		t.Fatal(err)
	}
	if read(filepath.Join(back, "dir", "sub", "deep.txt")) != "deep" || dp.Done() == 0 {
		t.Fatal("the folder did not come back")
	}
	if l, _ := os.Readlink(filepath.Join(back, "dir", "link")); l != "sub/deep.txt" {
		t.Errorf("link back = %q", l)
	}
	if err := c.Download(host+"/tool.sh", filepath.Join(back, "t.sh"), nil); err != nil || read(filepath.Join(back, "t.sh")) != "#!/bin/sh\n" {
		t.Fatalf("file back: %v", err)
	}
	if err := c.Download(host+"/gone", filepath.Join(back, "gone"), nil); err == nil {
		t.Fatal("downloaded a file that is not there")
	}

	// Canceled, a transfer leaves nothing behind.
	stop := NewProgress()
	stop.Cancel()
	if _, err := c.Upload([]string{filepath.Join(local, "dir")}, host+"/dir/sub", stop); err != ErrCanceled {
		t.Fatalf("canceled upload: %v", err)
	}
	if _, err := os.Stat(host + "/dir/sub/dir"); err == nil {
		t.Fatal("a canceled upload arrived")
	}
	if err := c.Download(host+"/dir", filepath.Join(back, "again"), stop); err != ErrCanceled {
		t.Fatalf("canceled download: %v", err)
	}
}
