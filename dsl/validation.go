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

	VALIDATION_UNREACHABLE_PATTERN         ValidationCode = "V_PAT006"
	VALIDATION_AMBIGUOUS_TOKEN_MATCH       ValidationCode = "V_LEX002"
	VALIDATION_IDENTICAL_TOKEN_PATTERN     ValidationCode = "V_LEX003"
	VALIDATION_TOKEN_SHADOWED              ValidationCode = "V_LEX004"
	VALIDATION_NO_EOF_IN_META              ValidationCode = "V_LEX005"
	VALIDATION_MULTIPLE_EOF_IN_META        ValidationCode = "V_LEX006"
	VALIDATION_TOKEN_UNREFERENCED_IN_PARSE ValidationCode = "V_LEX007"
	VALIDATION_UNRESOLVED_TOKEN_REF        ValidationCode = "V_LEX008"

	VALIDATION_NEGATION_INVALID_CONTENT  ValidationCode = "V_PAT007"
	VALIDATION_REPETITION_MIN_GT_MAX     ValidationCode = "V_PAT008"
	VALIDATION_REPETITION_NEGATIVE_BOUND ValidationCode = "V_PAT009"

	VALIDATION_DUPLICATE_PARSE_RULE_NAME ValidationCode = "V_PAR001"
	VALIDATION_UNRESOLVED_PARSE_RULE_REF ValidationCode = "V_PAR002"
	VALIDATION_UNREFERENCED_PARSE_RULE   ValidationCode = "V_PAR003"
	VALIDATION_PROGRAM_RULE_REQUIRED     ValidationCode = "V_PAR004"

	VALIDATION_PARSE_LEFT_RECURSION                ValidationCode = "V_PAR005"
	VALIDATION_PARSE_UNBOUNDED_OPTIONAL_REPETITION ValidationCode = "V_PAR006"

	VALIDATION_DUPLICATE_PRATT_EXPR     ValidationCode = "V_PRA001"
	VALIDATION_PRATT_UNRESOLVED_TOKEN   ValidationCode = "V_PRA002"
	VALIDATION_PRATT_UNRESOLVED_PATTERN ValidationCode = "V_PRA003"
	VALIDATION_PRATT_UNRESOLVED_RULE    ValidationCode = "V_PRA004"
	VALIDATION_PRATT_LOCAL_LEAK         ValidationCode = "V_PRA005"
)

func (v ValidationCode) String() string {
	return string(v)
}

const programRuleName = "PROGRAM"

// ------------------------------------------------------------- STAGES REGISTRATION

func getValidationStages() []*ValidationStage {
	return []*ValidationStage{
		{
			Name:        "Symbol Binding & Environment",
			Description: "Builds semantic environment, checks duplicates, scoping, and reference resolution.",
			Order:       0,
			Processor:   processSymbolBinding,
		},
		{
			Name:        "Structure & Reachability",
			Description: "Analyzes cross-section reachability and detects cyclic dependencies.",
			Order:       1,
			Processor:   processReachability,
		},
		{
			Name:        "Lexer Semantics",
			Description: "Validates EOF configurations, shadowing, and ambiguous token matches.",
			Order:       2,
			Processor:   processLexSemantics,
		},
		{
			Name:        "Pattern Semantics",
			Description: "Validates pattern section logic, repetition bounds, and negations.",
			Order:       3,
			Processor:   processPatternSemantics,
		},
		{
			Name:        "Parser Safety",
			Description: "Detects infinite loops and left-recursion in the parse section.",
			Order:       4,
			Processor:   processParseSafety,
		},
	}
}

// ------------------------------------------------------------- SYMBOL BINDING (STAGE 0)

