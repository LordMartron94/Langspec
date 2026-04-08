package editor

import (
	"cmp"
	"foundation/text"
	"langspec"
	"lexarch"
	"sort"

	"autarch/pattern"
	"syntaxa"
	"syntaxa/lowering"
)

func stackOpFromLowering(op lowering.StackOp) StackOperation {
	switch op {
	case lowering.OpMatch:
		return STACK_NONE
	case lowering.OpPush:
		return STACK_PUSH
	case lowering.OpPop:
		return STACK_POP
	case lowering.OpSet:
		return STACK_SET
	case lowering.OpSyncToken:
		return STACK_POP
	case lowering.OpSyncTokenNoConsume:
		return STACK_POP
	default:
		return STACK_NONE
	}
}

/*
EditorIRFromStateGraph builds EditorIR from a generic StateGraph.

Binds tokens to patterns via lexingRuleset, assigns TContext via
config.contextProducer, and applies overrides. Transition order from the graph
is discovery order; this layer sorts each state's transitions by lexer rule
priority. HasFallthroughPop and FallthroughPopAmount come from graph ContextMeta.
PopAmount is read from the graph; the backend may add +1 when it injects wrapper states.

Prerequisites:
- sg, config, and grammarPackage must be non-nil.

Edge cases:
- Returns (nil, nil) if sg, config, or grammarPackage is nil.
*/
func EditorIRFromStateGraph[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TNodeKind comparable,
	TLexerState comparable,
	TContext any,
](
	sg *lowering.StateGraph[TNodeKind],
	lexingRuleset *LexingRuleSet[TObservation, TToken, TTokenRole],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	grammarPackage *syntaxa.GrammarPackage[TNodeKind],
) (*EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], error) {
	if sg == nil || config == nil || grammarPackage == nil {
		return nil, nil
	}

	sanitizer := text.NewIdentifierSanitizer()

	rulesByState, allLexerStates := editorLexRulesGroupedByState(lexingRuleset)
	lexerReach, err := editorComputeLexerStackReachability(sg, rulesByState)
	if err != nil {
		return nil, err
	}

	tokenToPriority := make(map[lexarch.TokenKind]int)
	tokenToRuleIndex := make(map[lexarch.TokenKind]int)

	for i, rule := range LexingRuleSetGetRules(lexingRuleset) {
		tok := lexarch.TokenKind(rule.Token)
		if p, ok := tokenToPriority[tok]; !ok || rule.Priority > p {
			tokenToPriority[tok] = rule.Priority
		}
		if prev, ok := tokenToRuleIndex[tok]; !ok || i < prev {
			tokenToRuleIndex[tok] = i
		}
	}

	stateByID := make(map[string]*EditorState[TObservation, TContext])
	for _, c := range sg.Contexts {
		sanitizedID := sanitizer.Sanitize(c.ID)
		s := &EditorState[TObservation, TContext]{ID: sanitizedID, Label: c.Label}
		stateByID[c.ID] = s

		if meta, ok := sg.ContextMeta[c.ID]; ok {
			if meta.HasOptionalContinuation {
				s.HasFallthroughPop = true
				s.FallthroughPopAmount = meta.FallthroughPopAmount
			}
		}
	}

	for _, c := range sg.Contexts {
		if meta, ok := sg.ContextMeta[c.ID]; ok && meta.ImmediatePushTargetID != "" {
			if target, exists := stateByID[meta.ImmediatePushTargetID]; exists {
				stateByID[c.ID].ImmediatePushTarget = target
			}
		}
	}

	nestLabelByContextID := make(map[string]syntaxa.GrammarLabel)
	for label, ctxID := range sg.NestBodyContextIDs {
		nestLabelByContextID[ctxID] = label
	}

	for id, s := range stateByID {
		var ownerNodeKind *TNodeKind
		if sg.ContextOwnerNodeKind != nil {
			if nk, ok := sg.ContextOwnerNodeKind[id]; ok {
				k := nk
				ownerNodeKind = &k
			}
		}
		if label, isNest := nestLabelByContextID[id]; isNest {
			edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
				IsNest: true, NestLabel: label, NodeKind: ownerNodeKind,
			}
			// Opening-production kind from lowering: manifest MetaScope on that node
			// replaces auto nestLabelToMetaScope ".body" for this wrapper (see toolchain).
			if sg.NestContentParentNodeKind != nil {
				if nk, ok := sg.NestContentParentNodeKind[id+"_content"]; ok {
					k := nk
					edCtx.NodeKind = &k
				}
			}
			s.Context = config.contextProducer(edCtx)
		} else if id == sg.RootContextID {
			s.Context = config.contextProducer(&EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{NodeKind: ownerNodeKind})
		} else {
			s.Context = config.contextProducer(&EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{NodeKind: ownerNodeKind})
		}
	}

	delimitedStates := make(map[string]*EditorState[TObservation, TContext])

	transCtxIDs := make([]string, 0, len(sg.Transitions))
	for ctxID := range sg.Transitions {
		transCtxIDs = append(transCtxIDs, ctxID)
	}
	sort.Strings(transCtxIDs)

	for _, ctxID := range transCtxIDs {
		transList := sg.Transitions[ctxID]
		s := stateByID[ctxID]
		if s == nil {
			continue
		}

		type pair struct {
			priority int
			tr       lowering.Transition[TNodeKind]
			order    int
		}
		pairs := make([]pair, 0, len(transList))
		for i, tr := range transList {
			pairs = append(pairs, pair{
				priority: tokenToPriority[tr.Token],
				tr:       tr,
				order:    i,
			})
		}

		sort.SliceStable(pairs, func(i, j int) bool {
			if pairs[i].priority != pairs[j].priority {
				return pairs[i].priority > pairs[j].priority
			}
			// For the same token, preserve consuming parse transitions before
			// recovery lookahead pops so boundary ownership stays deterministic.
			if pairs[i].tr.Token == pairs[j].tr.Token && pairs[i].tr.IsRecoveryTransition != pairs[j].tr.IsRecoveryTransition {
				return !pairs[i].tr.IsRecoveryTransition
			}
			// Absolute deterministic tie-breaker within same priority/token class.
			if pairs[i].order != pairs[j].order {
				return pairs[i].order < pairs[j].order
			}
			// Absolute deterministic tie-breaker
			return tokenToRuleIndex[pairs[i].tr.Token] < tokenToRuleIndex[pairs[j].tr.Token]
		})

		for _, p := range pairs {
			lexRule, lexOK, err := editorResolveLexingRuleForParseTransition(
				ctxID, p.tr.Token, lexerReach, rulesByState,
			)
			if err != nil {
				return nil, err
			}
			edTrs := buildEditorTransitionsFromGeneric(
				p.tr, stateByID, lexRule, lexOK, config, delimitedStates, sanitizer,
			)
			if len(edTrs) > 0 {
				s.Transitions = append(s.Transitions, edTrs...)
			}
		}

		if !s.HasFallthroughPop && len(s.Transitions) > 0 {
			s.HasFallbackInvalid = true
			s.FallbackInvalidContext = config.contextProducer(
				&EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
					IsInvalidContext: true,
				},
			)
		}
	}

	grammarTokens := collectGrammarTokens(grammarPackage.Grammars)
	ambientTransitions := buildAmbientFromStateGraph(
		lexingRuleset, grammarTokens, config, delimitedStates, sanitizer, tokenToRuleIndex,
	)

	finalStateIDs := make([]string, 0, len(stateByID))
	for id := range stateByID {
		finalStateIDs = append(finalStateIDs, id)
	}
	sort.Strings(finalStateIDs)

	allStates := make([]EditorState[TObservation, TContext], 0, len(stateByID)+len(delimitedStates))

	var rootOut EditorState[TObservation, TContext]
	for _, id := range finalStateIDs {
		s := stateByID[id]
		allStates = append(allStates, *s)
		if s.ID == sg.RootContextID {
			rootOut = *s
			break
		}
	}

	finalDelimIDs := make([]string, 0, len(delimitedStates))
	for id := range delimitedStates {
		finalDelimIDs = append(finalDelimIDs, id)
	}
	sort.Strings(finalDelimIDs)

	for _, id := range finalDelimIDs {
		allStates = append(allStates, *delimitedStates[id])
	}

	lexerModeStates := buildLexerModeStates(allLexerStates, rulesByState, config, sanitizer)

	return &EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		LanguageMeta: LanguageMeta{
			LanguageName:    grammarPackage.Name,
			LanguageVersion: grammarPackage.Version,
		},
		LanguageMachine: LanguageMachine[TObservation, TContext]{
			EditorStates:       allStates,
			RootState:          rootOut,
			AmbientTransitions: ambientTransitions,
			LexerModeStates:    lexerModeStates,
		},
	}, nil
}

