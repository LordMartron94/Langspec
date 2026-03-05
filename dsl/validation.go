package dsl

import (
	"fmt"
	"langspec/validation"
	"strconv"
	"strings"
)

// ------------------------------------------------------------- TYPES

type ValidationCtx = validation.LSTValidationStageContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type ValidationCode string

const (
	VALIDATION_DUPLICATE_TOKEN ValidationCode = "V_R001"

	VALIDATION_DUPLICATE_PATTERN_NAME    ValidationCode = "V_PAT001"
	VALIDATION_UNRESOLVED_PATTERN_REF    ValidationCode = "V_PAT002"
	VALIDATION_EMPTY_PATTERN_EXPRESSION  ValidationCode = "V_PAT003"
	VALIDATION_LOCAL_REF_OUTSIDE_SECTION ValidationCode = "V_PAT004"
	VALIDATION_CYCLIC_PATTERN_REF        ValidationCode = "V_PAT005"

	VALIDATION_UNREACHABLE_PATTERN     ValidationCode = "V_PAT006"
	VALIDATION_AMBIGUOUS_TOKEN_MATCH   ValidationCode = "V_LEX002"
	VALIDATION_IDENTICAL_TOKEN_PATTERN ValidationCode = "V_LEX003"
	VALIDATION_TOKEN_SHADOWED          ValidationCode = "V_LEX004"
	VALIDATION_NO_EOF_IN_META          ValidationCode = "V_LEX005"
	VALIDATION_MULTIPLE_EOF_IN_META    ValidationCode = "V_LEX006"

	VALIDATION_NEGATION_INVALID_CONTENT ValidationCode = "V_PAT007"
)

func (v ValidationCode) String() string {
	return string(v)
}

// ------------------------------------------------------------- STAGES

