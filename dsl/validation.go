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

	VALIDATION_NEGATION_INVALID_CONTENT  ValidationCode = "V_PAT007"
	VALIDATION_REPETITION_MIN_GT_MAX     ValidationCode = "V_PAT008"
	VALIDATION_REPETITION_NEGATIVE_BOUND ValidationCode = "V_PAT009"

	VALIDATION_DUPLICATE_PARSE_RULE_NAME ValidationCode = "V_PAR001"
	VALIDATION_UNRESOLVED_PARSE_RULE_REF ValidationCode = "V_PAR002"
	VALIDATION_UNREFERENCED_PARSE_RULE   ValidationCode = "V_PAR003"
	VALIDATION_PROGRAM_RULE_REQUIRED     ValidationCode = "V_PAR004"

	VALIDATION_PARSE_LEFT_RECURSION                ValidationCode = "V_PAR005"
	VALIDATION_PARSE_UNBOUNDED_OPTIONAL_REPETITION ValidationCode = "V_PAR006"
)

func (v ValidationCode) String() string {
	return string(v)
}

const programRuleName = "PROGRAM"

// ------------------------------------------------------------- STAGES REGISTRATION

func getValidationStages() []*ValidationStage {
	return []*ValidationStage{
		{
			Name:        "Lex Rule Validation",
			Description: "Validates the lex rule section for duplicates and EOF metadata.",
			Order:       0,
			Processor:   processLexRuleValidation,
		},
		{
			Name:        "Pattern Validation",
			Description: "Validates pattern section logic, bounds, scoping, and references.",
			Order:       1,
			Processor:   processPatternValidation,
		},
		{
			Name:        "Parse Rule Validation",
			Description: "Validates parse section references, duplication, and reachability.",
			Order:       2,
			Processor:   processParseRuleValidation,
		},
		{
			Name:        "Parse Rule Safety",
			Description: "Detects infinite loops in the parse section.",
			Order:       3,
			Processor:   processParseRuleSafety,
		},
		{
			Name:        "Unreachable Patterns",
			Description: "Emits warnings for unused pattern declarations.",
			Order:       4,
			Processor:   processUnreachablePatterns,
		},
		{
			Name:        "Lex Pattern Analysis",
			Description: "Detects ambiguous token matches and shadowing.",
			Order:       5,
			Processor:   processLexPatternAnalysis,
		},
	}
}

// ------------------------------------------------------------- STAGE PROCESSORS

func processLexRuleValidation(ctx *ValidationCtx) {
	lexRuleSection := ctx.RootNode.FindFirstKind(NodeLexSection)
	if lexRuleSection == nil {
		return
	}

	validateDuplicateTokens(ctx, lexRuleSection)
	validateEOFMetaValues(ctx, lexRuleSection)
}

func processPatternValidation(ctx *ValidationCtx) {
	defs := ctx.RootNode.FindAllKind(NodePatternDefinition)

	allDeclared, localNameToDef := buildPatternDeclarationMaps(defs)
	declaredSoFar := validateDuplicatePatterns(ctx, defs)

	validatePatternReferences(ctx, defs, declaredSoFar, allDeclared)
	validateLocalVarScoping(ctx, localNameToDef)
	validatePatternCycles(ctx, defs, declaredSoFar)
	validateNegationNodes(ctx)
	validateRepetitionBounds(ctx)
}

func processParseRuleValidation(ctx *ValidationCtx) {
	parseSection := ctx.RootNode.FindFirstKind(NodeParseSection)
	if parseSection == nil {
		return
	}

	parseRules := parseSection.FindAllKind(NodeParseRule)
	allDeclared := collectDeclaredParseRules(parseRules)

	validateProgramRulePresence(ctx, parseSection, allDeclared)
	declaredSoFar := validateDuplicateParseRules(ctx, parseRules)
	validateParseRuleReferences(ctx, parseRules, declaredSoFar, allDeclared)

	parseRuleDeps := buildParseRuleDependencyMap(parseSection)
	validateReachableParseRules(ctx, parseSection, parseRules, parseRuleDeps)
	validateUnreferencedTokens(ctx, parseSection)
}

