package generator

import (
	"lexarch"
	"syntaxa"
	"testing"
)

func TestBuildGenTemplatePlanRepeatedConcatVirtualTokens(t *testing.T) {
	type G = syntaxa.Grammar[lexarch.TokenKind, uint32]
	virtTok := func(tok lexarch.TokenKind) *G {
		return &G{Kind: syntaxa.GToken, Token: tok}
	}
	inner := func(a, b, c lexarch.TokenKind) *G {
		return &G{Kind: syntaxa.GConcat, Children: []*G{virtTok(a), virtTok(b), virtTok(c)}}
	}
	ruleRoot := func(body *G) *G {
		return &G{Kind: syntaxa.GConcat, IsContextBoundary: true, Children: []*G{body}}
	}
	rA := ruleRoot(inner(10, 11, 12))
	rB := ruleRoot(inner(20, 21, 22))
	pkg := &syntaxa.GrammarPackage[uint32]{
		Grammars: map[syntaxa.GrammarLabel]*G{
			"A": rA,
			"B": rB,
		},
		SortedGrammarLabels: []syntaxa.GrammarLabel{"A", "B"},
	}
	plan := buildGenTemplatePlan(pkg)
	if plan.templatesBlock == nil {
		t.Fatal("expected non-nil templatesBlock for two identical-shape subtrees")
	}
	if len(plan.replaceRoot) != 2 {
		t.Fatalf("expected 2 replacement sites, got %d", len(plan.replaceRoot))
	}
}

func TestGrammarFingerprintShapeRejectsChoice(t *testing.T) {
	type G = syntaxa.Grammar[lexarch.TokenKind, uint32]
	g := &G{
		Kind: syntaxa.GChoice,
		Children: []*G{
			{Kind: syntaxa.GToken, Token: 1},
			{Kind: syntaxa.GToken, Token: 2},
		},
	}
	_, ok := grammarFingerprintShape(g)
	if ok {
		t.Fatal("expected GChoice to be rejected for fingerprinting")
	}
}
