package semantics

import (
	"lexarch"
	"syntaxa"
	"testing"
)

func TestClassifyChoiceOverlapForToken_partitionedGuardsResolveAllPrefixes(t *testing.T) {
	path0 := syntaxa.NodePath("0.0")
	path1 := syntaxa.NodePath("0.1")
	key0 := syntaxa.NodeKeyFromPath(path0)
	key1 := syntaxa.NodeKeyFromPath(path1)

	g0 := &syntaxa.Grammar[lexarch.TokenKind, uint32]{Kind: syntaxa.GToken, Token: 10, NodePath: &path0}
	g1 := &syntaxa.Grammar[lexarch.TokenKind, uint32]{Kind: syntaxa.GToken, Token: 10, NodePath: &path1}

	analysis := &syntaxa.GrammarAnalysis{
		Nullable: map[syntaxa.NodeKey]bool{key0: false, key1: false},
		First: map[syntaxa.NodeKey]syntaxa.TokenSet{
			key0: {10: struct{}{}},
			key1: {10: struct{}{}},
		},
		Follow: map[syntaxa.NodeKey]syntaxa.TokenSet{},
		ArmPredict: map[syntaxa.NodeKey]syntaxa.GuardedArm{
			key0: {
				First: syntaxa.TokenSet{10: struct{}{}},
				Guard: []syntaxa.Lookahead[lexarch.TokenKind]{{Offset: 0, Expected: 10}, {Offset: 1, Expected: 20}},
			},
			key1: {
				First: syntaxa.TokenSet{10: struct{}{}},
				Guard: []syntaxa.Lookahead[lexarch.TokenKind]{{Offset: 0, Expected: 10}, {Offset: 1, Expected: 99}},
			},
		},
	}

	pkg := &GrammarPackage[uint32]{TokensUsed: []lexarch.TokenKind{10, 20, 99}}

	choice := &syntaxa.Grammar[lexarch.TokenKind, uint32]{
		Kind:     syntaxa.GChoice,
		Children: []*syntaxa.Grammar[lexarch.TokenKind, uint32]{g0, g1},
	}

	if classifyChoiceOverlapForToken(pkg, analysis, choice, 10) != choiceOverlapResolved {
		t.Fatal("expected resolved when guards partition peek(1) for every enumerated prefix")
	}
}

func TestClassifyChoiceOverlapForToken_orderedWarningWhenBothMatch(t *testing.T) {
	path0 := syntaxa.NodePath("0.0")
	path1 := syntaxa.NodePath("0.1")
	key0 := syntaxa.NodeKeyFromPath(path0)
	key1 := syntaxa.NodeKeyFromPath(path1)

	g0 := &syntaxa.Grammar[lexarch.TokenKind, uint32]{Kind: syntaxa.GToken, Token: 10, NodePath: &path0}
	g1 := &syntaxa.Grammar[lexarch.TokenKind, uint32]{Kind: syntaxa.GToken, Token: 10, NodePath: &path1}

	analysis := &syntaxa.GrammarAnalysis{
		Nullable: map[syntaxa.NodeKey]bool{key0: false, key1: false},
		First: map[syntaxa.NodeKey]syntaxa.TokenSet{
			key0: {10: struct{}{}},
			key1: {10: struct{}{}},
		},
		Follow: map[syntaxa.NodeKey]syntaxa.TokenSet{},
		ArmPredict: map[syntaxa.NodeKey]syntaxa.GuardedArm{
			key0: {
				First: syntaxa.TokenSet{10: struct{}{}},
				Guard: []syntaxa.Lookahead[lexarch.TokenKind]{{Offset: 0, Expected: 10}, {Offset: 1, Expected: 20}},
			},
		},
	}

	pkg := &GrammarPackage[uint32]{TokensUsed: []lexarch.TokenKind{10, 20}}

	choice := &syntaxa.Grammar[lexarch.TokenKind, uint32]{
		Kind:     syntaxa.GChoice,
		Children: []*syntaxa.Grammar[lexarch.TokenKind, uint32]{g0, g1},
	}

	if classifyChoiceOverlapForToken(pkg, analysis, choice, 10) != choiceOverlapOrderedWarning {
		t.Fatal("expected ordered-choice warning when CharLiteral+Range matches two arms")
	}
}