func processParseRuleSafety(ctx *ValidationCtx) {
	parseSection := ctx.RootNode.FindFirstKind(NodeParseSection)
	if parseSection == nil {
		return
	}

	ruleBodies := buildParseRuleBodyMap(parseSection)
	if len(ruleBodies) == 0 {
		return
	}

	ruleNullable := computeParseRuleNullable(ruleBodies)
	firstRefs := computeParseRuleFirstRefs(ruleBodies, ruleNullable)

	validateLeftRecursion(ctx, parseSection, firstRefs)
	validateUnboundedOptionalRepetition(ctx, parseSection, ruleBodies, ruleNullable)
}

func processUnreachablePatterns(ctx *ValidationCtx) {
	patternDeps := buildPatternDependencyMap(ctx.RootNode, nil)
	reachable := computeReachablePatterns(ctx.RootNode, patternDeps)

	for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
		name, nameNode := extractPatternDefName(def)
		if nameNode == nil {
			continue
		}

		if def.FindFirstKind(NodeLocalVariable) != nil {
			continue
		}

		if !reachable[name] {
			msg := fmt.Sprintf("pattern '%s' is never referenced (unreachable)", name)
			// Escalated to Warning. Dead patterns pollute the syntax tree.
			ctx.ReportWarning(VALIDATION_UNREACHABLE_PATTERN.String(), msg, nameNode)
		}
	}
}