func getValidationStages() []*ValidationStage {
	stages := []*ValidationStage{
		{
			Name:        "Lex Rule Validation",
			Description: "Validates the lex rule section.",
			Order:       0,
			Processor: func(ctx *ValidationCtx) {
				lexRuleSection := ctx.RootNode.FindFirstKind(NodeLexSection)
				if lexRuleSection == nil {
					return
				}
				tks := map[string]struct{}{}

				lexRuleTokens := lexRuleSection.FindAllKind(NodeLexRuleTokenName)
				for _, lexRuleToken := range lexRuleTokens {
					value := getStringValue(lexRuleToken)

					if _, seen := tks[value]; seen {
						msg := fmt.Sprintf("token '%s' already declared (duplicate entry)", value)
						ctx.ReportError(VALIDATION_DUPLICATE_TOKEN.String(), msg, lexRuleToken)
					} else {
						tks[value] = struct{}{}
					}
				}

				eofMetaValueNodes := lexRuleSectionCollectEOFTrueMetaValues(lexRuleSection)
				if len(eofMetaValueNodes) == 0 {
					msg := "no lexeme has EOF=true in its meta section; the compiler will automatically inject an EOF token"
					ctx.ReportInfo(VALIDATION_NO_EOF_IN_META.String(), msg, lexRuleSection)
				} else if len(eofMetaValueNodes) > 1 {
					for _, node := range eofMetaValueNodes {
						msg := "multiple lexemes have EOF=true in their meta section; only one is allowed"
						ctx.ReportError(VALIDATION_MULTIPLE_EOF_IN_META.String(), msg, node)
					}
				}
			},
		},
		{
			Name:        "Pattern Validation",
			Description: "Validates pattern section: duplicate declarations, unresolved variable references, and local variables used outside the pattern section. Variables must be declared before use; locals may be used in any pattern definition but not outside the pattern section.",
			Order:       1,
			Processor: func(ctx *ValidationCtx) {
				allDeclared := make(map[string]struct{})
				localNameToDef := make(map[string]*Node)
				for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
					defNameNode := def.FindFirstKind(NodePatternDefName)
					if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
						continue
					}
					name := string(defNameNode.Tokens()[0].Raw)
					allDeclared[name] = struct{}{}
					if def.FindFirstKind(NodeLocalVariable) != nil {
						localNameToDef[name] = def
					}
				}

				declaredSoFar := make(map[string]struct{})
				for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
					defNameNode := def.FindFirstKind(NodePatternDefName)
					if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
						continue
					}

					name := string(defNameNode.Tokens()[0].Raw)
					if _, already := declaredSoFar[name]; already {
						msg := fmt.Sprintf("pattern name '%s' already declared (duplicate)", name)
						ctx.ReportError(VALIDATION_DUPLICATE_PATTERN_NAME.String(), msg, defNameNode)
					} else {
						declaredSoFar[name] = struct{}{}
					}

					for _, varRef := range def.FindAllKind(NodeVarRef) {
						tokens := varRef.Tokens()
						if len(tokens) < 2 {
							continue
						}
						refName := string(tokens[1].Raw)
						if _, ok := declaredSoFar[refName]; !ok {
							var msg string
							if _, declaredLater := allDeclared[refName]; declaredLater {
								msg = fmt.Sprintf("pattern reference '%s' used before declaration", refName)
							} else {
								msg = fmt.Sprintf("unresolved pattern reference '%s'", refName)
							}
							ctx.ReportError(VALIDATION_UNRESOLVED_PATTERN_REF.String(), msg, varRef)
						}
					}
				}

				for _, varRef := range ctx.RootNode.FindAllKind(NodeVarRef) {
					if enclosingPatternDef(varRef) != nil {
						continue
					}
					tokens := varRef.Tokens()
					if len(tokens) < 2 {
						continue
					}
					refName := string(tokens[1].Raw)
					if _, isLocal := localNameToDef[refName]; isLocal {
						msg := fmt.Sprintf("local variable '%s' referenced outside pattern section (e.g. in lex section)", refName)
						ctx.ReportError(VALIDATION_LOCAL_REF_OUTSIDE_SECTION.String(), msg, varRef)
					}
				}

				patternDeps := buildPatternDependencyMap(ctx.RootNode, declaredSoFar)
				for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
					defNameNode := def.FindFirstKind(NodePatternDefName)
					if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
						continue
					}
					name := string(defNameNode.Tokens()[0].Raw)
					cycle := findCycleInPatternDeps(name, patternDeps)
					if cycle != nil {
						msg := fmt.Sprintf("pattern '%s' has cyclic reference (e.g. %s)", name, formatCycle(cycle))
						ctx.ReportError(VALIDATION_CYCLIC_PATTERN_REF.String(), msg, defNameNode)
					}
				}

				for _, negNode := range ctx.RootNode.FindAllKind(NodePatternNegation) {
					children := negNode.Children()
					if len(children) == 0 {
						continue
					}
					operand := children[0]
					validateNegationSubtree(operand, func(offending *Node) {
						msg := "negation (!) may only contain character, range, group, or alternation; other constructs are not allowed"
						ctx.ReportError(VALIDATION_NEGATION_INVALID_CONTENT.String(), msg, offending)
					})
				}
			},
		},
		{
			Name:        "Unreachable Patterns",
			Description: "Emits info for patterns that are never referenced by any lex rule or by another pattern.",
			Order:       2,
			Processor: func(ctx *ValidationCtx) {
				patternDeps := buildPatternDependencyMap(ctx.RootNode, nil)
				reachable := computeReachablePatterns(ctx.RootNode, patternDeps)
				for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
					defNameNode := def.FindFirstKind(NodePatternDefName)
					if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
						continue
					}
					name := strings.TrimSpace(string(defNameNode.Tokens()[0].Raw))
					if def.FindFirstKind(NodeLocalVariable) != nil {
						continue
					}
					if !reachable[name] {
						msg := fmt.Sprintf("pattern '%s' is never referenced (unreachable)", name)
						ctx.ReportInfo(VALIDATION_UNREACHABLE_PATTERN.String(), msg, defNameNode)
					}
				}
			},
		},
		{
			Name:        "Lex Pattern Analysis",
			Description: "Detects ambiguous token matches, identical token patterns, and tokens shadowed by higher-priority rules.",
			Order:       3,
			Processor: func(ctx *ValidationCtx) {
				lexRules := collectLexRules(ctx.RootNode)
				patternKeyToRules := make(map[string][]lexRuleInfo)
				for _, r := range lexRules {
					key := r.patternKey
					patternKeyToRules[key] = append(patternKeyToRules[key], r)
				}
				for key, rules := range patternKeyToRules {
					if len(rules) < 2 {
						continue
					}
					for _, r := range rules {
						msg := fmt.Sprintf("token '%s' produces the same pattern as other token(s) (pattern key: %s)", r.tokenName, key)
						ctx.ReportWarning(VALIDATION_IDENTICAL_TOKEN_PATTERN.String(), msg, r.tokenNameNode)
					}
					for _, r := range rules {
						msg := fmt.Sprintf("token '%s' can match the same input as other token(s) (ambiguous)", r.tokenName)
						ctx.ReportWarning(VALIDATION_AMBIGUOUS_TOKEN_MATCH.String(), msg, r.patternNode)
					}
					maxPri := rules[0].priority
					for _, r := range rules[1:] {
						if r.priority > maxPri {
							maxPri = r.priority
						}
					}
					for _, r := range rules {
						if r.priority < maxPri {
							msg := fmt.Sprintf("token '%s' is shadowed by higher-priority token(s) with the same pattern", r.tokenName)
							ctx.ReportWarning(VALIDATION_TOKEN_SHADOWED.String(), msg, r.tokenNameNode)
						}
					}
				}
			},
		},
	}

	return stages
}

