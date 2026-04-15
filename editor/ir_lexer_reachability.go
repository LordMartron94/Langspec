package editor

import (
	"cmp"
	"fmt"
	"sort"
	"strings"

	"langspec"
	"lexarch"
	"syntaxa/lowering"
)

// Matches lexarch: LexingSession begins with bottom marker + start state frame; at most
// lexingStackDepthMax frames total, so at most (lexingStackDepthMax - 1) named modes
// including the implicit start state on the stack.
const editorLexerMaxModesOnStack = 15

func editorLexerStartStack() []string {
	return []string{"INITIAL"}
}

func editorLexerStackClone(stack []string) []string {
	out := make([]string, len(stack))
	copy(out, stack)
	return out
}

func editorLexerStackKey(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return strings.Join(stack, "\x00")
}

func editorLexerStackDecode(key string) []string {
	if key == "" {
		return nil
	}
	return strings.Split(key, "\x00")
}

func editorLexerStackTopMode(stack []string) string {
	if len(stack) == 0 {
		return "INITIAL"
	}
	return stack[len(stack)-1]
}

func editorPopLexerStackFrames(stack []string, amount int) []string {
	if amount < 1 {
		amount = 1
	}
	n := len(stack)
	newLen := n - amount
	if newLen < 1 {
		newLen = 1
	}
	return stack[:newLen]
}

func editorApplyLexerStackAfterRule(
	stack []string,
	kind langspec.LexerStackOpKind,
	targets []string,
	popAmt int,
) ([]string, error) {
	if len(stack) == 0 {
		return nil, fmt.Errorf("langspec editor: internal error: empty lexer stack in simulation")
	}
	switch kind {
	case langspec.LexerStackOpNone:
		return editorLexerStackClone(stack), nil
	case langspec.LexerStackOpPush:
		if len(targets) == 0 {
			return editorLexerStackClone(stack), nil
		}
		out := editorLexerStackClone(stack)
		out = append(out, targets...)
		if len(out) > editorLexerMaxModesOnStack {
			return nil, fmt.Errorf(
				"langspec editor: lexer stack would exceed max depth (%d modes, cap %d)",
				len(out), editorLexerMaxModesOnStack,
			)
		}
		return out, nil
	case langspec.LexerStackOpPop:
		amt := popAmt
		if amt < 1 {
			amt = 1
		}
		return editorLexerStackClone(editorPopLexerStackFrames(stack, amt)), nil
	case langspec.LexerStackOpSet:
		if len(targets) == 0 {
			return editorLexerStackClone(stack), nil
		}
		afterPop := editorPopLexerStackFrames(stack, 1)
		out := append(editorLexerStackClone(afterPop), targets...)
		if len(out) > editorLexerMaxModesOnStack {
			return nil, fmt.Errorf(
				"langspec editor: lexer stack would exceed max depth (%d modes, cap %d)",
				len(out), editorLexerMaxModesOnStack,
			)
		}
		return out, nil
	default:
		return editorLexerStackClone(stack), nil
	}
}

/*
LexingRulesGroupedByLexerState sorts rules per lexer state by priority (desc) then token id.
Used by syntax emitters (e.g. Sublime) with LexerStackReachabilityFromStateGraph.
*/
func LexingRulesGroupedByLexerState[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](
	lexingRuleset *LexingRuleSet[TObservation, TToken, TTokenRole],
) (
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
	allLexerStates []string,
) {
	return editorLexRulesGroupedByState(lexingRuleset)
}

/*
editorLexRulesGroupedByState sorts rules per lexer state by priority (desc) then token id.
*/
func editorLexRulesGroupedByState[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](
	lexingRuleset *LexingRuleSet[TObservation, TToken, TTokenRole],
) (
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
	allLexerStates []string,
) {
	rules := LexingRuleSetGetRules(lexingRuleset)
	by := make(map[string][]LexingRule[TObservation, TToken, TTokenRole])
	seenState := make(map[string]bool)
	for _, r := range rules {
		st := r.LexerState
		if st == "" {
			st = "INITIAL"
		}
		by[st] = append(by[st], r)
		seenState[st] = true
	}
	for st := range seenState {
		allLexerStates = append(allLexerStates, st)
		list := by[st]
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Priority != list[j].Priority {
				return list[i].Priority > list[j].Priority
			}
			return uint32(list[i].Token) < uint32(list[j].Token)
		})
		by[st] = list
	}
	sort.Strings(allLexerStates)
	return by, allLexerStates
}

func editorFindLexRuleForTokenInState[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
	state string,
	token lexarch.TokenKind,
) (LexingRule[TObservation, TToken, TTokenRole], bool) {
	list := rulesByState[state]
	for _, r := range list {
		if lexarch.TokenKind(r.Token) == token {
			return r, true
		}
	}
	return LexingRule[TObservation, TToken, TTokenRole]{}, false
}