func buildLexerModeStates[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
	TContext any,
](
	allLexerStates []string,
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	sanitizer *text.Sanitizer,
) []EditorState[TObservation, TContext] {
	if len(allLexerStates) == 0 {
		return nil
	}

	states := make([]EditorState[TObservation, TContext], 0, len(allLexerStates))
	for _, mode := range allLexerStates {
		label := editorLexModeLabel(mode, sanitizer)
		state := EditorState[TObservation, TContext]{
			ID:    label,
			Label: label,
		}

		for _, rule := range rulesByState[mode] {
			token := rule.Token
			role := rule.Role
			edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
				Token:     &token,
				TokenRole: &role,
			}
			tr := EditorTransition[TObservation, TContext]{
				OnPattern:    rule.Pattern,
				MatchContext: config.contextProducer(edCtx),
				Operation:    STACK_NONE,
			}
			copyLexStackFromLexingRule(&tr, rule)
			state.Transitions = append(state.Transitions, tr)
		}
		states = append(states, state)
	}
	return states
}

func editorLexModeLabel(mode string, sanitizer *text.Sanitizer) string {
	return "lex__" + sanitizer.Sanitize(mode)
}

func buildEditorTransitionsFromGeneric[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
	TContext any,
](
	tr lowering.Transition[TNodeKind],
	stateByID map[string]*EditorState[TObservation, TContext],
	lexRule LexingRule[TObservation, TToken, TTokenRole],
	lexOK bool,
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	delimitedStates map[string]*EditorState[TObservation, TContext],
	sanitizer *text.Sanitizer,
) []EditorTransition[TObservation, TContext] {

	edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		NodeKind: tr.NodeKind,
	}
	token := TToken(tr.Token)
	edCtx.Token = &token
	overrides := config.overrideProducer(edCtx)

	// Fallback to the default single transition if no overrides exist
	if len(overrides) == 0 {
		if defaultTr, ok := buildDefaultTransition(tr, edCtx, lexRule, lexOK, config, stateByID); ok {
			return []EditorTransition[TObservation, TContext]{defaultTr}
		}
		return nil
	}

	// Expand 1-to-N overrides
	var out []EditorTransition[TObservation, TContext]
	for _, override := range overrides {
		if ovrTr, ok := buildOverrideTransition(tr, edCtx, *override, lexRule, lexOK, config, stateByID, delimitedStates, sanitizer); ok {
			out = append(out, ovrTr)
		}
	}
	return out
}

