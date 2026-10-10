package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// repo makes a work tree with a commit of a.txt and b.txt.
func repo(t *testing.T) (*Repo, func(args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@t")
	git("config", "user.name", "t")
	write(t, dir, "a.txt", "one\ntwo\nthree\n")
	write(t, dir, "b.txt", "bee\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Root != dir {
		t.Fatalf("root %q, want %q as spelled", r.Root, dir)
	}
	return r, git
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatus(t *testing.T) {
	r, git := repo(t)
	write(t, r.Root, "a.txt", "one\n2\nthree\n")
	write(t, r.Root, "new dir/c d.txt", "sea\n")
	git("mv", "b.txt", "bb.txt")
	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || st.Head == "" {
		t.Errorf("branch %q head %q", st.Branch, st.Head)
	}
	got := map[string]string{}
	for _, e := range st.Entries {
		got[r.Rel(e.Path)] = string([]byte{e.X, e.Y})
		if e.X == 'R' && r.Rel(e.Orig) != "b.txt" {
			t.Errorf("renamed from %q", e.Orig)
		}
	}
	want := map[string]string{"a.txt": ".M", "bb.txt": "R.", "new dir/c d.txt": "??"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q (all %v)", k, got[k], v, got)
		}
	}
}

func TestStageUnstageDiscardCommit(t *testing.T) {
	r, _ := repo(t)
	a := filepath.Join(r.Root, "a.txt")
	write(t, r.Root, "a.txt", "one\n2\nthree\n")
	if err := r.Stage(a); err != nil {
		t.Fatal(err)
	}
	if text, ok, _ := r.Show("", a); !ok || text != "one\n2\nthree\n" {
		t.Fatalf("index has %q", text)
	}
	if err := r.Unstage(a); err != nil {
		t.Fatal(err)
	}
	if text, _, _ := r.Show("", a); text != "one\ntwo\nthree\n" {
		t.Fatalf("index has %q after unstaging", text)
	}
	if err := r.StageText(a, "one\ntwo\nthree\nfour\n"); err != nil {
		t.Fatal(err)
	}
	if err := r.Discard(a); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(a); string(data) != "one\ntwo\nthree\nfour\n" {
		t.Fatalf("discarding gave %q", data)
	}
	if err := r.Commit("second\n\nbody", false); err != nil {
		t.Fatal(err)
	}
	if m := r.LastMessage(); m != "second\n\nbody" {
		t.Fatalf("message %q", m)
	}
	if err := r.Commit("second, amended", true); err != nil {
		t.Fatal(err)
	}
	if m := r.LastMessage(); m != "second, amended" {
		t.Fatalf("message %q", m)
	}
	if _, ok, err := r.Show("HEAD", filepath.Join(r.Root, "none.txt")); ok || err != nil {
		t.Fatalf("a file HEAD lacks: ok %v err %v", ok, err)
	}
}

func TestBranchesAndBlame(t *testing.T) {
	r, _ := repo(t)
	if err := r.Switch("topic", true); err != nil {
		t.Fatal(err)
	}
	bs, _ := r.Branches()
	if !slices.Equal(bs, []string{"main", "topic"}) {
		t.Fatalf("branches %v", bs)
	}
	a := filepath.Join(r.Root, "a.txt")
	b, err := r.BlameLine(a, 1, "one\ntwo\nthree\n")
	if err != nil || b.Commit == "" || b.Author != "t" || b.Summary != "first" {
		t.Fatalf("blame %+v %v", b, err)
	}
	b, err = r.BlameLine(a, 1, "one\nTWO\nthree\n")
	if err != nil || b.Commit != "" {
		t.Fatalf("blame of a line changed %+v %v", b, err)
	}
}

func TestPushPullFetch(t *testing.T) {
	r, git := repo(t)
	remote := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	git("remote", "add", "origin", remote)
	if err := r.Push("main", false); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Status()
	if st.Upstream != "origin/main" || st.Ahead != 0 {
		t.Fatalf("upstream %q ahead %d", st.Upstream, st.Ahead)
	}
	write(t, r.Root, "a.txt", "changed\n")
	r.Stage(filepath.Join(r.Root, "a.txt"))
	r.Commit("third", false)
	if st, _ = r.Status(); st.Ahead != 1 {
		t.Fatalf("ahead %d", st.Ahead)
	}
	if err := r.Push("main", true); err != nil {
		t.Fatal(err)
	}
	if err := r.Fetch(); err != nil {
		t.Fatal(err)
	}
	if err := r.Pull(); err != nil {
		t.Fatal(err)
	}
	if st, _ = r.Status(); st.Ahead != 0 || st.Behind != 0 {
		t.Fatalf("ahead %d behind %d", st.Ahead, st.Behind)
	}
	git("remote", "set-url", "origin", "https://127.0.0.1:1/none.git")
	if err := r.Fetch(); err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Fatalf("fetch from nowhere: %v", err)
	}
}
