package server

import (
	"strconv"
	"strings"
	"testing"
)

func TestUnifiedDiff_Equal(t *testing.T) {
	if got := unifiedDiff("a", "b", "one\ntwo\n", "one\ntwo\n"); got != "" {
		t.Errorf("equal texts produced a diff:\n%s", got)
	}
	if got := unifiedDiff("a", "b", "", ""); got != "" {
		t.Errorf("empty texts produced a diff:\n%s", got)
	}
}

func TestUnifiedDiff_Insert(t *testing.T) {
	got := unifiedDiff("before", "after", "a\nb\n", "a\nx\nb\n")
	want := "--- before\n" +
		"+++ after\n" +
		"@@ -1,2 +1,3 @@\n" +
		" a\n" +
		"+x\n" +
		" b\n"
	if got != want {
		t.Errorf("insert diff = %q, want %q", got, want)
	}
}

func TestUnifiedDiff_Delete(t *testing.T) {
	got := unifiedDiff("before", "after", "a\nx\nb\n", "a\nb\n")
	want := "--- before\n" +
		"+++ after\n" +
		"@@ -1,3 +1,2 @@\n" +
		" a\n" +
		"-x\n" +
		" b\n"
	if got != want {
		t.Errorf("delete diff = %q, want %q", got, want)
	}
}

func TestUnifiedDiff_Change(t *testing.T) {
	got := unifiedDiff("before", "after", "a\nb\nc\n", "a\nB\nc\n")
	want := "--- before\n" +
		"+++ after\n" +
		"@@ -1,3 +1,3 @@\n" +
		" a\n" +
		"-b\n" +
		"+B\n" +
		" c\n"
	if got != want {
		t.Errorf("change diff = %q, want %q", got, want)
	}
}

// numbered returns n lines named line1..linen with a trailing newline.
func numbered(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line" + strconv.Itoa(i) + "\n")
	}
	return b.String()
}

func TestUnifiedDiff_HunkContext_SeparateHunks(t *testing.T) {
	before := numbered(20)
	after := strings.ReplaceAll(before, "line3\n", "LINE3\n")
	after = strings.ReplaceAll(after, "line15\n", "LINE15\n")

	got := unifiedDiff("current", "proposed", before, after)
	if n := strings.Count(got, "@@ "); n != 2 {
		t.Fatalf("got %d hunks, want 2 (changes 12 lines apart):\n%s", n, got)
	}
	for _, want := range []string{"-line3\n", "+LINE3\n", "-line15\n", "+LINE15\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("diff does not contain %q:\n%s", want, got)
		}
	}
	// The hunks stop at their context: line1 is context of the first change,
	// but line10 and line20 are context of neither.
	if strings.Contains(got, "line10") || strings.Contains(got, "line20") {
		t.Errorf("diff shows a line that is not context of either change:\n%s", got)
	}
}

func TestUnifiedDiff_HunkContext_MergedHunks(t *testing.T) {
	before := numbered(20)
	after := strings.ReplaceAll(before, "line5\n", "LINE5\n")
	after = strings.ReplaceAll(after, "line9\n", "LINE9\n")

	got := unifiedDiff("current", "proposed", before, after)
	if n := strings.Count(got, "@@ "); n != 1 {
		t.Fatalf("got %d hunks, want 1 (changes 4 lines apart):\n%s", n, got)
	}
	for _, want := range []string{"-line5\n", "+LINE5\n", "-line9\n", "+LINE9\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("diff does not contain %q:\n%s", want, got)
		}
	}
}

func TestUnifiedDiff_NoTrailingNewline(t *testing.T) {
	got := unifiedDiff("before", "after", "a", "a\nb")
	want := "--- before\n" +
		"+++ after\n" +
		"@@ -1,1 +1,2 @@\n" +
		" a\n" +
		"+b\n"
	if got != want {
		t.Errorf("diff = %q, want %q", got, want)
	}
}