func buildDefaultTransition[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
	TContext any,
](
	tr lowering.Transition[TNodeKind],
	edCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	lexRule LexingRule[TObservation, TToken, TTokenRole],
	lexOK bool,
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	stateByID map[string]*EditorState[TObservation, TContext],
) (EditorTransition[TObservation, TContext], bool) {

	if !lexOK {
		return EditorTransition[TObservation, TContext]{}, false
	}

	out := EditorTransition[TObservation, TContext]{
		OnPattern:    lexRule.Pattern,
		MatchContext: config.contextProducer(edCtx),
		Operation:    stackOpFromLowering(tr.Operation),
		Targets:      resolveTargets(tr.TargetContextIDs, stateByID),
		PopAmount:    determinePopAmount(tr),
		IsLookahead:  tr.Operation == lowering.OpSyncTokenNoConsume,
	}
	copyLexStackFromLexingRule(&out, lexRule)
	return out, true
}

func buildOverrideTransition[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
	TContext any,
](
	tr lowering.Transition[TNodeKind],
	edCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	override EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	lexRule LexingRule[TObservation, TToken, TTokenRole],
	lexOK bool,
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	stateByID map[string]*EditorState[TObservation, TContext],
	delimitedStates map[string]*EditorState[TObservation, TContext],
	sanitizer *text.Sanitizer,
) (EditorTransition[TObservation, TContext], bool) {

	var pat pattern.RegulaAST[TObservation]
	var regexPat *string

	if override.PatternRegex != nil {
		regexPat = override.PatternRegex
	} else if override.Pattern != nil {
		pat = *override.Pattern
	} else if lexOK {
		pat = lexRule.Pattern
	} else {
		return EditorTransition[TObservation, TContext]{}, false
	}

	matchCtx := config.contextProducer(edCtx)
	if override.MatchContext != nil {
		matchCtx = *override.MatchContext
	}

	if override.ForeignPayload != nil {
		out := EditorTransition[TObservation, TContext]{
			OnPattern:      pat,
			RegexPattern:   regexPat,
			MatchContext:   matchCtx,
			Captures:       override.Captures,
			Operation:      STACK_EMBED,
			ForeignPayload: override.ForeignPayload,
		}
		if lexOK {
			copyLexStackFromLexingRule(&out, lexRule)
		}
		return out, true
	}

	if override.DelimitedPayload != nil {
		bodyState := getOrCreateDelimitedState(override.DelimitedPayload, delimitedStates, sanitizer)
		out := EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			RegexPattern: regexPat,
			MatchContext: matchCtx,
			Captures:     override.Captures,
			Operation:    STACK_PUSH,
			Targets:      []*EditorState[TObservation, TContext]{bodyState},
		}
		if lexOK {
			copyLexStackFromLexingRule(&out, lexRule)
		}
		return out, true
	}

	out := EditorTransition[TObservation, TContext]{
		OnPattern:    pat,
		RegexPattern: regexPat,
		MatchContext: matchCtx,
		Captures:     override.Captures,
		Operation:    stackOpFromLowering(tr.Operation),
		Targets:      resolveTargets(tr.TargetContextIDs, stateByID),
		PopAmount:    determinePopAmount(tr),
		IsLookahead:  tr.Operation == lowering.OpSyncTokenNoConsume,
	}
	if lexOK {
		copyLexStackFromLexingRule(&out, lexRule)
	}
	return out, true
}

