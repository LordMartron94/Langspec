package editor

import (
	"cmp"
	"foundation/text"
	"sort"

	"autarch/pattern"
	"lexarch"
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
	TToken comparable,
	TTokenRole comparable,
	TNodeKind comparable,
	TLexerState comparable,
	TContext any,
](
	sg *lowering.StateGraph[TToken, TNodeKind],
	lexingRuleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
) (*EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], error) {
	if sg == nil || config == nil || grammarPackage == nil {
		return nil, nil
	}

	sanitizer := text.NewIdentifierSanitizer()

	tokenToRule := make(map[TToken]lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole])
	tokenToPriority := make(map[TToken]int)
	for _, rule := range lexingRuleset.GetRules() {
		tokenToRule[rule.Token] = rule
		tokenToPriority[rule.Token] = rule.Priority
	}

	stateByID := make(map[string]*EditorState[TObservation, TContext])
	for _, c := range sg.Contexts {
		sanitizedID := sanitizer.Sanitize(c.ID)
		s := &EditorState[TObservation, TContext]{ID: sanitizedID, Label: c.Label}

		// Map by ORIGINAL ID to maintain graph link resolution from targets
		stateByID[c.ID] = s

		if meta, ok := sg.ContextMeta[c.ID]; ok && meta.HasOptionalContinuation {
			s.HasFallthroughPop = true
			s.FallthroughPopAmount = meta.FallthroughPopAmount
		}
	}

	nestLabelByContextID := make(map[string]syntaxa.GrammarLabel)
	for label, ctxID := range sg.NestBodyContextIDs {
		nestLabelByContextID[ctxID] = label
	}

	for id, s := range stateByID {
		if label, isNest := nestLabelByContextID[id]; isNest {
			edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
				IsNest: true, NestLabel: label,
			}
			s.Context = config.contextProducer(edCtx)
		} else if id == sg.RootContextID {
			s.Context = config.contextProducer(&EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{})
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
			tr       lowering.Transition[TToken, TNodeKind]
		}
		pairs := make([]pair, 0, len(transList))
		for _, tr := range transList {
			pri := tokenToPriority[tr.Token]
			pairs = append(pairs, pair{priority: pri, tr: tr})
		}
		sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].priority > pairs[j].priority })
		for _, p := range pairs {
			edTr := buildEditorTransitionFromGeneric(
				p.tr,
				stateByID,
				tokenToRule,
				config,
				delimitedStates,
				sanitizer,
			)
			if edTr != nil {
				s.Transitions = append(s.Transitions, *edTr)
			}
		}
	}

	grammarTokens := collectGrammarTokens(grammarPackage.Grammars)
	ambientTransitions := buildAmbientFromStateGraph(
		lexingRuleset,
		grammarTokens,
		config,
		delimitedStates,
		sanitizer,
	)

	splitNestBodiesAndWireImmediatePush(stateByID, nestLabelByContextID, sg.RootContextID, sanitizer)

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

	return &EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		LanguageMeta: LanguageMeta{
			LanguageName:    grammarPackage.Name,
			LanguageVersion: grammarPackage.Version,
		},
		LanguageMachine: LanguageMachine[TObservation, TContext]{
			EditorStates:       allStates,
			RootState:          rootOut,
			AmbientTransitions: ambientTransitions,
		},
	}, nil
}

func splitNestBodiesAndWireImmediatePush[TObservation cmp.Ordered, TContext any](
	stateByID map[string]*EditorState[TObservation, TContext],
	nestLabelByContextID map[string]syntaxa.GrammarLabel,
	rootContextID string,
	sanitizer *text.Sanitizer,
) {
	originalIDs := make([]string, 0, len(stateByID))
	for id := range stateByID {
		originalIDs = append(originalIDs, id)
	}

	for _, id := range originalIDs {
		s := stateByID[id]
		if id == rootContextID {
			continue
		}
		if _, isNest := nestLabelByContextID[id]; !isNest {
			continue
		}

		contentID := sanitizer.Sanitize(s.ID + "_content")
		contentState := &EditorState[TObservation, TContext]{
			ID:          contentID,
			Label:       sanitizer.Sanitize(s.Label + "_content"),
			Transitions: s.Transitions,
		}

		stateByID[id+"__content"] = contentState

		s.Transitions = nil
		s.ImmediatePushTarget = contentState
	}
}

