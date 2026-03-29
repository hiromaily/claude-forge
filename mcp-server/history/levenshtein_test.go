package history

import (
	"testing"
)

func TestLevenshteinRatio_BothEmpty(t *testing.T) {
	t.Parallel()
	got := levenshteinRatio("", "")
	if got != 0.0 {
		t.Errorf("levenshteinRatio(\"\", \"\") = %v, want 0.0", got)
	}
}

func TestLevenshteinRatio_OneEmpty(t *testing.T) {
	t.Parallel()
	got := levenshteinRatio("", "abc")
	if got != 1.0 {
		t.Errorf("levenshteinRatio(\"\", \"abc\") = %v, want 1.0", got)
	}

	got2 := levenshteinRatio("abc", "")
	if got2 != 1.0 {
		t.Errorf("levenshteinRatio(\"abc\", \"\") = %v, want 1.0", got2)
	}
}

func TestLevenshteinRatio_NearIdentical(t *testing.T) {
	t.Parallel()
	// "error handling" vs "error handlng" — one character dropped
	got := levenshteinRatio("error handling", "error handlng")
	if got >= 0.3 {
		t.Errorf("levenshteinRatio(\"error handling\", \"error handlng\") = %v, want < 0.3", got)
	}
}

func TestLevenshteinRatio_Identical(t *testing.T) {
	t.Parallel()
	got := levenshteinRatio("hello", "hello")
	if got != 0.0 {
		t.Errorf("levenshteinRatio(\"hello\", \"hello\") = %v, want 0.0", got)
	}
}

func TestLevenshteinRatio_TotallyDifferent(t *testing.T) {
	t.Parallel()
	// "abc" vs "xyz" — all different, edit distance = 3, max len = 3 → ratio = 1.0
	got := levenshteinRatio("abc", "xyz")
	if got != 1.0 {
		t.Errorf("levenshteinRatio(\"abc\", \"xyz\") = %v, want 1.0", got)
	}
}

func TestLevenshteinRatio_Symmetry(t *testing.T) {
	t.Parallel()
	a := "error handling"
	b := "error handlng"
	if levenshteinRatio(a, b) != levenshteinRatio(b, a) {
		t.Errorf("levenshteinRatio is not symmetric for %q and %q", a, b)
	}
}

func TestLevenshteinRatio_InRange(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{"foo", "bar"},
		{"a", "ab"},
		{"kitten", "sitting"},
		{"saturday", "sunday"},
	}
	for _, p := range pairs {
		got := levenshteinRatio(p[0], p[1])
		if got < 0.0 || got > 1.0 {
			t.Errorf("levenshteinRatio(%q, %q) = %v, want value in [0.0, 1.0]", p[0], p[1], got)
		}
	}
}