func resolveTargets[TObservation cmp.Ordered, TContext any](
	ids []string,
	stateByID map[string]*EditorState[TObservation, TContext],
) []*EditorState[TObservation, TContext] {
	targets := make([]*EditorState[TObservation, TContext], 0, len(ids))
	for _, id := range ids {
		if t := stateByID[id]; t != nil {
			targets = append(targets, t)
		}
	}
	return targets
}

func determinePopAmount[TNodeKind comparable](tr lowering.Transition[TNodeKind]) int {
	if tr.PopAmount > 0 {
		return tr.PopAmount
	}

	if tr.Operation == lowering.OpPop || tr.Operation == lowering.OpSyncToken || tr.Operation == lowering.OpSyncTokenNoConsume {
		return 1
	}

	return 0
}

func getOrCreateDelimitedState[
	TObservation cmp.Ordered,
	TContext any,
](
	dp *DelimitedPayload[TObservation, TContext],
	delimitedStates map[string]*EditorState[TObservation, TContext],
	sanitizer *text.Sanitizer,
) *EditorState[TObservation, TContext] {
	sanitizedID := sanitizer.Sanitize(dp.StateLabel)
	if s, ok := delimitedStates[sanitizedID]; ok {
		return s
	}
	s := &EditorState[TObservation, TContext]{
		ID:      sanitizedID,
		Label:   dp.StateLabel,
		Context: dp.BodyContext,
		Transitions: []EditorTransition[TObservation, TContext]{
			{
				OnPattern:    dp.ClosePattern,
				MatchContext: dp.CloseContext,
				Operation:    STACK_POP,
				PopAmount:    1,
			},
		},
	}
	delimitedStates[sanitizedID] = s
	return s
}