func buildEditorTransitionFromGeneric[
	TObservation cmp.Ordered,
	TToken comparable,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
	TContext any,
](
	tr lowering.Transition[TToken, TNodeKind],
	stateByID map[string]*EditorState[TObservation, TContext],
	tokenToRule map[TToken]lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	delimitedStates map[string]*EditorState[TObservation, TContext],
	sanitizer *text.Sanitizer,
) *EditorTransition[TObservation, TContext] {
	edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		Token:    &tr.Token,
		NodeKind: tr.NodeKind,
	}
	override, hasOverride := config.overrideProducer(edCtx)
	matchCtx := config.contextProducer(edCtx)

	var pat pattern.RegulaAST[TObservation]
	hasPattern := false
	if hasOverride && override.Pattern != nil {
		pat = *override.Pattern
		hasPattern = true
	} else if rule, ok := tokenToRule[tr.Token]; ok {
		pat = rule.Pattern
		hasPattern = true
	}
	if !hasPattern {
		return nil
	}

	if hasOverride && override.MatchContext != nil {
		matchCtx = *override.MatchContext
	}

	var captures map[int]TContext
	if hasOverride {
		captures = override.Captures
	}

	if hasOverride && override.ForeignPayload != nil {
		return &EditorTransition[TObservation, TContext]{
			OnPattern:      pat,
			MatchContext:   matchCtx,
			Captures:       captures,
			Operation:      STACK_EMBED,
			ForeignPayload: override.ForeignPayload,
		}
	}

	if hasOverride && override.DelimitedPayload != nil {
		bodyState := getOrCreateDelimitedState(override.DelimitedPayload, delimitedStates, sanitizer)
		return &EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     captures,
			Operation:    STACK_PUSH,
			Targets:      []*EditorState[TObservation, TContext]{bodyState},
		}
	}

	targets := make([]*EditorState[TObservation, TContext], 0, len(tr.TargetContextIDs))
	for _, id := range tr.TargetContextIDs {
		if t := stateByID[id]; t != nil {
			targets = append(targets, t)
		}
	}

	popAmount := tr.PopAmount
	if popAmount <= 0 && tr.Operation == lowering.OpPop {
		popAmount = 1
	}

	return &EditorTransition[TObservation, TContext]{
		OnPattern:    pat,
		MatchContext: matchCtx,
		Captures:     captures,
		Operation:    stackOpFromLowering(tr.Operation),
		Targets:      targets,
		PopAmount:    popAmount,
	}
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

func collectGrammarTokens[TToken, TNodeKind comparable](
	grammars map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
) map[TToken]bool {
	seen := make(map[*syntaxa.Grammar[TToken, TNodeKind]]bool)
	result := make(map[TToken]bool)
	for _, g := range grammars {
		walkGrammarTokens(g, grammars, seen, result)
	}
	return result
}

func walkGrammarTokens[TToken, TNodeKind comparable](
	g *syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	seen map[*syntaxa.Grammar[TToken, TNodeKind]]bool,
	tokens map[TToken]bool,
) {
	if g == nil || seen[g] {
		return
	}
	seen[g] = true
	switch g.Kind {
	case syntaxa.GToken:
		tokens[g.Token] = true
	case syntaxa.GNest:
		if g.OpenToken != nil {
			tokens[*g.OpenToken] = true
		}
		if g.CloseToken != nil {
			tokens[*g.CloseToken] = true
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
	TToken comparable,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
	TContext any,
](
	lexingRuleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	grammarTokens map[TToken]bool,
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	delimitedStates map[string]*EditorState[TObservation, TContext],
	sanitizer *text.Sanitizer,
) []EditorTransition[TObservation, TContext] {
	type ruleEntry struct {
		rule     lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]
		priority int
	}
	var candidates []ruleEntry
	for _, rule := range lexingRuleset.GetRules() {
		if !grammarTokens[rule.Token] {
			candidates = append(candidates, ruleEntry{rule: rule, priority: rule.Priority})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].priority > candidates[j].priority
	})
	var out []EditorTransition[TObservation, TContext]
	for _, entry := range candidates {
		token := entry.rule.Token
		edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{Token: &token}
		override, hasOverride := config.overrideProducer(edCtx)
		matchCtx := config.contextProducer(edCtx)
		var pat pattern.RegulaAST[TObservation]
		if hasOverride && override.Pattern != nil {
			pat = *override.Pattern
		} else {
			pat = entry.rule.Pattern
		}
		if hasOverride && override.MatchContext != nil {
			matchCtx = *override.MatchContext
		}
		var captures map[int]TContext
		if hasOverride {
			captures = override.Captures
		}
		if hasOverride && override.ForeignPayload != nil {
			out = append(out, EditorTransition[TObservation, TContext]{
				OnPattern:      pat,
				MatchContext:   matchCtx,
				Captures:       captures,
				Operation:      STACK_EMBED,
				ForeignPayload: override.ForeignPayload,
			})
			continue
		}
		if hasOverride && override.DelimitedPayload != nil {
			bodyState := getOrCreateDelimitedState(override.DelimitedPayload, delimitedStates, sanitizer)
			out = append(out, EditorTransition[TObservation, TContext]{
				OnPattern:    pat,
				MatchContext: matchCtx,
				Captures:     captures,
				Operation:    STACK_PUSH,
				Targets:      []*EditorState[TObservation, TContext]{bodyState},
			})
			continue
		}
		out = append(out, EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     captures,
			Operation:    STACK_NONE,
		})
	}
	return out
}
