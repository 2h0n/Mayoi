// Package git runs the git command for the editor: the state of a work
// tree's files, the text of a file at HEAD or in the index, staging and
// committing, branches, blame and the remotes.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo is a Git work tree. Root is spelled as the folder it was found
// from, so that its paths compare with the editor's, which may go through
// a symlink that Git resolves.
type Repo struct {
	Root string
	// Dir is the work tree's own Git folder, which has its index and its
	// HEAD; Common the folder its worktrees share, which has the refs.
	Dir, Common string
}

// Open finds the work tree that dir is in.
func Open(dir string) (*Repo, error) {
	out, err := run(dir, nil, "rev-parse", "--show-cdup", "--absolute-git-dir", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 3 {
		return nil, errors.New("not a work tree")
	}
	r := &Repo{Root: filepath.Clean(filepath.Join(dir, lines[0])), Dir: lines[1], Common: lines[2]}
	if !filepath.IsAbs(r.Common) {
		r.Common = filepath.Join(dir, r.Common)
	}
	return r, nil
}

// Error is a git command that failed, with what it said.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	if e.Stderr != "" {
		return e.Stderr
	}
	return fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// run runs git in dir, with stdin, and returns what it wrote. It never
// asks for a password: there is no terminal to ask in.
func run(dir string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes")
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), &Error{Args: args, Stderr: strings.TrimSpace(errb.String()), Err: err}
	}
	return out.Bytes(), nil
}

func (r *Repo) run(stdin []byte, args ...string) ([]byte, error) { return run(r.Root, stdin, args...) }

// Rel returns path relative to the root, with slashes, as Git names it.
func (r *Repo) Rel(path string) string {
	rel, err := filepath.Rel(r.Root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// Abs returns the path of a file Git names relative to the root.
func (r *Repo) Abs(rel string) string { return filepath.Join(r.Root, filepath.FromSlash(rel)) }

// Contains reports whether path is in the work tree.
func (r *Repo) Contains(path string) bool {
	return path == r.Root || strings.HasPrefix(path, r.Root+string(filepath.Separator))
}

// Entry is a file whose state differs from HEAD, or that Git does not
// track. X is its state in the index against HEAD and Y in the work tree
// against the index, as git status says them: '.' unchanged, 'M', 'A',
// 'D', 'R', 'C', 'T', 'U' for a conflict, and '?' for both of a file not
// tracked.
type Entry struct {
	Path string // absolute
	Orig string // where a renamed file was, absolute
	X, Y byte
}

// Untracked reports whether Git does not track the file.
func (e Entry) Untracked() bool { return e.X == '?' }

// Conflicted reports whether a merge left the file in conflict.
func (e Entry) Conflicted() bool {
	return e.X == 'U' || e.Y == 'U' || e.X == 'A' && e.Y == 'A' || e.X == 'D' && e.Y == 'D'
}

// Staged reports whether the index has changes of the file.
func (e Entry) Staged() bool { return !e.Untracked() && !e.Conflicted() && e.X != '.' }

// Unstaged reports whether the work tree has changes of the file the
// index has not.
func (e Entry) Unstaged() bool { return !e.Untracked() && !e.Conflicted() && e.Y != '.' }

// Letter is how the file shows in a list: as its work tree is, or else
// as its index is.
func (e Entry) Letter() byte {
	switch {
	case e.Conflicted():
		return 'C'
	case e.Untracked():
		return 'U'
	case e.Y != '.':
		return e.Y
	}
	return e.X
}

// Status is the state of the work tree: its branch and the files that
// differ.
type Status struct {
	Branch   string // "" when HEAD is detached
	Head     string // the commit checked out, "" before the first
	Upstream string
	Ahead    int
	Behind   int
	Entries  []Entry
}

// Status reads the state of the work tree.
func (r *Repo) Status() (*Status, error) {
	out, err := r.run(nil, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parseStatus(r, out), nil
}

func parseStatus(r *Repo, out []byte) *Status {
	st := &Status{}
	recs := strings.Split(string(out), "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if len(rec) < 2 {
			continue
		}
		switch rec[0] {
		case '#':
			key, val, _ := strings.Cut(rec[2:], " ")
			switch key {
			case "branch.oid":
				if val != "(initial)" {
					st.Head = val
				}
			case "branch.head":
				if val != "(detached)" {
					st.Branch = val
				}
			case "branch.upstream":
				st.Upstream = val
			case "branch.ab":
				if a, b, ok := strings.Cut(val, " "); ok {
					st.Ahead, _ = strconv.Atoi(strings.TrimPrefix(a, "+"))
					st.Behind, _ = strconv.Atoi(strings.TrimPrefix(b, "-"))
				}
			}
		case '1', '2', 'u':
			// The fields before the path: 8 of an ordinary file, 9 of a
			// renamed one, 10 of a conflict.
			n := map[byte]int{'1': 8, '2': 9, 'u': 10}[rec[0]]
			f := strings.SplitN(rec, " ", n+1)
			if len(f) < n+1 || len(f[1]) != 2 {
				continue
			}
			e := Entry{Path: r.Abs(f[n]), X: f[1][0], Y: f[1][1]}
			if rec[0] == '2' && i+1 < len(recs) {
				i++
				e.Orig = r.Abs(recs[i])
			}
			st.Entries = append(st.Entries, e)
		case '?':
			st.Entries = append(st.Entries, Entry{Path: r.Abs(rec[2:]), X: '?', Y: '?'})
		}
	}
	return st
}

// Show returns the text of path at rev: "HEAD", or "" for the index. ok
// is false when it has no such file.
func (r *Repo) Show(rev, path string) (text string, ok bool, err error) {
	out, err := r.run(nil, "show", rev+":"+r.Rel(path))
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) {
			for _, s := range []string{"exists on disk, but not in", "does not exist in", "invalid object name", "bad revision"} {
				if strings.Contains(ge.Stderr, s) {
					return "", false, nil
				}
			}
		}
		return "", false, err
	}
	return strings.ReplaceAll(string(out), "\r\n", "\n"), true, nil
}

