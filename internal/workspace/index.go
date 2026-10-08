package workspace

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxIndexed is the most files the index holds.
const MaxIndexed = 200_000

// Index lists the files under root, relative to it with slashes, leaving
// out the directories that Skipped names. It runs in the caller's
// goroutine; call it from another.
func Index(root string) []string {
	var files []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && path != root {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != root && Skipped(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if Hidden(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		if len(files) >= MaxIndexed {
			return fs.SkipAll
		}
		return nil
	})
	return files
}

// Match is a file that a query matches: its index in the list matched,
// its score, and the byte offsets of its matched characters.
type Match struct {
	Index     int
	Score     int
	Positions []int
}

// Fuzzy finds the files that query matches, its characters in order,
// best first: matches in the file's name, at the starts of words, and of
// consecutive characters, rank above those scattered in its directories.
// It returns at most limit.
func Fuzzy(files []string, query string, limit int) []Match {
	query = strings.ToLower(strings.ReplaceAll(query, " ", ""))
	if query == "" {
		n := min(limit, len(files))
		out := make([]Match, n)
		for i := range n {
			out[i] = Match{Index: i}
		}
		return out
	}
	var out []Match
	q := []rune(query)
	for i, f := range files {
		score, pos, ok := score(f, q)
		if !ok {
			continue
		}
		out = append(out, Match{Index: i, Score: score, Positions: pos})
	}
	slices.SortStableFunc(out, func(a, b Match) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return len(files[a.Index]) - len(files[b.Index])
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// score matches q in s, preferring the characters of the base name, then
// those at word starts and following one another.
func score(s string, q []rune) (int, []int, bool) {
	// Quickly reject: every rune of q, in order.
	j := 0
	for _, r := range strings.ToLower(s) {
		if j < len(q) && r == q[j] {
			j++
		}
	}
	if j < len(q) {
		return 0, nil, false
	}

	base := strings.LastIndexByte(s, '/') + 1
	// Match greedily from the end, so that the base name takes what it can,
	// then score the positions.
	pos := make([]int, len(q))
	k := len(q) - 1
	for i := len(s); i > 0 && k >= 0; {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
		if unicode.ToLower(r) == q[k] {
			pos[k] = i
			k--
		}
	}
	if k >= 0 {
		return 0, nil, false
	}
	// Pull matches right after an earlier match next to it, keeping runs
	// together: "mai" in "src/main.go" takes m-a-i of main.
	total := 0
	for n, p := range pos {
		total += 10
		if p >= base {
			total += 8
		}
		if p == 0 || isBoundary(s, p) {
			total += 12
		}
		if n > 0 && pos[n-1]+utf8.RuneLen(q[n-1]) == p {
			total += 15
		}
		if n == 0 && p == base {
			total += 10
		}
	}
	total -= (len(s) - base) / 4 // shorter names first
	total -= strings.Count(s, "/")
	return total, pos, true
}

func isBoundary(s string, i int) bool {
	prev, _ := utf8.DecodeLastRuneInString(s[:i])
	cur, _ := utf8.DecodeRuneInString(s[i:])
	switch prev {
	case '/', '_', '-', '.', ' ':
		return true
	}
	return unicode.IsLower(prev) && unicode.IsUpper(cur)
}