func processSymbolBinding(ctx *ValidationCtx) {
	env := BuildSemanticEnv(ctx.RootNode, func(kind SemanticSymbolKind, name string, node *Node) {
		var code ValidationCode
		var msg string
		switch kind {
		case SymbolKindToken:
			code = VALIDATION_DUPLICATE_TOKEN
			msg = fmt.Sprintf("token '%s' already declared", name)
		case SymbolKindPattern:
			code = VALIDATION_DUPLICATE_PATTERN_NAME
			msg = fmt.Sprintf("pattern '%s' already declared", name)
		case SymbolKindRule:
			code = VALIDATION_DUPLICATE_PARSE_RULE_NAME
			msg = fmt.Sprintf("parse rule '%s' already declared", name)
		case SymbolKindPratt:
			code = VALIDATION_DUPLICATE_PRATT_EXPR
			msg = fmt.Sprintf("pratt expression '%s' already declared", name)
		default:
			code = VALIDATION_DUPLICATE_TOKEN
			msg = fmt.Sprintf("symbol '%s' already declared", name)
		}
		ctx.ReportError(code.String(), msg, node)
	})

	validateTokenReferences(ctx, env)
	validatePatternReferences(ctx, env)
	validateRuleReferences(ctx, env)
}

func validateTokenReferences(ctx *ValidationCtx, env *SemanticEnv) {
	used := make(map[string]bool)
	ignoredRoles := getIgnoredRoles(ctx)

	markExplicitTokenReferences(ctx, env, used)
	markNestTokenReferences(ctx, env, used)
	reportUnusedTokens(ctx, env, used, ignoredRoles)
}

func markExplicitTokenReferences(ctx *ValidationCtx, env *SemanticEnv, used map[string]bool) {
	for _, ref := range ctx.RootNode.FindAllKind(NodeParseOpRef) {
		name := getParseRuleRefName(ref)
		if name == "" {
			continue
		}
		if _, isToken := env.Tokens[name]; isToken {
			validateAndMarkToken(ctx, env, used, ref)
		}
	}
}

func markNestTokenReferences(ctx *ValidationCtx, env *SemanticEnv, used map[string]bool) {
	for _, nestOp := range ctx.RootNode.FindAllKind(NodeParseOpNest) {
		validateAndMarkToken(ctx, env, used, nestOp.FindFirstKind(NodeParseNestOpenToken))
		validateAndMarkToken(ctx, env, used, nestOp.FindFirstKind(NodeParseNestCloseToken))
	}
}

func validateAndMarkToken(ctx *ValidationCtx, env *SemanticEnv, used map[string]bool, tokenNode *Node) {
	if tokenNode == nil {
		return
	}

	name := getIdentifierValue(tokenNode)
	if _, exists := env.Tokens[name]; !exists {
		reportUnresolvedToken(ctx, tokenNode, name)
	} else {
		used[name] = true
	}
}

func reportUnresolvedToken(ctx *ValidationCtx, ref *Node, name string) {
	code := VALIDATION_UNRESOLVED_TOKEN_REF
	if enclosingPrattDef(ref) != nil {
		code = VALIDATION_PRATT_UNRESOLVED_TOKEN
	}
	ctx.ReportError(code.String(), fmt.Sprintf("unresolved token reference '%s'", name), ref)
}

func reportUnusedTokens(ctx *ValidationCtx, env *SemanticEnv, used map[string]bool, ignoredRoles map[string]bool) {
	for name, node := range env.Tokens {
		if used[name] || ignoredRoles[getTokenRole(node)] {
			continue
		}
		ctx.ReportWarning(VALIDATION_TOKEN_UNREFERENCED_IN_PARSE.String(), fmt.Sprintf("token '%s' is never referenced in parse or pratt", name), node)
	}
}

func getIgnoredRoles(ctx *ValidationCtx) map[string]bool {
	ignored := make(map[string]bool)
	parse := ctx.RootNode.FindFirstKind(NodeParseSection)
	if parse == nil {
		return ignored
	}

	ignoreSec := parse.FindFirstKind(NodeParseIgnoreSection)
	if ignoreSec == nil {
		return ignored
	}

	for _, roleNode := range ignoreSec.FindAllKind(NodeParseIgnoreRole) {
		ignored[getIdentifierValue(roleNode)] = true
	}
	return ignored
}