type lexerReachWorkItem struct {
	ctx   string
	stack []string
}

/*
editorComputeLexerStackReachability runs a worklist over (parse context, lexer stack) pairs.
Stacks mirror lexarch: at least one mode frame (INITIAL), push appends targets in order,
pop removes up to k top frames without going below INITIAL, set is pop(1) then push(targets).
*/
func editorLexerReachTryAdd(
	reached map[string]map[string]bool,
	ctx string,
	stack []string,
) (newly bool, err error) {
	if len(stack) > editorLexerMaxModesOnStack {
		return false, fmt.Errorf(
			"langspec editor: lexer stack depth exceeds max (%d modes, cap %d)",
			len(stack), editorLexerMaxModesOnStack,
		)
	}
	sk := editorLexerStackKey(stack)
	if reached[ctx] == nil {
		reached[ctx] = make(map[string]bool)
	}
	if reached[ctx][sk] {
		return false, nil
	}
	reached[ctx][sk] = true
	return true, nil
}

func editorComputeLexerStackReachability[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TNodeKind comparable,
](
	sg *lowering.StateGraph[TNodeKind],
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
) (map[string]map[string]bool, error) {
	reached := make(map[string]map[string]bool)
	if sg == nil {
		return reached, nil
	}
	root := sg.RootContextID
	if root == "" {
		return reached, nil
	}

	rootStack := editorLexerStartStack()
	if _, err := editorLexerReachTryAdd(reached, root, rootStack); err != nil {
		return nil, err
	}
	queue := []lexerReachWorkItem{{ctx: root, stack: editorLexerStackClone(rootStack)}}

	for head := 0; head < len(queue); head++ {
		w := queue[head]
		transList := sg.Transitions[w.ctx]
		topMode := editorLexerStackTopMode(w.stack)

		for _, tr := range transList {
			rule, ok := editorFindLexRuleForTokenInState(rulesByState, topMode, tr.Token)
			if !ok {
				continue
			}
			nextStack, err := editorApplyLexerStackAfterRule(
				w.stack, rule.StackKind, rule.StackTargets, rule.StackPopAmount,
			)
			if err != nil {
				return nil, err
			}
			for _, tid := range tr.TargetContextIDs {
				if tid == "" {
					continue
				}
				newly, err := editorLexerReachTryAdd(reached, tid, nextStack)
				if err != nil {
					return nil, err
				}
				if newly {
					queue = append(queue, lexerReachWorkItem{
						ctx:   tid,
						stack: editorLexerStackClone(nextStack),
					})
				}
			}
		}
	}

	return reached, nil
}

/*
LexerStackReachabilityFromStateGraph is the exported name for editorComputeLexerStackReachability.
*/
func LexerStackReachabilityFromStateGraph[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
	TNodeKind comparable,
](
	sg *lowering.StateGraph[TNodeKind],
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
) (map[string]map[string]bool, error) {
	return editorComputeLexerStackReachability(sg, rulesByState)
}

/*
ResolveLexingRuleAtParseContext returns the lexing rule for token tok at parse context
parseContextID using the same reachability rules as EditorIRFromStateGraph. Syntax
emitters use this to build peek lookaheads from PeekAfterMatch constraints; the editor
IR carries only grammar data, not composed regexes.
*/
func ResolveLexingRuleAtParseContext[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](
	parseContextID string,
	tok lexarch.TokenKind,
	reached map[string]map[string]bool,
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
) (LexingRule[TObservation, TToken, TTokenRole], bool, error) {
	return editorResolveLexingRuleForParseTransition(parseContextID, tok, reached, rulesByState)
}

func editorLexRuleSignature[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](r LexingRule[TObservation, TToken, TTokenRole]) string {
	return fmt.Sprintf("%d|%s|%d|%v|%d", r.Priority, r.LexerState, r.StackKind, r.StackTargets, r.StackPopAmount)
}

func editorLexRuleEmissionKey[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](r LexingRule[TObservation, TToken, TTokenRole]) (string, error) {
	reStr, err := r.Pattern.ToRegEx()
	if err != nil {
		return "", fmt.Errorf("langspec editor: pattern to regex: %w", err)
	}
	return reStr + "\x1f" + editorLexRuleSignature(r), nil
}

