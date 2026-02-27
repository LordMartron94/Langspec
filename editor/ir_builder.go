package editor

import (
	"cmp"
	"fmt"
	"foundation/bytes"
	"foundation/hash"
	"lexarch"
	"slices"
	"syntaxa"
)

var xxh3Hasher = hash.XXH3HasherCreateWithSeed(42)

/*
EditorIRConstruct builds an EditorIR from a grammar package and lexical rules.

Grammar tokens are registered first; then any tokens with a role in skipRoles (e.g. whitespace, comments)
are added and recorded as global trivia. Token definitions are built from the combined token list and
lexical rules. Contexts are derived from the grammar: each context boundary (rule root or nest) becomes
an EditorContext with boundaries and valid paths. The entry rule determines the start context.

Panics if the grammar package's entry rule is missing.
*/
func EditorIRConstruct[TObservation cmp.Ordered, TTokenMeta, TContextMeta any, TToken, TTokenRole comparable](
	grammarPackage syntaxa.GrammarPackage[TToken],
	lexicalRules []lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	tokenMetadataProvider MetadataProvider[TTokenMeta],
	contextMetadataProvider MetadataProvider[TContextMeta],
	skipRoles []TTokenRole,
) *EditorIR[TObservation, TTokenMeta, TContextMeta, TToken] {

	tokenToID := make(map[TToken]TokenID)
	var tokensToDefine []TToken
	var globalTrivia []TokenID

	// 1a. Register all grammar tokens (Syntax)
	for _, t := range grammarPackage.TokensUsed {
		id := TokenID(len(tokenToID))
		tokenToID[t] = id
		tokensToDefine = append(tokensToDefine, t)
	}

	// 1b. Register all trivia tokens (Whitespace, Comments)
	for _, rs := range lexicalRules {
		for _, rule := range rs.GetRules() {
			if slices.Contains(skipRoles, rule.Role) {
				if _, exists := tokenToID[rule.Token]; !exists {
					id := TokenID(len(tokenToID))
					tokenToID[rule.Token] = id
					tokensToDefine = append(tokensToDefine, rule.Token)
					globalTrivia = append(globalTrivia, id)
				}
			}
		}
	}

	// 2. Build Token Definitions using the combined list
	tokenDefinitions := buildTokenDefinitions[TObservation, TTokenMeta](
		tokensToDefine, // Replaced grammarPackage.TokensUsed
		tokenToID,
		lexicalRules,
	)

	contexts := make(map[ContextID]*EditorContext[TContextMeta])

	startRule, ok := grammarPackage.Rules[grammarPackage.EntryRule]
	if !ok {
		panic(fmt.Sprintf("Entry rule %v not found", grammarPackage.EntryRule))
	}

	startID := generateContextID(grammarPackage.EntryRule, *startRule.NodePath)

	for _, rule := range grammarPackage.Rules {
		buildContextsRecursive(rule, grammarPackage.Analysis, tokenToID, contexts)
	}

	return EditorIRCreate(
		tokenMetadataProvider,
		contextMetadataProvider,
		tokenDefinitions,
		globalTrivia,
		contexts,
		startID,
	)
}

func buildContextsRecursive[TToken comparable, TMeta any](
	g *syntaxa.Grammar[TToken],
	analysis *syntaxa.GrammarAnalysis[TToken],
	tokenToID map[TToken]TokenID,
	contexts map[ContextID]*EditorContext[TMeta],
) {
	if g == nil {
		return
	}

	if g.IsContextBoundary || g.Kind == syntaxa.GNest {
		ctxID := generateContextID(g.GrammarID, *g.NodePath)

		if _, exists := contexts[ctxID]; !exists {
			ctx := &EditorContext[TMeta]{
				ID:   ctxID,
				Name: string(g.GrammarID),
			}

			var contentNode *syntaxa.Grammar[TToken]

			if g.Kind == syntaxa.GNest {
				ctx.Boundaries = append(ctx.Boundaries, Boundary[TMeta]{
					OnTrigger: tokenToID[*g.CloseToken],
					IsExit:    true,
				})
				contentNode = g.Children[0]
			} else {
				contentNode = g
			}

			populateContextContent(ctx, contentNode, tokenToID)
			contexts[ctxID] = ctx
		}
	}

	for _, child := range g.Children {
		buildContextsRecursive(child, analysis, tokenToID, contexts)
	}
}

