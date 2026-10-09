package linediff

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestMyersRebuildsTheText(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		a := make([]string, r.IntN(30))
		for i := range a {
			a[i] = fmt.Sprint(r.IntN(6))
		}
		b := slices.Clone(a)
		for range r.IntN(8) {
			switch at := r.IntN(len(b) + 1); r.IntN(3) {
			case 0:
				b = slices.Insert(b, at, fmt.Sprint(r.IntN(6)))
			case 1:
				if at < len(b) {
					b = slices.Delete(b, at, at+1)
				}
			default:
				if at < len(b) {
					b[at] = "x"
				}
			}
		}
		ops, ok := myers(a, b)
		if !ok {
			t.Fatal("gave up")
		}
		var got []string
		i, j := 0, 0
		for _, o := range ops {
			switch o {
			case opEqual:
				if a[i] != b[j] {
					t.Fatalf("equal of %q and %q", a[i], b[j])
				}
				got = append(got, a[i])
				i, j = i+1, j+1
			case opDelete:
				i++
			case opInsert:
				got = append(got, b[j])
				j++
			}
		}
		if i != len(a) || !slices.Equal(got, b) {
			t.Fatalf("%v → %v: rebuilt %v", a, b, got)
		}
		if marks := Lines(a, b); len(marks) != len(b) {
			t.Fatalf("%d marks of %d lines", len(marks), len(b))
		}
	}
}

func TestLines(t *testing.T) {
	base := []string{"a", "b", "c", "d", "e"}
	for _, tc := range []struct {
		b    []string
		want []Kind
	}{
		{[]string{"a", "b", "c", "d", "e"}, []Kind{0, 0, 0, 0, 0}},
		{[]string{"a", "B", "c", "d", "e"}, []Kind{0, Modified, 0, 0, 0}},
		{[]string{"a", "b", "x", "y", "c", "d", "e"}, []Kind{0, 0, Added, Added, 0, 0, 0}},
		{[]string{"a", "d", "e"}, []Kind{0, Deleted, 0}},
		{[]string{"a", "b", "c"}, []Kind{0, 0, DeletedBelow}},
		{[]string{"a", "X", "Y", "Z", "e"}, []Kind{0, Modified, Modified, Modified, 0}},
	} {
		if got := Lines(base, tc.b); !slices.Equal(got, tc.want) {
			t.Errorf("%v: %v, want %v", tc.b, got, tc.want)
		}
	}
}

func BenchmarkLinesLargeFewEdits(b *testing.B) {
	a := make([]string, 100_000)
	for i := range a {
		a[i] = fmt.Sprintf("line %d", i)
	}
	c := slices.Clone(a)
	c[500], c[50_000] = "x", "y"
	c = slices.Insert(c, 70_000, "new")
	for b.Loop() {
		Lines(a, c)
	}
}