func processLexPatternAnalysis(ctx *ValidationCtx) {
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

// ------------------------------------------------------------- LOGIC EXTRACTS: LEX

func validateDuplicateTokens(ctx *ValidationCtx, lexSection *Node) {
	tks := make(map[string]struct{})
	for _, lexRuleToken := range lexSection.FindAllKind(NodeLexRuleTokenName) {
		value := getIdentifierValue(lexRuleToken)
		if _, seen := tks[value]; seen {
			msg := fmt.Sprintf("token '%s' already declared (duplicate entry)", value)
			ctx.ReportError(VALIDATION_DUPLICATE_TOKEN.String(), msg, lexRuleToken)
			continue
		}
		tks[value] = struct{}{}
	}
}

func validateEOFMetaValues(ctx *ValidationCtx, lexSection *Node) {
	eofMetaValueNodes := lexRuleSectionCollectEOFTrueMetaValues(lexSection)
	if len(eofMetaValueNodes) == 0 {
		msg := "no lexeme has EOF=true in its meta section; the compiler will automatically inject an EOF token"
		ctx.ReportInfo(VALIDATION_NO_EOF_IN_META.String(), msg, lexSection)
		return
	}

	if len(eofMetaValueNodes) > 1 {
		for _, node := range eofMetaValueNodes {
			msg := "multiple lexemes have EOF=true in their meta section; only one is allowed"
			ctx.ReportError(VALIDATION_MULTIPLE_EOF_IN_META.String(), msg, node)
		}
	}
}

func groupLexRulesByPattern(rules []lexRuleInfo) map[string][]lexRuleInfo {
	grouped := make(map[string][]lexRuleInfo)
	for _, r := range rules {
		grouped[r.patternKey] = append(grouped[r.patternKey], r)
	}
	return grouped
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

// ------------------------------------------------------------- LOGIC EXTRACTS: PATTERN

func extractPatternDefName(def *Node) (string, *Node) {
	nameNode := def.FindFirstKind(NodePatternDefName)
	if nameNode == nil || len(nameNode.Tokens()) == 0 {
		return "", nil
	}
	return string(nameNode.Tokens()[0].Raw), nameNode
}

func buildPatternDeclarationMaps(defs []*Node) (map[string]struct{}, map[string]*Node) {
	allDeclared := make(map[string]struct{})
	localNameToDef := make(map[string]*Node)

	for _, def := range defs {
		name, nameNode := extractPatternDefName(def)
		if nameNode == nil {
			continue
		}
		allDeclared[name] = struct{}{}
		if def.FindFirstKind(NodeLocalVariable) != nil {
			localNameToDef[name] = def
		}
	}
	return allDeclared, localNameToDef
}

func validateDuplicatePatterns(ctx *ValidationCtx, defs []*Node) map[string]struct{} {
	declaredSoFar := make(map[string]struct{})
	for _, def := range defs {
		name, nameNode := extractPatternDefName(def)
		if nameNode == nil {
			continue
		}

		if _, already := declaredSoFar[name]; already {
			msg := fmt.Sprintf("pattern name '%s' already declared (duplicate)", name)
			ctx.ReportError(VALIDATION_DUPLICATE_PATTERN_NAME.String(), msg, nameNode)
		} else {
			declaredSoFar[name] = struct{}{}
		}
	}
	return declaredSoFar
}

func validatePatternReferences(ctx *ValidationCtx, defs []*Node, declaredSoFar, allDeclared map[string]struct{}) {
	for _, def := range defs {
		for _, varRef := range def.FindAllKind(NodeVarRef) {
			tokens := varRef.Tokens()
			if len(tokens) < 2 {
				continue
			}
			refName := string(tokens[1].Raw)

			if _, ok := declaredSoFar[refName]; ok {
				continue
			}

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

func validateLocalVarScoping(ctx *ValidationCtx, localNameToDef map[string]*Node) {
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
			msg := fmt.Sprintf("local variable '%s' referenced outside pattern section", refName)
			ctx.ReportError(VALIDATION_LOCAL_REF_OUTSIDE_SECTION.String(), msg, varRef)
		}
	}
}

func validatePatternCycles(ctx *ValidationCtx, defs []*Node, declaredSoFar map[string]struct{}) {
	patternDeps := buildPatternDependencyMap(ctx.RootNode, declaredSoFar)
	for _, def := range defs {
		name, nameNode := extractPatternDefName(def)
		if nameNode == nil {
			continue
		}

		cycle := findCycleInPatternDeps(name, patternDeps)
		if cycle != nil {
			msg := fmt.Sprintf("pattern '%s' has cyclic reference (e.g. %s)", name, formatCycle(cycle))
			ctx.ReportError(VALIDATION_CYCLIC_PATTERN_REF.String(), msg, nameNode)
		}
	}
}

func validateNegationNodes(ctx *ValidationCtx) {
	for _, negNode := range ctx.RootNode.FindAllKind(NodePatternNegation) {
		children := negNode.Children()
		if len(children) == 0 {
			continue
		}
		validateNegationSubtree(children[0], func(offending *Node) {
			msg := "negation (!) may only contain character, range, group, or alternation"
			ctx.ReportError(VALIDATION_NEGATION_INVALID_CONTENT.String(), msg, offending)
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

		reportRepetitionErrors(ctx, boundsNode, minNode, maxNode, minVal, maxVal, minOK, maxOK)
	}
}

func reportRepetitionErrors(ctx *ValidationCtx, boundsNode, minNode, maxNode *Node, minVal, maxVal int, minOK, maxOK bool) {
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

// ------------------------------------------------------------- LOGIC EXTRACTS: PARSE

func collectDeclaredParseRules(parseRules []*Node) map[string]struct{} {
	allDeclared := make(map[string]struct{})
	for _, rule := range parseRules {
		nameNode := rule.FindFirstKind(NodeParseRuleName)
		if name := getParseRuleName(nameNode); name != "" {
			allDeclared[name] = struct{}{}
		}
	}
	return allDeclared
}

func validateProgramRulePresence(ctx *ValidationCtx, parseSection *Node, allDeclared map[string]struct{}) {
	if _, hasProgram := allDeclared[programRuleName]; !hasProgram {
		msg := fmt.Sprintf("parse section must define a rule named '%s' (entry point)", programRuleName)
		ctx.ReportError(VALIDATION_PROGRAM_RULE_REQUIRED.String(), msg, parseSection)
	}
}

func validateDuplicateParseRules(ctx *ValidationCtx, parseRules []*Node) map[string]struct{} {
	declaredSoFar := make(map[string]struct{})
	for _, rule := range parseRules {
		nameNode := rule.FindFirstKind(NodeParseRuleName)
		name := getParseRuleName(nameNode)
		if name == "" {
			continue
		}

		if _, already := declaredSoFar[name]; already {
			msg := fmt.Sprintf("parse rule '%s' already declared (duplicate)", name)
			ctx.ReportError(VALIDATION_DUPLICATE_PARSE_RULE_NAME.String(), msg, nameNode)
		} else {
			declaredSoFar[name] = struct{}{}
		}
	}
	return declaredSoFar
}

func validateParseRuleReferences(ctx *ValidationCtx, parseRules []*Node, declaredSoFar, allDeclared map[string]struct{}) {
	for _, rule := range parseRules {
		body := rule.FindFirstKind(NodeParseRuleBody)
		if body == nil {
			continue
		}

		for _, refNode := range body.FindAllKind(NodeParseRuleReference) {
			refName := getParseRuleRefName(refNode)
			if refName == "" {
				continue
			}

			if _, ok := declaredSoFar[refName]; ok {
				continue
			}

			var msg string
			if _, declaredLater := allDeclared[refName]; declaredLater {
				msg = fmt.Sprintf("parse rule reference '%s' used before declaration", refName)
			} else {
				msg = fmt.Sprintf("unresolved parse rule reference '%s'", refName)
			}
			ctx.ReportError(VALIDATION_UNRESOLVED_PARSE_RULE_REF.String(), msg, refNode)
		}
	}
}

func validateReachableParseRules(ctx *ValidationCtx, parseSection *Node, parseRules []*Node, parseRuleDeps map[string][]string) {
	reachableParse := computeReachableParseRules(parseSection, parseRuleDeps, programRuleName)
	if reachableParse == nil {
		return
	}

	for _, rule := range parseRules {
		nameNode := rule.FindFirstKind(NodeParseRuleName)
		name := getParseRuleName(nameNode)
		if name != "" && !reachableParse[name] {
			msg := fmt.Sprintf("parse rule '%s' is never referenced (unreachable)", name)
			ctx.ReportInfo(VALIDATION_UNREFERENCED_PARSE_RULE.String(), msg, nameNode)
		}
	}
}

func validateUnreferencedTokens(ctx *ValidationCtx, parseSection *Node) {
	lexSection := ctx.RootNode.FindFirstKind(NodeLexSection)
	if lexSection == nil {
		return
	}

	referencedTokens := collectTokenNamesReferencedInParseSection(parseSection)
	for _, lexRuleToken := range lexSection.FindAllKind(NodeLexRuleTokenName) {
		if len(lexRuleToken.Tokens()) != 1 {
			continue
		}

		tokenName := strings.TrimSpace(string(lexRuleToken.Tokens()[0].Raw))
		if tokenName == "" {
			continue
		}

		if _, referenced := referencedTokens[tokenName]; !referenced {
			msg := fmt.Sprintf("token '%s' is defined in LEX but not referenced in the parse section", tokenName)
			// Escalated to Warning. Dead tokens clutter the lexer output.
			ctx.ReportWarning(VALIDATION_TOKEN_UNREFERENCED_IN_PARSE.String(), msg, lexRuleToken)
		}
	}
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
			// Escalated to Error. This guarantees a runtime failure.
			ctx.ReportError(VALIDATION_PARSE_UNBOUNDED_OPTIONAL_REPETITION.String(), msg, node)
		}
	}

	for _, node := range parseSection.FindAllKind(NodeParsePlus) {
		if children := node.Children(); len(children) > 0 && parseExprNullable(children[0], ruleNullable, ruleBodies) {
			// Escalated to Error.
			ctx.ReportError(VALIDATION_PARSE_UNBOUNDED_OPTIONAL_REPETITION.String(), msg, node)
		}
	}
}

// ------------------------------------------------------------- UTILITIES / EXISTING HELPER FUNCS

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

func getIdentifierValue(node *Node) string {
	tks := node.Tokens()
	if len(tks) != 1 {
		panic("engine error: identifier node must have exactly 1 child")
	}
	return string(tks[0].Raw)
}

func getParseRuleName(nameNode *Node) string {
	if nameNode == nil || len(nameNode.Tokens()) == 0 {
		return ""
	}
	return strings.TrimSpace(getIdentifierValue(nameNode))
}

func getParseRuleRefName(refNode *Node) string {
	if refNode == nil || len(refNode.Tokens()) == 0 {
		return ""
	}
	return strings.TrimSpace(getIdentifierValue(refNode))
}

func buildParseRuleDependencyMap(parseSection *Node) map[string][]string {
	out := make(map[string][]string)
	for _, rule := range parseSection.FindAllKind(NodeParseRule) {
		nameNode := rule.FindFirstKind(NodeParseRuleName)
		name := getParseRuleName(nameNode)
		if name == "" {
			continue
		}

		body := rule.FindFirstKind(NodeParseRuleBody)
		if body == nil {
			out[name] = nil
			continue
		}

		var refs []string
		for _, refNode := range body.FindAllKind(NodeParseRuleReference) {
			if refName := getParseRuleRefName(refNode); refName != "" {
				refs = append(refs, refName)
			}
		}
		out[name] = refs
	}
	return out
}

func computeReachableParseRules(parseSection *Node, deps map[string][]string, entryRuleName string) map[string]bool {
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
	case NodeParseRuleReference:
		return ruleNullable[getParseRuleRefName(node)]
	case NodeParseSegment:
		if children := node.Children(); len(children) > 0 {
			ch := children[0]
			if ch.Kind() == NodeParseOpRef || ch.Kind() == NodeParseGroup {
				return parseExprNullable(ch, ruleNullable, ruleBodies)
			}
		}
		return false
	case NodeParseOpRef:
		if refNode := node.FindFirstKind(NodeParseRuleReference); refNode != nil {
			return ruleNullable[getParseRuleRefName(refNode)]
		}
		return false
	default:
		return false
	}
}

func parseExprFirstRuleRefs(node *Node, ruleNullable map[string]bool, ruleBodies map[string]*Node) map[string]struct{} {
	out := make(map[string]struct{})
	if node == nil {
		return out
	}
	switch node.Kind() {
	case NodeParseRuleReference:
		if name := getParseRuleRefName(node); name != "" {
			out[name] = struct{}{}
		}
	case NodeParseOptional, NodeParseStar, NodeParsePlus, NodeParseGroup:
		if children := node.Children(); len(children) > 0 {
			for k := range parseExprFirstRuleRefs(children[0], ruleNullable, ruleBodies) {
				out[k] = struct{}{}
			}
		}
	case NodeParseAlternation:
		for _, ch := range node.Children() {
			for k := range parseExprFirstRuleRefs(ch, ruleNullable, ruleBodies) {
				out[k] = struct{}{}
			}
		}
	case NodeParseConcat:
		for _, ch := range node.Children() {
			for k := range parseExprFirstRuleRefs(ch, ruleNullable, ruleBodies) {
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
				return parseExprFirstRuleRefs(ch, ruleNullable, ruleBodies)
			}
		}
	case NodeParseOpRef:
		if refNode := node.FindFirstKind(NodeParseRuleReference); refNode != nil {
			if name := getParseRuleRefName(refNode); name != "" {
				out[name] = struct{}{}
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

func computeParseRuleFirstRefs(ruleBodies map[string]*Node, ruleNullable map[string]bool) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{})
	for name, root := range ruleBodies {
		out[name] = parseExprFirstRuleRefs(root, ruleNullable, ruleBodies)
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

func collectTokenNamesReferencedInParseSection(parseSection *Node) map[string]struct{} {
	out := make(map[string]struct{})
	extractName := func(n *Node) {
		if len(n.Tokens()) == 1 {
			if name := strings.TrimSpace(string(n.Tokens()[0].Raw)); name != "" {
				out[name] = struct{}{}
			}
		}
	}

	for _, n := range parseSection.FindAllKind(NodeParseTokenReference) {
		extractName(n)
	}
	for _, n := range parseSection.FindAllKind(NodeIdentifier) {
		extractName(n)
	}
	return out
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

func enclosingPatternDef(node *Node) *Node {
	for n := node; n != nil; n = n.Parent() {
		if n.Kind() == NodePatternDefinition {
			return n
		}
	}
	return nil
}

func getVarRefTargetName(varRef *Node) string {
	if targetNode := varRef.FindFirstKind(NodeVarRefTarget); targetNode != nil && len(targetNode.Tokens()) > 0 {
		return strings.TrimSpace(string(targetNode.Tokens()[0].Raw))
	}
	if tokens := varRef.Tokens(); len(tokens) >= 2 {
		return strings.TrimSpace(string(tokens[1].Raw))
	}
	return ""
}

func buildPatternDependencyMap(root *Node, declaredOnly map[string]struct{}) map[string][]string {
	out := make(map[string][]string)
	for _, def := range root.FindAllKind(NodePatternDefinition) {
		name, _ := extractPatternDefName(def)
		if name == "" {
			continue
		}

		var refs []string
		for _, varRef := range def.FindAllKind(NodeVarRef) {
			refName := getVarRefTargetName(varRef)
			if refName == "" || (declaredOnly != nil && !mapContains(declaredOnly, refName)) {
				continue
			}
			refs = append(refs, refName)
		}
		out[name] = refs
	}
	return out
}

func mapContains(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
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

func computeReachablePatterns(root *Node, deps map[string][]string) map[string]bool {
	entryPoints := make(map[string]bool)
	if lexSection := root.FindFirstKind(NodeLexSection); lexSection != nil {
		for _, ruleNode := range lexSection.FindAllKind(NodeLexRule) {
			if varRef := ruleNode.FindFirstKind(NodeVarRef); varRef != nil {
				if name := getVarRefTargetName(varRef); name != "" {
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
		if name := getVarRefTargetName(varRef); name != "" {
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
