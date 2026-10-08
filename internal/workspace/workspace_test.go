package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func mkfiles(t *testing.T, root string, files ...string) {
	for _, f := range files {
		p := filepath.Join(root, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// post runs the tree's functions as a window's main thread would.
type mainThread chan func()

func (m mainThread) post(fn func()) { m <- fn }

func (m mainThread) wait(t *testing.T, until func() bool) {
	deadline := time.After(5 * time.Second)
	for !until() {
		select {
		case fn := <-m:
			fn()
		case <-deadline:
			t.Fatal("timed out")
		}
	}
}

func TestTreeLoadsSortsAndFollowsTheDisk(t *testing.T) {
	root := t.TempDir()
	mkfiles(t, root, "b.go", "A.md", "src/main.go", ".git/HEAD", "zeta/x")
	m := make(mainThread, 64)
	tree := NewTree(root, m.post)
	defer tree.Close()

	if kids := tree.Children(root); kids == nil || len(kids) != 0 {
		t.Fatalf("unread root: %v", kids)
	}
	tree.Load(root)
	m.wait(t, func() bool { return tree.Loaded(root) })
	var names []string
	for _, k := range tree.Children(root) {
		names = append(names, filepath.Base(k))
	}
	if want := []string{"src", "zeta", "A.md", "b.go"}; !slices.Equal(names, want) {
		t.Errorf("entries %v, want %v", names, want)
	}
	if tree.Children(filepath.Join(root, "b.go")) != nil {
		t.Error("a file has children")
	}

	mkfiles(t, root, "new.txt")
	m.wait(t, func() bool { return len(tree.Children(root)) == 5 })
	var changed []string
	tree.OnChange = func(c, r []string) { changed = append(changed, c...) }
	os.WriteFile(filepath.Join(root, "b.go"), []byte("changed"), 0o644)
	m.wait(t, func() bool { return slices.Contains(changed, filepath.Join(root, "b.go")) })
}

func TestIndexSkipsAndFuzzyRanks(t *testing.T) {
	root := t.TempDir()
	mkfiles(t, root, "src/main.go", "src/domain/manifest.json", "node_modules/m/index.js", "docs/README.md", "cmd/mayoi/main_test.go")
	files := Index(root)
	slices.Sort(files)
	if want := []string{"cmd/mayoi/main_test.go", "docs/README.md", "src/domain/manifest.json", "src/main.go"}; !slices.Equal(files, want) {
		t.Fatalf("index %v", files)
	}
	got := Fuzzy(files, "main", 10)
	if len(got) < 2 || files[got[0].Index] != "src/main.go" {
		var order []string
		for _, m := range got {
			order = append(order, files[m.Index])
		}
		t.Errorf("order %v", order)
	}
	if got := Fuzzy(files, "rdme", 10); len(got) == 0 || files[got[0].Index] != "docs/README.md" {
		t.Errorf("rdme: %v", got)
	}
	if got := Fuzzy(files, "zzz", 10); len(got) != 0 {
		t.Errorf("zzz: %v", got)
	}
}