func getTokenRole(tokenNameNode *Node) string {
	parent := tokenNameNode.Parent()
	if parent == nil || parent.Kind() != NodeLexRule {
		return ""
	}

	roleNode := parent.FindFirstKind(NodeLexRuleRole)
	if roleNode == nil {
		return ""
	}

	return getIdentifierValue(roleNode)
}

func validatePatternReferences(ctx *ValidationCtx, env *SemanticEnv) {
	for _, ref := range ctx.RootNode.FindAllKind(NodeVarRef) {
		name := VarRefTargetName(ref)
		if name == "" {
			continue
		}

		if _, exists := env.Patterns[name]; !exists {
			code := VALIDATION_UNRESOLVED_PATTERN_REF
			if enclosingPrattDef(ref) != nil {
				code = VALIDATION_PRATT_UNRESOLVED_PATTERN
			}
			ctx.ReportError(code.String(), fmt.Sprintf("unresolved pattern reference '%s'", name), ref)
		} else {
			if env.LocalPatterns[name] && enclosingPatternDef(ref) == nil {
				ctx.ReportError(VALIDATION_LOCAL_REF_OUTSIDE_SECTION.String(), fmt.Sprintf("local pattern '%s' referenced outside its pattern scope", name), ref)
			}
		}
	}
}

func validateRuleReferences(ctx *ValidationCtx, env *SemanticEnv) {
	if _, ok := env.Rules[programRuleName]; !ok {
		ctx.ReportError(VALIDATION_PROGRAM_RULE_REQUIRED.String(), fmt.Sprintf("parse section must define entry rule '%s'", programRuleName), ctx.RootNode)
	}

	for _, ref := range ctx.RootNode.FindAllKind(NodeParseOpRef) {
		name := getParseRuleRefName(ref)
		if name == "" {
			continue
		}
		_, isRule := env.Rules[name]
		_, isPratt := env.Pratt[name]
		_, isToken := env.Tokens[name]

		if isRule || isPratt {
			if isPratt && env.LocalPratt[name] && enclosingPrattDef(ref) == nil {
				ctx.ReportError(VALIDATION_PRATT_LOCAL_LEAK.String(), fmt.Sprintf("local pratt expression '%s' referenced outside pratt section", name), ref)
			}
			continue
		}
		if isToken {
			continue
		}
		code := VALIDATION_UNRESOLVED_PARSE_RULE_REF
		if enclosingPrattDef(ref) != nil {
			code = VALIDATION_PRATT_UNRESOLVED_RULE
		}
		ctx.ReportError(code.String(), fmt.Sprintf("unresolved parse or pratt rule reference '%s'", name), ref)
	}
}

// ------------------------------------------------------------- REACHABILITY (STAGE 1)

func processReachability(ctx *ValidationCtx) {
	env := BuildSemanticEnv(ctx.RootNode, nil)

	patternDeps := buildPatternDependencyMap(ctx.RootNode, env)
	checkPatternCycles(ctx, env, patternDeps)
	checkPatternReachability(ctx, env, patternDeps)

	parseDeps := buildUnifiedParseDependencyMap(ctx.RootNode, env)
	checkParseReachability(ctx, env, parseDeps)
}

func checkPatternCycles(ctx *ValidationCtx, env *SemanticEnv, deps map[string][]string) {
	for name, node := range env.Patterns {
		if cycle := findCycleInPatternDeps(name, deps); cycle != nil {
			_, nameNode := extractPatternDefName(node)
			ctx.ReportError(VALIDATION_CYCLIC_PATTERN_REF.String(), fmt.Sprintf("cyclic reference detected: %s", formatCycle(cycle)), nameNode)
		}
	}
}

func checkPatternReachability(ctx *ValidationCtx, env *SemanticEnv, deps map[string][]string) {
	reachable := computeReachablePatterns(ctx.RootNode, deps)
	for name, node := range env.Patterns {
		if env.LocalPatterns[name] {
			continue
		}
		if !reachable[name] {
			_, nameNode := extractPatternDefName(node)
			ctx.ReportWarning(VALIDATION_UNREACHABLE_PATTERN.String(), fmt.Sprintf("pattern '%s' is never referenced (unreachable)", name), nameNode)
		}
	}
}