func collectGrammarTokens[TNodeKind comparable](
	grammars map[syntaxa.GrammarLabel]*syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
) map[lexarch.TokenKind]bool {
	seen := make(map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]bool)
	result := make(map[lexarch.TokenKind]bool)
	for _, g := range grammars {
		walkGrammarTokens(g, grammars, seen, result)
	}
	return result
}

func walkGrammarTokens[TNodeKind comparable](
	g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
	seen map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]bool,
	tokens map[lexarch.TokenKind]bool,
) {
	if g == nil || seen[g] {
		return
	}
	seen[g] = true
	switch g.Kind {
	case syntaxa.GToken:
		tokens[lexarch.TokenKind(g.Token)] = true
	case syntaxa.GNest:
		if g.OpenToken != nil {
			tokens[lexarch.TokenKind(*g.OpenToken)] = true
		}
		if g.CloseToken != nil {
			tokens[lexarch.TokenKind(*g.CloseToken)] = true
		}
	case syntaxa.GReference:
		target := g.ResolvedReference
		if target == nil {
			target = rules[g.ReferenceTarget]
		}
		if target != nil {
			walkGrammarTokens(target, rules, seen, tokens)
		}
	}
	for _, child := range g.Children {
		walkGrammarTokens(child, rules, seen, tokens)
	}
}

func buildAmbientFromStateGraph[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
	TContext any,
](
	lexingRuleset *LexingRuleSet[TObservation, TToken, TTokenRole],
	grammarTokens map[lexarch.TokenKind]bool,
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	delimitedStates map[string]*EditorState[TObservation, TContext],
	sanitizer *text.Sanitizer,
	tokenToRuleIndex map[lexarch.TokenKind]int,
) []EditorTransition[TObservation, TContext] {

	candidates := getSortedAmbientCandidates(lexingRuleset, grammarTokens, tokenToRuleIndex)

	var out []EditorTransition[TObservation, TContext]
	for _, entry := range candidates {
		token := entry.rule.Token
		edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{Token: &token}

		out = append(out, buildAmbientTransitionsForToken(
			entry.rule, edCtx, config, delimitedStates, sanitizer,
		)...)
	}
	return out
}

