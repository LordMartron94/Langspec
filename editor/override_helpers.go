package editor

import "fmt"

// ------------------------------------------------------------------ NEST STATE SEQUENCE (declarative builder)

/*
NestRuleAction is the action for a rule within a nest step. Structural only; no domain semantics.
*/
type NestRuleAction uint8

const (
	NestRuleActionMatch    NestRuleAction = iota // Consume token, stay in same state
	NestRuleActionPushNext                       // Push to next step's state
	NestRuleActionPop                            // Pop one or more states from stack
)

/*
NestStepRule describes one rule in a nest step: which token, scope, and action. Used by BuildNestStateSequence.
*/
type NestStepRule[TToken comparable] struct {
	Token    TToken
	Scope    string
	Action   NestRuleAction
	PopCount int // Used when Action is NestRuleActionPop (number of states to pop)
}

/*
NestStep describes one state in a linear nest sequence: optional meta scope and a list of rules.
*/
type NestStep[TToken comparable] struct {
	LabelSuffix string // Used for state ID derivation and label (e.g. "expect_name")
	MetaScope   string // Optional; applied to the state
	Rules       []NestStepRule[TToken]
}

/*
BuildNestStateSequence builds a linear sequence of states from declarative steps. For each step
it creates a State with derived ID/label and optional MetaScope; for each rule it uses
ctx.GetRegEx(rule.Token) and ctx.ApplyScope(rule.Scope). PushNext links to the next step's
state; Pop uses the rule's PopCount. Returns the first state's ID and the slice of states.
*/
func BuildNestStateSequence[TToken comparable](
	ctx *NestOverrideContext[TToken],
	steps []NestStep[TToken],
) (entryStateID StateID, states []State) {
	if len(steps) == 0 {
		return 0, nil
	}

	stateIDs := make([]StateID, len(steps))
	states = make([]State, 0, len(steps))

	for i := range steps {
		stateIDs[i] = ctx.DeriveStateID(steps[i].LabelSuffix)
	}

	for i, step := range steps {
		var rules []StateRule
		for ri, r := range step.Rules {
			sr := StateRule{
				ID:    StateRuleID(ctx.DeriveStateID(fmt.Sprintf("%s_rule_%d", step.LabelSuffix, ri))),
				Label: step.LabelSuffix + "_rule_" + fmt.Sprintf("%d", ri),
				RegEx: ctx.GetRegEx(r.Token),
				Scope: ctx.ApplyScope(r.Scope),
			}
			switch r.Action {
			case NestRuleActionMatch:
				sr.Action = ACTION_MATCH
			case NestRuleActionPushNext:
				sr.Action = ACTION_PUSH
				if i+1 < len(stateIDs) {
					sr.ActionTarget = stateIDs[i+1]
				}
			case NestRuleActionPop:
				sr.Action = ACTION_POP
				sr.PopCount = r.PopCount
				if sr.PopCount <= 0 {
					sr.PopCount = 1
				}
			}
			rules = append(rules, sr)
		}

		// Invalid fallback so the highlighter marks unexpected tokens in this state.
		rules = append(rules, InvalidFallbackRule(
			StateRuleID(ctx.DeriveStateID(step.LabelSuffix+"_invalid")),
			ctx.ScopeExtension,
		))

		st := State{
			ID:            stateIDs[i],
			Label:         ctx.NestLabel + "_" + step.LabelSuffix,
			IsRootContext: false,
			Rules:         rules,
		}
		if step.MetaScope != "" {
			st.MetaScope = ctx.ApplyScope(step.MetaScope)
		}
		states = append(states, st)
	}

	return stateIDs[0], states
}

// ------------------------------------------------------------------ TOKEN OVERRIDE HELPERS

/*
TokenOverrideDelimitedRegion builds a token override that pushes to a single inner state
whose only rule matches a close pattern and pops. Structure: one rule matching openRegex
pushes to a new state; that state has one rule matching closeRegex that pops. Scopes are
applied to the open match, the inner region (meta scope on the inner state), and the close
match. Use for any open/close-delimited region (paired delimiters). The editor does not
prescribe semantics; the client supplies regex and scope strings.
*/
func TokenOverrideDelimitedRegion(
	ctx *TokenOverrideContext,
	openRegex, closeRegex string,
	scopeOpen, scopeBody, scopeClose string,
) (StateRule, []State) {
	bodyStateID := ctx.DeriveStateID("inner")

	mainRule := StateRule{
		ID:           StateRuleID(ctx.BaseID),
		Label:        ctx.Label + "_open",
		RegEx:        openRegex,
		Scope:        ctx.ApplyScope(scopeOpen),
		Action:       ACTION_PUSH,
		ActionTarget: bodyStateID,
	}

	bodyState := State{
		ID:        bodyStateID,
		Label:     ctx.Label + "_inner",
		MetaScope: ctx.ApplyScope(scopeBody),
		Rules: []StateRule{
			{
				ID:     StateRuleID(ctx.DeriveStateID("close")),
				Label:  ctx.Label + "_close",
				RegEx:  closeRegex,
				Scope:  ctx.ApplyScope(scopeClose),
				Action: ACTION_POP,
			},
		},
		OmitPrototype: true,
	}

	return mainRule, []State{bodyState}
}

/*
TokenOverrideMatchWithCapture builds a token override that is a single StateRule with the
given regex and scope, and an optional scope for capture group 1 (e.g. a leading delimiter).
scopeCapture1 may be empty for no capture. Use for any single-token match with an optional
sub-scope. The editor does not prescribe semantics; the client supplies regex and scope strings.
*/
func TokenOverrideMatchWithCapture(
	ctx *TokenOverrideContext,
	regex, scopeMatch, scopeCapture1 string,
) (StateRule, []State) {
	mainRule := StateRule{
		ID:     StateRuleID(ctx.BaseID),
		Label:  ctx.Label,
		RegEx:  regex,
		Scope:  ctx.ApplyScope(scopeMatch),
		Action: ACTION_MATCH,
	}
	if scopeCapture1 != "" {
		mainRule.Captures = map[int]string{1: ctx.ApplyScope(scopeCapture1)}
	}
	return mainRule, nil
}

/*
TokenOverrideEmbed builds a single StateRule that embeds another syntax (ST4 embed/escape).
matchRegex matches the opening delimiter; embedTarget is the context to embed (e.g. scope:source.regexp);
embedScope is applied to the embedded region; escapeRegex ends the embed; escapeCaptures scopes the
escape pattern's capture groups (0 = entire escape match). scopeForMatch is applied to the opening
delimiter. Returns one StateRule and no extra states. Use when the content until escape should be
highlighted by another syntax.
*/
func TokenOverrideEmbed(
	ctx *TokenOverrideContext,
	matchRegex, scopeForMatch, embedTarget, embedScope, escapeRegex string,
	escapeCaptures map[int]string,
) (StateRule, []State) {
	mainRule := StateRule{
		ID:         StateRuleID(ctx.BaseID),
		Label:      ctx.Label,
		RegEx:      matchRegex,
		Scope:      ctx.ApplyScope(scopeForMatch),
		Action:     ACTION_EMBED,
		Embed:      embedTarget,
		EmbedScope: ctx.ApplyScope(embedScope),
		Escape:     escapeRegex,
	}
	if len(escapeCaptures) > 0 {
		applied := make(map[int]string, len(escapeCaptures))
		for k, v := range escapeCaptures {
			applied[k] = ctx.ApplyScope(v)
		}
		mainRule.EscapeCaptures = applied
	}
	return mainRule, nil
}