func checkParseReachability(ctx *ValidationCtx, env *SemanticEnv, deps map[string][]string) {
	reachable := computeReachableParseRules(deps, programRuleName)
	if reachable == nil {
		return
	}

	for name, node := range env.Rules {
		if !reachable[name] {
			nameNode := node.FindFirstKind(NodeParseRuleName)
			ctx.ReportWarning(VALIDATION_UNREFERENCED_PARSE_RULE.String(), fmt.Sprintf("parse rule '%s' is never referenced", name), nameNode)
		}
	}
}

// ------------------------------------------------------------- LEX SEMANTICS (STAGE 2)

func processLexSemantics(ctx *ValidationCtx) {
	lexRuleSection := ctx.RootNode.FindFirstKind(NodeLexSection)
	if lexRuleSection != nil {
		validateEOFMetaValues(ctx, lexRuleSection)
	}

	lexRules := collectLexRules(ctx.RootNode)
	patternKeyToRules := groupLexRulesByPattern(lexRules)

	for key, rules := range patternKeyToRules {
		if len(rules) < 2 {
			continue
		}
		reportIdenticalAndAmbiguousPatterns(ctx, key, rules)
		reportShadowedTokens(ctx, rules)
	}
}

func validateEOFMetaValues(ctx *ValidationCtx, lexSection *Node) {
	eofMetaValueNodes := lexRuleSectionCollectEOFTrueMetaValues(lexSection)
	if len(eofMetaValueNodes) == 0 {
		ctx.ReportInfo(VALIDATION_NO_EOF_IN_META.String(), "no lexeme has EOF=true; compiler will inject an EOF token", lexSection)
		return
	}

	if len(eofMetaValueNodes) > 1 {
		for _, node := range eofMetaValueNodes {
			ctx.ReportError(VALIDATION_MULTIPLE_EOF_IN_META.String(), "multiple lexemes have EOF=true; only one allowed", node)
		}
	}
}

func reportIdenticalAndAmbiguousPatterns(ctx *ValidationCtx, key string, rules []lexRuleInfo) {
	for _, r := range rules {
		msgIden := fmt.Sprintf("token '%s' produces the same pattern as other token(s) (pattern key: %s)", r.tokenName, key)
		ctx.ReportWarning(VALIDATION_IDENTICAL_TOKEN_PATTERN.String(), msgIden, r.tokenNameNode)

		msgAmb := fmt.Sprintf("token '%s' can match the same input as other token(s) (ambiguous)", r.tokenName)
		ctx.ReportWarning(VALIDATION_AMBIGUOUS_TOKEN_MATCH.String(), msgAmb, r.patternNode)
	}
}