func populateContextContent[TToken comparable, TMeta any](
	ctx *EditorContext[TMeta],
	g *syntaxa.Grammar[TToken],
	tokenToID map[TToken]TokenID,
) {
	var extractPaths func(node *syntaxa.Grammar[TToken]) [][]PathElement

	extractPaths = func(node *syntaxa.Grammar[TToken]) [][]PathElement {
		if node == nil {
			return nil
		}

		// 1. Handle Subroutines (References)
		if node != g && node.IsContextBoundary {
			targetCtxID := generateContextID(node.GrammarID, *node.NodePath)

			// If the subroutine is a Nest, we MUST capture its OpenToken to trigger the ENTER
			if node.Kind == syntaxa.GNest {
				boundaryExists := false
				for _, b := range ctx.Boundaries {
					if b.TargetContext == targetCtxID {
						boundaryExists = true
						break
					}
				}

				if !boundaryExists {
					ctx.Boundaries = append(ctx.Boundaries, Boundary[TMeta]{
						OnTrigger:     tokenToID[*node.OpenToken],
						IsExit:        false,
						TargetContext: targetCtxID,
					})
				}
				// The path requires the Open Token first, THEN it enters the reference
				return [][]PathElement{{
					{Type: ElementToken, Token: tokenToID[*node.OpenToken]},
					{Type: ElementContextRef, TargetContext: targetCtxID},
				}}
			}

			// Standard non-nest subroutine
			return [][]PathElement{{
				{Type: ElementContextRef, TargetContext: targetCtxID},
			}}
		}

		// 2. Handle Inline Elements
		switch node.Kind {
		case syntaxa.GToken:
			return [][]PathElement{{
				{Type: ElementToken, Token: tokenToID[node.Token]},
			}}

		case syntaxa.GChoice:
			var paths [][]PathElement
			for _, child := range node.Children {
				paths = append(paths, extractPaths(child)...)
			}
			return paths

		case syntaxa.GConcat:
			paths := [][]PathElement{{}}
			for _, child := range node.Children {
				childPaths := extractPaths(child)
				if len(childPaths) == 0 {
					continue
				}

				var newPaths [][]PathElement
				for _, existingPath := range paths {
					for _, childPath := range childPaths {
						combined := make([]PathElement, len(existingPath)+len(childPath))
						copy(combined, existingPath)
						copy(combined[len(existingPath):], childPath)
						newPaths = append(newPaths, combined)
					}
				}
				paths = newPaths
			}
			return paths

		case syntaxa.GRepeat:
			childPaths := extractPaths(node.Children[0])
			if node.Min == 0 {
				childPaths = append(childPaths, []PathElement{}) // Allow empty
			}
			return childPaths

		case syntaxa.GOptional:
			childPaths := extractPaths(node.Children[0])
			return append(childPaths, []PathElement{}) // Allow empty

		default:
			return nil
		}
	}

	// 3. Assign Paths
	paths := extractPaths(g)
	for _, p := range paths {
		ctx.ValidPaths = append(ctx.ValidPaths, StrictSequence[TMeta]{
			ExpectedPath: p,
		})
	}
}

func buildTokenDefinitions[TObservation cmp.Ordered, TMeta any, TToken, TTokenRole comparable](
	tokens []TToken,
	tokenToID map[TToken]TokenID,
	lexicalRules []lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
) []TokenDefinition[TObservation, TMeta, TToken] {

	ruleLookup := make(map[TToken]lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole])

	for _, rs := range lexicalRules {
		for _, rule := range rs.GetRules() {
			ruleLookup[rule.Token] = rule
		}
	}

	defs := make([]TokenDefinition[TObservation, TMeta, TToken], 0, len(tokens))
	for _, t := range tokens {
		var pattern Pattern[TObservation]
		var priority int

		if rule, found := ruleLookup[t]; found {
			pattern = rule.Pattern
			priority = rule.Priority
		}

		defs = append(defs, TokenDefinition[TObservation, TMeta, TToken]{
			ID:       tokenToID[t],
			Name:     fmt.Sprintf("%v", t),
			Pattern:  pattern,
			Priority: priority,
		})
	}

	return defs
}

func generateContextID(grammarID syntaxa.GrammarID, nodePath syntaxa.NodePath) ContextID {
	bytes := bytes.StringSliceToBytes([]string{string(grammarID), string(nodePath)}, ';')
	hashed := hash.XXH3HasherHash64(xxh3Hasher, bytes)
	return ContextID(hashed)
}