func (r *Repo) rels(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = r.Rel(p)
	}
	return out
}

// Stage puts the files as they are in the index, removed ones too.
func (r *Repo) Stage(paths ...string) error {
	_, err := r.run(nil, append([]string{"add", "-A", "--"}, r.rels(paths)...)...)
	return err
}

// Unstage takes the files' changes out of the index, which has them as
// HEAD does again.
func (r *Repo) Unstage(paths ...string) error {
	if _, err := r.run(nil, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		// Before the first commit there is no HEAD to restore from.
		_, err := r.run(nil, append([]string{"rm", "--cached", "-r", "-q", "--"}, r.rels(paths)...)...)
		return err
	}
	_, err := r.run(nil, append([]string{"restore", "--staged", "--"}, r.rels(paths)...)...)
	return err
}

// Discard makes the tracked files in the work tree as the index has them.
func (r *Repo) Discard(paths ...string) error {
	_, err := r.run(nil, append([]string{"restore", "--worktree", "--"}, r.rels(paths)...)...)
	return err
}

// StageText puts text in the index as the file at path.
func (r *Repo) StageText(path, text string) error {
	mode := "100644"
	if out, err := r.run(nil, "ls-files", "-s", "--", r.Rel(path)); err == nil {
		if f := strings.Fields(string(out)); len(f) > 0 {
			mode = f[0]
		}
	}
	sha, err := r.run([]byte(text), "hash-object", "-w", "--stdin", "--path", r.Rel(path))
	if err != nil {
		return err
	}
	_, err = r.run(nil, "update-index", "--add", "--cacheinfo", mode+","+strings.TrimSpace(string(sha))+","+r.Rel(path))
	return err
}

// Commit commits the index with message, or amends the last commit.
func (r *Repo) Commit(message string, amend bool) error {
	args := []string{"commit", "-F", "-"}
	if amend {
		args = append(args, "--amend")
	}
	_, err := r.run([]byte(message), args...)
	return err
}

// LastMessage returns the message of the last commit, "" before the
// first.
func (r *Repo) LastMessage() string {
	out, err := r.run(nil, "log", "-1", "--format=%B")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Branches returns the local branches, by name.
func (r *Repo) Branches() ([]string, error) {
	out, err := r.run(nil, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	var bs []string
	for _, b := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if b != "" {
			bs = append(bs, b)
		}
	}
	return bs, nil
}

// Switch checks out a branch, making it first when create is set.
func (r *Repo) Switch(branch string, create bool) error {
	args := []string{"switch", branch}
	if create {
		args = []string{"switch", "-c", branch}
	}
	_, err := r.run(nil, args...)
	return err
}

// Blame is who last changed a line, and when.
type Blame struct {
	Commit  string // "" for a line not committed yet
	Author  string
	Time    time.Time
	Summary string
}

// BlameLine blames line (from 0) of the file at path, its text being
// text, which may have changes not saved.
func (r *Repo) BlameLine(path string, line int, text string) (Blame, error) {
	n := strconv.Itoa(line + 1)
	out, err := r.run([]byte(text), "blame", "--porcelain", "-L", n+","+n, "--contents", "-", "--", r.Rel(path))
	if err != nil {
		return Blame{}, err
	}
	return parseBlame(out), nil
}

func parseBlame(out []byte) Blame {
	var b Blame
	for i, l := range strings.Split(string(out), "\n") {
		if i == 0 {
			if f := strings.Fields(l); len(f) > 0 && strings.Trim(f[0], "0") != "" {
				b.Commit = f[0]
			}
			continue
		}
		key, val, _ := strings.Cut(l, " ")
		switch key {
		case "author":
			b.Author = val
		case "author-time":
			if s, err := strconv.ParseInt(val, 10, 64); err == nil {
				b.Time = time.Unix(s, 0)
			}
		case "summary":
			b.Summary = val
		}
	}
	return b
}

// Fetch fetches from the remotes.
func (r *Repo) Fetch() error {
	_, err := r.run(nil, "fetch", "--all", "--prune")
	return err
}

// Pull merges the upstream's commits, when that is a fast-forward.
func (r *Repo) Pull() error {
	_, err := r.run(nil, "pull", "--ff-only")
	return err
}

// Push pushes the branch to its upstream, or to a remote of the same name
// that becomes its upstream when it has none.
func (r *Repo) Push(branch string, hasUpstream bool) error {
	if hasUpstream {
		_, err := r.run(nil, "push")
		return err
	}
	out, err := r.run(nil, "remote")
	if err != nil {
		return err
	}
	remotes := strings.Fields(string(out))
	if len(remotes) == 0 {
		return errors.New("the repository has no remote to push to")
	}
	remote := remotes[0]
	for _, rm := range remotes {
		if rm == "origin" {
			remote = rm
		}
	}
	_, err = r.run(nil, "push", "-u", remote, branch)
	return err
}

// NeedsAuth reports whether a remote command failed for want of a login,
// which a terminal can ask for.
func NeedsAuth(err error) bool {
	var ge *Error
	if !errors.As(err, &ge) {
		return false
	}
	s := strings.ToLower(ge.Stderr)
	for _, k := range []string{"terminal prompts disabled", "could not read username", "could not read password",
		"authentication failed", "permission denied", "host key verification failed"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// Init makes dir a Git repository.
func Init(dir string) error {
	_, err := run(dir, nil, "init")
	return err
}