func buildAmbientTransitionsForToken[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
	TContext any,
](
	rule LexingRule[TObservation, TToken, TTokenRole],
	edCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	delimitedStates map[string]*EditorState[TObservation, TContext],
	sanitizer *text.Sanitizer,
) []EditorTransition[TObservation, TContext] {

	overrides := config.overrideProducer(edCtx)

	if len(overrides) == 0 {
		tr := EditorTransition[TObservation, TContext]{
			OnPattern:    rule.Pattern,
			MatchContext: config.contextProducer(edCtx),
			Operation:    STACK_NONE,
		}
		copyLexStackFromLexingRule(&tr, rule)
		return []EditorTransition[TObservation, TContext]{tr}
	}

	var out []EditorTransition[TObservation, TContext]
	for _, override := range overrides {
		pat := rule.Pattern
		if override.Pattern != nil {
			pat = *override.Pattern
		}

		matchCtx := config.contextProducer(edCtx)
		if override.MatchContext != nil {
			matchCtx = *override.MatchContext
		}

		if override.ForeignPayload != nil {
			t := EditorTransition[TObservation, TContext]{
				OnPattern:      pat,
				MatchContext:   matchCtx,
				Captures:       override.Captures,
				Operation:      STACK_EMBED,
				ForeignPayload: override.ForeignPayload,
			}
			copyLexStackFromLexingRule(&t, rule)
			out = append(out, t)
			continue
		}

		if override.DelimitedPayload != nil {
			bodyState := getOrCreateDelimitedState(override.DelimitedPayload, delimitedStates, sanitizer)
			t := EditorTransition[TObservation, TContext]{
				OnPattern:    pat,
				MatchContext: matchCtx,
				Captures:     override.Captures,
				Operation:    STACK_PUSH,
				Targets:      []*EditorState[TObservation, TContext]{bodyState},
			}
			copyLexStackFromLexingRule(&t, rule)
			out = append(out, t)
			continue
		}

		t := EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     override.Captures,
			Operation:    STACK_NONE,
		}
		copyLexStackFromLexingRule(&t, rule)
		out = append(out, t)
	}
	return out
}

func getSortedAmbientCandidates[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](
	lexingRuleset *LexingRuleSet[TObservation, TToken, TTokenRole],
	grammarTokens map[lexarch.TokenKind]bool,
	tokenToRuleIndex map[lexarch.TokenKind]int,
) []struct {
	rule     LexingRule[TObservation, TToken, TTokenRole]
	priority int
} {
	var candidates []struct {
		rule     LexingRule[TObservation, TToken, TTokenRole]
		priority int
	}

	bestAmbient := make(map[lexarch.TokenKind]LexingRule[TObservation, TToken, TTokenRole])
	bestAmbientIdx := make(map[lexarch.TokenKind]int)
	for i, rule := range LexingRuleSetGetRules(lexingRuleset) {
		if rule.LexerState != "" && rule.LexerState != "INITIAL" {
			// Ambient transitions map to Sublime "prototype" (global scope).
			// Non-root lexer states are context-dependent and must not be global.
			continue
		}
		tok := lexarch.TokenKind(rule.Token)
		if grammarTokens[tok] {
			continue
		}
		prev, ok := bestAmbient[tok]
		if !ok || rule.Priority > prev.Priority ||
			(rule.Priority == prev.Priority && i < bestAmbientIdx[tok]) {
			bestAmbient[tok] = rule
			bestAmbientIdx[tok] = i
		}
	}
	for _, rule := range bestAmbient {
		candidates = append(candidates, struct {
			rule     LexingRule[TObservation, TToken, TTokenRole]
			priority int
		}{rule: rule, priority: rule.Priority})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority > candidates[j].priority
		}
		ti := lexarch.TokenKind(candidates[i].rule.Token)
		tj := lexarch.TokenKind(candidates[j].rule.Token)
		if tokenToRuleIndex[ti] != tokenToRuleIndex[tj] {
			return tokenToRuleIndex[ti] < tokenToRuleIndex[tj]
		}
		return ti < tj
	})

	return candidates
}

func copyLexStackFromLexingRule[
	TObservation cmp.Ordered,
	TToken comparable,
	TTokenRole comparable,
	TContext any,
](
	tr *EditorTransition[TObservation, TContext],
	rule LexingRule[TObservation, TToken, TTokenRole],
) {
	switch rule.StackKind {
	case langspec.LexerStackOpPush:
		tr.LexPushStates = append([]string(nil), rule.StackTargets...)
	case langspec.LexerStackOpPop:
		amt := rule.StackPopAmount
		if amt < 1 {
			amt = 1
		}
		tr.LexPopAmount += amt
	case langspec.LexerStackOpSet:
		tr.LexSetStates = append([]string(nil), rule.StackTargets...)
	case langspec.LexerStackOpNone:
	}
}