// negationAllowedKind returns true if this node kind is allowed inside a negation (!) subtree.
func negationAllowedKind(kind LangSpecParserNodeKind) bool {
	switch kind {
	case NodeCharLiteral, NodePatternRange, NodePatternGroup, NodePatternAlternation, NodePatternSegment:
		return true
	default:
		return false
	}
}

// validateNegationSubtree traverses the negated operand subtree and calls report for any node that is not
// a character, range, group, or alternation (or segment wrapper). Recurses into Group, Alternation, and Segment.
func validateNegationSubtree(node *Node, report func(offending *Node)) {
	kind := node.Kind()
	if !negationAllowedKind(kind) {
		report(node)
	}
	switch kind {
	case NodeCharLiteral, NodePatternRange:
		return
	case NodePatternGroup, NodePatternAlternation, NodePatternSegment:
		for _, ch := range node.Children() {
			validateNegationSubtree(ch, report)
		}
	default:
		for _, ch := range node.Children() {
			validateNegationSubtree(ch, report)
		}
	}
}

func getStringValue(node *Node) string {
	value, ok := AttributeAs[string](node, ATTRIBUTE_LITERAL_STRING_VALUE)
	if !ok {
		panic(fmt.Errorf("engine error encountered: %v not stored for node %v", ATTRIBUTE_LITERAL_STRING_VALUE, node))
	}

	return value
}

// lexRuleSectionCollectEOFTrueMetaValues returns all NodeMetaValue nodes (keyword true) for meta key "EOF" under section.
func lexRuleSectionCollectEOFTrueMetaValues(lexSection *Node) []*Node {
	var out []*Node
	for _, ruleNode := range lexSection.FindAllKind(NodeLexRule) {
		metaSection := ruleNode.FindFirstKind(NodeMetaSection)
		if metaSection == nil {
			continue
		}
		for _, pair := range metaSection.FindAllKind(NodeMetaKeyValuePair) {
			keyNode := pair.FindFirstKind(NodeMetaKey)
			valueNode := pair.FindFirstKind(NodeMetaValue)
			if keyNode == nil || valueNode == nil || len(keyNode.Tokens()) == 0 || len(valueNode.Tokens()) == 0 {
				continue
			}
			key := strings.TrimSpace(string(keyNode.Tokens()[0].Raw))
			if !strings.EqualFold(key, "EOF") {
				continue
			}
			if valueNode.Tokens()[0].Token == TokKWTrue {
				out = append(out, valueNode)
			}
		}
	}
	return out
}

/*
enclosingPatternDef returns the innermost NodePatternDefinition that contains node, or nil if node is not inside any pattern definition. Walks Parent() upward.
*/
func enclosingPatternDef(node *Node) *Node {
	for n := node; n != nil; n = n.Parent() {
		if n.Kind() == NodePatternDefinition {
			return n
		}
	}
	return nil
}

// getVarRefTargetName returns the pattern name referenced by a NodeVarRef (from second token or NodeVarRefTarget child). Trimmed.
func getVarRefTargetName(varRef *Node) string {
	if targetNode := varRef.FindFirstKind(NodeVarRefTarget); targetNode != nil && len(targetNode.Tokens()) > 0 {
		return strings.TrimSpace(string(targetNode.Tokens()[0].Raw))
	}
	tokens := varRef.Tokens()
	if len(tokens) >= 2 {
		return strings.TrimSpace(string(tokens[1].Raw))
	}
	return ""
}

