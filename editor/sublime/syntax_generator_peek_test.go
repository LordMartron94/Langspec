package sublime

import (
	"strings"
	"testing"

	"lexarch"
	"syntaxa"
)

func TestPeekAfterMatchSignature_DistinguishesGuards(t *testing.T) {
	a := peekAfterMatchSignature(nil)
	b := peekAfterMatchSignature([]syntaxa.Lookahead[lexarch.TokenKind]{{Offset: 0, Expected: 10}})
	if a == b {
		t.Fatal("expected different signatures for empty vs non-empty peek")
	}
}

func TestReportDuplicateShadowedMatches_WritesWarning(t *testing.T) {
	m := "same"
	var sb strings.Builder
	entries := []contextEntry{
		{Match: &m, Set: []string{"A"}},
		{Match: &m, Set: []string{"B"}},
	}
	reportDuplicateShadowedMatches(&sb, "ctx", entries)
	if !strings.Contains(sb.String(), "identical match") {
		t.Fatalf("expected warning, got %q", sb.String())
	}
}

func TestReportDuplicateShadowedMatches_SameStackEffectSilent(t *testing.T) {
	m := "same"
	var sb strings.Builder
	entries := []contextEntry{
		{Match: &m, Set: []string{"A"}},
		{Match: &m, Set: []string{"A"}},
	}
	reportDuplicateShadowedMatches(&sb, "ctx", entries)
	if sb.Len() != 0 {
		t.Fatalf("expected no warning, got %q", sb.String())
	}
}

func TestMergeIdenticalMatchesToBranchPoints_ST4Branch(t *testing.T) {
	m := "identical-regex"
	entries := []contextEntry{
		{Match: &m, Set: []string{"CHAR"}},
		{Match: &m, Set: []string{"SEGMENT"}},
		{Match: stringPtr("other"), Set: []string{"X"}},
	}
	used := map[string]bool{"ctx": true}
	got, extras := mergeIdenticalMatchesToBranchPoints(entries, "ctx", used)
	if len(got) != 2 {
		t.Fatalf("expected 2 top-level entries, got %d", len(got))
	}
	if got[0].BranchPoint == nil || len(got[0].Branch) != 2 || got[1].Match == nil || *got[1].Match != "other" {
		t.Fatalf("unexpected merged entries: %#v", got)
	}
	if len(extras) != 2 {
		t.Fatalf("expected 2 arm contexts, got %d", len(extras))
	}
}