func reportShadowedTokens(ctx *ValidationCtx, rules []lexRuleInfo) {
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

// ------------------------------------------------------------- PATTERN SEMANTICS (STAGE 3)

func processPatternSemantics(ctx *ValidationCtx) {
	validateNegationNodes(ctx)
	validateRepetitionBounds(ctx)
}

func validateNegationNodes(ctx *ValidationCtx) {
	for _, negNode := range ctx.RootNode.FindAllKind(NodePatternNegation) {
		children := negNode.Children()
		if len(children) == 0 {
			continue
		}
		validateNegationSubtree(children[0], func(offending *Node) {
			ctx.ReportError(VALIDATION_NEGATION_INVALID_CONTENT.String(), "negation (!) may only contain character, range, group, or alternation", offending)
		})
	}
}

func validateRepetitionBounds(ctx *ValidationCtx) {
	for _, repNode := range ctx.RootNode.FindAllKind(NodeRepetition) {
		children := repNode.Children()
		if len(children) < 2 {
			continue
		}

		boundsNode := children[1]
		minNode := boundsNode.FindFirstKind(NodeRepetitionMin)
		maxNode := boundsNode.FindFirstKind(NodeRepetitionMax)

		minVal, minOK := parseIntFromNode(minNode)
		maxVal, maxOK := parseIntFromNode(maxNode)

		if minNode != nil && (!minOK || minVal < 0) {
			ctx.ReportError(VALIDATION_REPETITION_NEGATIVE_BOUND.String(), "repetition min must be a valid non-negative integer", minNode)
		}
		if maxNode != nil && (!maxOK || maxVal < 0) {
			ctx.ReportError(VALIDATION_REPETITION_NEGATIVE_BOUND.String(), "repetition max must be a valid non-negative integer", maxNode)
		}
		if minOK && maxOK && minNode != nil && maxNode != nil && minVal > maxVal {
			msg := fmt.Sprintf("repetition min (%d) must not be greater than max (%d)", minVal, maxVal)
			ctx.ReportError(VALIDATION_REPETITION_MIN_GT_MAX.String(), msg, boundsNode)
		}
	}
}

// ------------------------------------------------------------- PARSER SAFETY (STAGE 4)

func processParseSafety(ctx *ValidationCtx) {
	parseSection := ctx.RootNode.FindFirstKind(NodeParseSection)
	if parseSection == nil {
		return
	}

	ruleBodies := buildParseRuleBodyMap(parseSection)
	if len(ruleBodies) == 0 {
		return
	}

	env := BuildSemanticEnv(ctx.RootNode, nil)
	ruleNullable := computeParseRuleNullable(ruleBodies)
	firstRefs := computeParseRuleFirstRefs(ruleBodies, ruleNullable, env)

	validateLeftRecursion(ctx, parseSection, firstRefs)
	validateUnboundedOptionalRepetition(ctx, parseSection, ruleBodies, ruleNullable)
}

func validateLeftRecursion(ctx *ValidationCtx, parseSection *Node, firstRefs map[string]map[string]struct{}) {
	cycles := findLeftRecursionCycles(firstRefs)
	for _, ruleName := range cycles {
		ruleNode := findParseRuleByName(parseSection, ruleName)
		if ruleNode != nil {
			body := ruleNode.FindFirstKind(NodeParseRuleBody)
			msg := fmt.Sprintf("parse rule '%s' is left-recursive; parser may hang or stack overflow", ruleName)
			ctx.ReportFatal(VALIDATION_PARSE_LEFT_RECURSION.String(), msg, body)
		}
	}
}

func validateUnboundedOptionalRepetition(ctx *ValidationCtx, parseSection *Node, ruleBodies map[string]*Node, ruleNullable map[string]bool) {
	msg := "unbounded repetition of an optional or nullable expression causes an infinite parser loop"

	for _, node := range parseSection.FindAllKind(NodeParseStar) {
		if children := node.Children(); len(children) > 0 && parseExprNullable(children[0], ruleNullable, ruleBodies) {
			ctx.ReportError(VALIDATION_PARSE_UNBOUNDED_OPTIONAL_REPETITION.String(), msg, node)
		}
	}

	for _, node := range parseSection.FindAllKind(NodeParsePlus) {
		if children := node.Children(); len(children) > 0 && parseExprNullable(children[0], ruleNullable, ruleBodies) {
			ctx.ReportError(VALIDATION_PARSE_UNBOUNDED_OPTIONAL_REPETITION.String(), msg, node)
		}
	}
}

// ------------------------------------------------------------- UTILITIES

func getIdentifierValue(node *Node) string {
	tks := node.Tokens()
	if len(tks) != 1 {
		panic("engine error: identifier node must have exactly 1 child")
	}
	return string(tks[0].Raw)
}

func extractPatternDefName(def *Node) (string, *Node) {
	nameNode := def.FindFirstKind(NodePatternDefName)
	if nameNode == nil || len(nameNode.Tokens()) == 0 {
		return "", nil
	}
	return string(nameNode.Tokens()[0].Raw), nameNode
}

func getParseRuleName(nameNode *Node) string {
	return getTrimmedIdentifierNodeContent(nameNode)
}

func getTrimmedIdentifierNodeContent(node *Node) string {
	if node == nil || len(node.Tokens()) == 0 {
		return ""
	}
	return strings.TrimSpace(getIdentifierValue(node))
}

func getParseRuleRefName(refNode *Node) string {
	if refNode == nil || len(refNode.Tokens()) == 0 {
		return ""
	}
	return strings.TrimSpace(getIdentifierValue(refNode))
}

func enclosingPatternDef(node *Node) *Node {
	for n := node; n != nil; n = n.Parent() {
		if n.Kind() == NodePatternDefinition {
			return n
		}
	}
	return nil
}

func enclosingPrattDef(node *Node) *Node {
	for n := node; n != nil; n = n.Parent() {
		if n.Kind() == NodePrattExprDef {
			return n
		}
	}
	return nil
}

func buildPatternDependencyMap(root *Node, env *SemanticEnv) map[string][]string {
	out := make(map[string][]string)
	for _, def := range root.FindAllKind(NodePatternDefinition) {
		name, _ := extractPatternDefName(def)
		if name == "" {
			continue
		}

		var refs []string
		for _, varRef := range def.FindAllKind(NodeVarRef) {
			refName := VarRefTargetName(varRef)
			if refName != "" && env.Patterns[refName] != nil {
				refs = append(refs, refName)
			}
		}
		out[name] = refs
	}
	return out
}

func buildUnifiedParseDependencyMap(root *Node, env *SemanticEnv) map[string][]string {
	out := make(map[string][]string)

	// 1. Process Standard Parse Rules
	parseSection := root.FindFirstKind(NodeParseSection)
	if parseSection != nil {
		for _, rule := range parseSection.FindAllKind(NodeParseRule) {
			name := getParseRuleName(rule.FindFirstKind(NodeParseRuleName))
			if name != "" {
				out[name] = extractAllRuleRefs(rule, env)
			}
		}
	}

	// 2. Process Pratt Expression Definitions
	prattSection := root.FindFirstKind(NodePrattSection)
	if prattSection != nil {
		for _, prattDef := range prattSection.FindAllKind(NodePrattExprDef) {
			name := getIdentifierValue(prattDef.FindFirstKind(NodePrattExprName))
			if name != "" {
				out[name] = extractAllRuleRefs(prattDef, env)
			}
		}
	}

	return out
}

func extractAllRuleRefs(container *Node, env *SemanticEnv) []string {
	var refs []string
	for _, refNode := range container.FindAllKind(NodeParseOpRef) {
		name := getParseRuleRefName(refNode)
		if name == "" {
			continue
		}
		if env.Rules[name] != nil || env.Pratt[name] != nil {
			refs = append(refs, name)
		}
	}
	return refs
}
func computeReachablePatterns(root *Node, deps map[string][]string) map[string]bool {
	entryPoints := make(map[string]bool)
	if lexSection := root.FindFirstKind(NodeLexSection); lexSection != nil {
		for _, ruleNode := range lexSection.FindAllKind(NodeLexRule) {
			if varRef := ruleNode.FindFirstKind(NodeVarRef); varRef != nil {
				if name := VarRefTargetName(varRef); name != "" {
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

func computeReachableParseRules(deps map[string][]string, entryRuleName string) map[string]bool {
	if _, ok := deps[entryRuleName]; !ok {
		return nil
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
	bfs(entryRuleName)
	return reachable
}

func findCycleInPatternDeps(start string, deps map[string][]string) []string {
	path := make(map[string]bool)
	var stack []string
	var cycle []string

	var dfs func(name string) bool
	dfs = func(name string) bool {
		if path[name] {
			for i := range stack {
				if stack[i] == name {
					cycle = append([]string{}, stack[i:]...)
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
	return strings.Join(cycle, " -> ")
}

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
			if strings.EqualFold(key, "EOF") && valueNode.Tokens()[0].Token == TokKWTrue {
				out = append(out, valueNode)
			}
		}
	}
	return out
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
		if tokenNameNode == nil {
			continue
		}
		tokenName, ok := AttributeAs[string](tokenNameNode, ATTRIBUTE_LITERAL_STRING_VALUE)
		if !ok {
			continue
		}

		priority := parsePriority(ruleNode.FindFirstKind(NodeLexRulePriority))
		patternKey, patternNode := extractLexPattern(ruleNode)

		if patternKey != "" && patternNode != nil {
			out = append(out, lexRuleInfo{
				tokenName:     tokenName,
				patternKey:    patternKey,
				priority:      priority,
				tokenNameNode: tokenNameNode,
				patternNode:   patternNode,
			})
		}
	}
	return out
}

func groupLexRulesByPattern(rules []lexRuleInfo) map[string][]lexRuleInfo {
	grouped := make(map[string][]lexRuleInfo)
	for _, r := range rules {
		grouped[r.patternKey] = append(grouped[r.patternKey], r)
	}
	return grouped
}

func parsePriority(priNode *Node) int {
	if priNode != nil {
		if raw := priNode.GetContent(""); raw != "" {
			if n, err := parseIntFromContent(raw); err == nil {
				return n
			}
		}
	}
	return 0
}

func extractLexPattern(ruleNode *Node) (string, *Node) {
	if varRef := ruleNode.FindFirstKind(NodeVarRef); varRef != nil {
		if name := VarRefTargetName(varRef); name != "" {
			return "ref:" + name, varRef
		}
	}
	if regexNode := ruleNode.FindFirstKind(NodeLexRulePattern); regexNode != nil {
		return "regex:" + regexNode.GetContent(""), regexNode
	}
	return "", nil
}

func parseIntFromContent(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func parseIntFromNode(node *Node) (int, bool) {
	if node == nil || len(node.Tokens()) != 1 {
		return 0, false
	}
	raw := strings.TrimSpace(string(node.Tokens()[0].Raw))
	n, err := parseIntFromContent(raw)
	return n, err == nil
}

func negationAllowedKind(kind LangSpecParserNodeKind) bool {
	switch kind {
	case NodeCharLiteral, NodePatternRange, NodePatternGroup, NodePatternAlternation, NodePatternSegment:
		return true
	default:
		return false
	}
}

func validateNegationSubtree(node *Node, report func(offending *Node)) {
	kind := node.Kind()
	if !negationAllowedKind(kind) {
		report(node)
	}
	switch kind {
	case NodeCharLiteral, NodePatternRange:
		return
	default:
		for _, ch := range node.Children() {
			validateNegationSubtree(ch, report)
		}
	}
}

func getParseRuleBodyRoot(body *Node) *Node {
	if body == nil {
		return nil
	}
	for _, ch := range body.Children() {
		if ch == nil {
			continue
		}
		k := ch.Kind()
		if k == NodeParseAlternation || k == NodeParseConcat || k == NodeParseOptional ||
			k == NodeParseStar || k == NodeParsePlus || k == NodeParseSegment || k == NodeParseGroup {
			return ch
		}
	}
	return nil
}

func buildParseRuleBodyMap(parseSection *Node) map[string]*Node {
	out := make(map[string]*Node)
	for _, rule := range parseSection.FindAllKind(NodeParseRule) {
		nameNode := rule.FindFirstKind(NodeParseRuleName)
		name := getParseRuleName(nameNode)
		if name == "" {
			continue
		}

		body := rule.FindFirstKind(NodeParseRuleBody)
		if body != nil {
			if root := getParseRuleBodyRoot(body); root != nil {
				out[name] = root
			}
		}
	}
	return out
}

func parseExprNullable(node *Node, ruleNullable map[string]bool, ruleBodies map[string]*Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case NodeParseOptional, NodeParseStar:
		return true
	case NodeParsePlus, NodeParseGroup:
		children := node.Children()
		if len(children) == 0 {
			return false
		}
		return parseExprNullable(children[0], ruleNullable, ruleBodies)
	case NodeParseAlternation:
		for _, ch := range node.Children() {
			if parseExprNullable(ch, ruleNullable, ruleBodies) {
				return true
			}
		}
		return false
	case NodeParseConcat:
		for _, ch := range node.Children() {
			if !parseExprNullable(ch, ruleNullable, ruleBodies) {
				return false
			}
		}
		return true
	case NodeParseOpRef:
		return ruleNullable[getParseRuleRefName(node)]
	case NodeParseSegment:
		if children := node.Children(); len(children) > 0 {
			ch := children[0]
			if ch.Kind() == NodeParseOpRef || ch.Kind() == NodeParseGroup {
				return parseExprNullable(ch, ruleNullable, ruleBodies)
			}
		}
		return false
	default:
		return false
	}
}

func parseExprFirstRuleRefs(node *Node, ruleNullable map[string]bool, ruleBodies map[string]*Node, env *SemanticEnv) map[string]struct{} {
	out := make(map[string]struct{})
	if node == nil {
		return out
	}
	switch node.Kind() {
	case NodeParseOpRef:
		if name := getParseRuleRefName(node); name != "" && (env.Rules[name] != nil || env.Pratt[name] != nil) {
			out[name] = struct{}{}
		}
	case NodeParseOptional, NodeParseStar, NodeParsePlus, NodeParseGroup:
		if children := node.Children(); len(children) > 0 {
			for k := range parseExprFirstRuleRefs(children[0], ruleNullable, ruleBodies, env) {
				out[k] = struct{}{}
			}
		}
	case NodeParseAlternation:
		for _, ch := range node.Children() {
			for k := range parseExprFirstRuleRefs(ch, ruleNullable, ruleBodies, env) {
				out[k] = struct{}{}
			}
		}
	case NodeParseConcat:
		for _, ch := range node.Children() {
			for k := range parseExprFirstRuleRefs(ch, ruleNullable, ruleBodies, env) {
				out[k] = struct{}{}
			}
			if !parseExprNullable(ch, ruleNullable, ruleBodies) {
				break
			}
		}
	case NodeParseSegment:
		if children := node.Children(); len(children) > 0 {
			ch := children[0]
			if ch.Kind() == NodeParseOpRef || ch.Kind() == NodeParseGroup {
				return parseExprFirstRuleRefs(ch, ruleNullable, ruleBodies, env)
			}
		}
	}
	return out
}

func computeParseRuleNullable(ruleBodies map[string]*Node) map[string]bool {
	nullable := make(map[string]bool)
	for name := range ruleBodies {
		nullable[name] = false
	}
	for {
		changed := false
		for name, root := range ruleBodies {
			prev := nullable[name]
			nullable[name] = parseExprNullable(root, nullable, ruleBodies)
			if nullable[name] != prev {
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return nullable
}

func computeParseRuleFirstRefs(ruleBodies map[string]*Node, ruleNullable map[string]bool, env *SemanticEnv) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{})
	for name, root := range ruleBodies {
		out[name] = parseExprFirstRuleRefs(root, ruleNullable, ruleBodies, env)
	}
	return out
}

func findLeftRecursionCycles(firstRefs map[string]map[string]struct{}) []string {
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	cycleSet := make(map[string]struct{})

	var dfs func(name string)
	dfs = func(name string) {
		visited[name] = true
		inStack[name] = true
		for ref := range firstRefs[name] {
			if !visited[ref] {
				dfs(ref)
			} else if inStack[ref] {
				cycleSet[ref] = struct{}{}
			}
		}
		inStack[name] = false
	}

	for name := range firstRefs {
		if !visited[name] {
			dfs(name)
		}
	}

	var cycles []string
	for name := range cycleSet {
		cycles = append(cycles, name)
	}
	return cycles
}

func findParseRuleByName(parseSection *Node, ruleName string) *Node {
	for _, rule := range parseSection.FindAllKind(NodeParseRule) {
		nameNode := rule.FindFirstKind(NodeParseRuleName)
		if getParseRuleName(nameNode) == ruleName {
			return rule
		}
	}
	return nil
}