// buildPatternDependencyMap returns a map from pattern name to the list of pattern names it references (via VarRef).
// If declaredOnly is non-nil, only refs that are keys in declaredOnly are included.
func buildPatternDependencyMap(root *Node, declaredOnly map[string]struct{}) map[string][]string {
	out := make(map[string][]string)
	for _, def := range root.FindAllKind(NodePatternDefinition) {
		defNameNode := def.FindFirstKind(NodePatternDefName)
		if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
			continue
		}
		name := strings.TrimSpace(string(defNameNode.Tokens()[0].Raw))
		var refs []string
		for _, varRef := range def.FindAllKind(NodeVarRef) {
			refName := getVarRefTargetName(varRef)
			if refName == "" {
				continue
			}
			if declaredOnly != nil {
				if _, ok := declaredOnly[refName]; !ok {
					continue
				}
			}
			refs = append(refs, refName)
		}
		out[name] = refs
	}
	return out
}

func findCycleInPatternDeps(start string, deps map[string][]string) []string {
	path := make(map[string]bool)
	stack := make([]string, 0, 8)
	var cycle []string
	var dfs func(name string) bool
	dfs = func(name string) bool {
		if path[name] {
			for i := range stack {
				if stack[i] == name {
					cycle = make([]string, 0, len(stack)-i+1)
					cycle = append(cycle, stack[i:]...)
					cycle = append(cycle, name)
					return true
				}
			}
			return true
		}
		path[name] = true
		stack = append(stack, name)
		for _, ref := range deps[name] {
			if dfs(ref) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		path[name] = false
		return false
	}
	if dfs(start) && cycle != nil {
		return cycle
	}
	return nil
}

func formatCycle(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	s := cycle[0]
	for i := 1; i < len(cycle); i++ {
		s += " -> " + cycle[i]
	}
	return s
}

func computeReachablePatterns(root *Node, deps map[string][]string) map[string]bool {
	entryPoints := make(map[string]bool)
	lexSection := root.FindFirstKind(NodeLexSection)
	if lexSection != nil {
		for _, ruleNode := range lexSection.FindAllKind(NodeLexRule) {
			varRef := ruleNode.FindFirstKind(NodeVarRef)
			if varRef != nil {
				name := getVarRefTargetName(varRef)
				if name != "" {
					entryPoints[name] = true
				}
			}
		}
	}
	reachable := make(map[string]bool)
	var bfs func(name string)
	bfs = func(name string) {
		if reachable[name] {
			return
		}
		reachable[name] = true
		for _, ref := range deps[name] {
			bfs(ref)
		}
	}
	for name := range entryPoints {
		bfs(name)
	}
	return reachable
}

type lexRuleInfo struct {
	tokenName     string
	patternKey    string
	priority      int
	tokenNameNode *Node
	patternNode   *Node
}

func collectLexRules(root *Node) []lexRuleInfo {
	var out []lexRuleInfo
	lexSection := root.FindFirstKind(NodeLexSection)
	if lexSection == nil {
		return out
	}
	for _, ruleNode := range lexSection.FindAllKind(NodeLexRule) {
		tokenNameNode := ruleNode.FindFirstKind(NodeLexRuleTokenName)
		priNode := ruleNode.FindFirstKind(NodeLexRulePriority)
		if tokenNameNode == nil {
			continue
		}
		tokenName, ok := AttributeAs[string](tokenNameNode, ATTRIBUTE_LITERAL_STRING_VALUE)
		if !ok {
			continue
		}
		priority := 0
		if priNode != nil {
			raw := priNode.GetContent("")
			if raw != "" {
				if n, err := parseIntFromContent(raw); err == nil {
					priority = n
				}
			}
		}
		var patternKey string
		var patternNode *Node
		if varRef := ruleNode.FindFirstKind(NodeVarRef); varRef != nil {
			if name := getVarRefTargetName(varRef); name != "" {
				patternKey = "ref:" + name
				patternNode = varRef
			}
		}
		if patternKey == "" {
			if regexNode := ruleNode.FindFirstKind(NodeLexRulePattern); regexNode != nil {
				patternKey = "regex:" + regexNode.GetContent("")
				patternNode = regexNode
			}
		}
		if patternKey == "" || patternNode == nil {
			continue
		}
		out = append(out, lexRuleInfo{
			tokenName:     tokenName,
			patternKey:    patternKey,
			priority:      priority,
			tokenNameNode: tokenNameNode,
			patternNode:   patternNode,
		})
	}
	return out
}

func parseIntFromContent(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