/*
editorResolveLexingRuleForParseTransition picks the lexing rule for grammar token tok at parse
context ctxID. Among reachable lexer stacks, only those whose top mode defines tok are
considered; those candidates must agree on the same emission (regex + metadata), or an
error is returned. If no stack can lex tok, (false, nil) is returned.
*/
func editorResolveLexingRuleForParseTransition[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](
	ctxID string,
	tok lexarch.TokenKind,
	reached map[string]map[string]bool,
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
) (LexingRule[TObservation, TToken, TTokenRole], bool, error) {
	stacks := reached[ctxID]
	if len(stacks) == 0 {
		stacks = map[string]bool{editorLexerStackKey(editorLexerStartStack()): true}
	}

	var first LexingRule[TObservation, TToken, TTokenRole]
	var firstKey string
	var have bool
	candidates := make([]LexingRule[TObservation, TToken, TTokenRole], 0)

	for sk := range stacks {
		stack := editorLexerStackDecode(sk)
		if len(stack) == 0 {
			stack = editorLexerStartStack()
		}
		mode := editorLexerStackTopMode(stack)
		rule, ok := editorFindLexRuleForTokenInState(rulesByState, mode, tok)
		if !ok {
			continue
		}
		key, err := editorLexRuleEmissionKey(rule)
		if err != nil {
			return LexingRule[TObservation, TToken, TTokenRole]{}, false, err
		}
		if !have {
			first = rule
			firstKey = key
			have = true
			candidates = append(candidates, rule)
			continue
		}
		if key != firstKey {
			candidates = append(candidates, rule)
			// If all candidates share identical regex, prefer highest-priority rule.
			// This covers embed close-token handoff where mode-specific stack metadata differs.
			sameRegex, picked, okPick := editorPickHighestPrioritySameRegex(candidates)
			if sameRegex && okPick {
				return picked, true, nil
			}
			return LexingRule[TObservation, TToken, TTokenRole]{}, false, fmt.Errorf(
				"langspec editor: ambiguous lex rule for parse context %q token kind %v: reachable lexer stacks that can lex this token disagree on pattern or stack metadata",
				ctxID, tok,
			)
		}
		candidates = append(candidates, rule)
	}
	if !have {
		if fallbackRule, ok, err := editorResolveUniqueLexRuleAcrossAllStates(rulesByState, tok); err != nil {
			return LexingRule[TObservation, TToken, TTokenRole]{}, false, err
		} else if ok {
			return fallbackRule, true, nil
		}
		return LexingRule[TObservation, TToken, TTokenRole]{}, false, nil
	}
	return first, true, nil
}

func editorResolveUniqueLexRuleAcrossAllStates[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](
	rulesByState map[string][]LexingRule[TObservation, TToken, TTokenRole],
	tok lexarch.TokenKind,
) (LexingRule[TObservation, TToken, TTokenRole], bool, error) {
	var (
		haveFirst  bool
		firstRule  LexingRule[TObservation, TToken, TTokenRole]
		firstKey   string
		candidates []LexingRule[TObservation, TToken, TTokenRole]
	)
	for _, rules := range rulesByState {
		for _, r := range rules {
			if lexarch.TokenKind(r.Token) != tok {
				continue
			}
			candidates = append(candidates, r)
			key, err := editorLexRuleEmissionKey(r)
			if err != nil {
				return LexingRule[TObservation, TToken, TTokenRole]{}, false, err
			}
			if !haveFirst {
				haveFirst = true
				firstRule = r
				firstKey = key
				continue
			}
			if key != firstKey {
				// Allow fallback selection when all candidate rules share the same regex
				// but differ by mode-specific metadata (e.g. lexer state name).
				sameRegex, picked, okPick := editorPickHighestPrioritySameRegex(candidates)
				if sameRegex && okPick {
					return picked, true, nil
				}
				return LexingRule[TObservation, TToken, TTokenRole]{}, false, nil
			}
		}
	}
	if !haveFirst {
		return LexingRule[TObservation, TToken, TTokenRole]{}, false, nil
	}
	return firstRule, true, nil
}

func editorPickHighestPrioritySameRegex[
	TObservation cmp.Ordered,
	TToken ~uint32,
	TTokenRole comparable,
](rules []LexingRule[TObservation, TToken, TTokenRole]) (bool, LexingRule[TObservation, TToken, TTokenRole], bool) {
	if len(rules) == 0 {
		return false, LexingRule[TObservation, TToken, TTokenRole]{}, false
	}
	var regex0 string
	for i, r := range rules {
		re, err := r.Pattern.ToRegEx()
		if err != nil {
			return false, LexingRule[TObservation, TToken, TTokenRole]{}, false
		}
		if i == 0 {
			regex0 = re
			continue
		}
		if re != regex0 {
			return false, LexingRule[TObservation, TToken, TTokenRole]{}, false
		}
	}
	best := rules[0]
	for i := 1; i < len(rules); i++ {
		if rules[i].Priority > best.Priority {
			best = rules[i]
		}
	}
	return true, best, true
}
