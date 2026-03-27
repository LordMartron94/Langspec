package dsl

import (
	"autarch/pattern"
	"fmt"
	"langspec"
	"langspec/dsl/semantics"
	dslspec "langspec/dsl/spec"
	"lexarch"
	"strconv"
	"strings"
	"syntaxa"
	"syntaxa/lowering"
	"syntaxa/rule"
)

type LexerSpec = langspec.LexerSpec[rune, string, string, string]
type ParserSpec = langspec.ParserSpec[rune, string, string, string, string]
type RuleRegistry = syntaxa.RuleRegistry[rune, string, string, string, string]

type LexerRuleset = lexarch.LexingRuleset[rune, string, string]

type CompiledRule = syntaxa.ParserRule[rune, string, string, string, string]
type CompilerRuleBuilder = rule.RuleBuilder[rune, string, string, string, string]

/* ToolPragma is one parsed PRAGMA toolchain line: tool name and key/value settings. */
type ToolPragma struct {
	ToolName string
	Settings map[string]any // Values will be string, bool, or []string
}

/* CompiledLangSpec is compileTree output: lexer/parser specs, lowered grammar, EOF token, and pragmas. */
type CompiledLangSpec struct {
	dslName    string
	dslVersion string

	targetLangspecVersion string

	lexerSpec  *LexerSpec
	parserSpec *ParserSpec

	grammarPackage GrammarPackage

	toolPragmas []ToolPragma

	eofToken  string
	sourceMap map[*syntaxa.Grammar[string, string]]*Node
}

type compiler struct {
	factory *pattern.RegulaASTFactory[rune]
}

/*
patternCompileCtx holds shared state for compiling the pattern section (regex/varrefs)
into Regula ASTs. Passed through instead of (c *compiler, patternTable, env) on every call.
variables is the running map during pattern compilation; patternTable is the exported result.
*/
type patternCompileCtx struct {
	c            *compiler
	rootNode     *Node
	patternTable map[string]pattern.RegulaAST[rune]
	env          *SemanticEnv
	variables    map[string]pattern.RegulaAST[rune]
}

/*
parseCompileCtx holds shared state for compiling parse rules into grammar rules.
Passed through instead of (builder, env, grammarID, nodeKind, ruleName, counts, rootLevel, transparent).
*/
type parseCompileCtx struct {
	builder     *CompilerRuleBuilder
	env         *SemanticEnv
	grammarID   syntaxa.GrammarLabel
	nodeKind    string
	ruleName    string
	counts      map[string]int
	rootLevel   bool
	transparent bool

	sourceMap map[*syntaxa.Grammar[string, string]]*Node
}

func compileTree(comp *LangSpecCompiler, rootNode *Node) *CompiledLangSpec {
	dslName, dslVersion, langspecTargetVersion := getInfoFromHeader(rootNode.FindFirstKind(dslspec.NodeHeader))

	env := semantics.BuildSemanticEnv(rootNode, nil)

	eofToken := getEOFToken(rootNode)
	domain := lexarch.LexarchRuneDomain()
	lexerSpec := langspec.LexerSpecCreate[rune, string, string](
		eofToken,
		"default",
		lexarch.NewlineDetectorRune(),
		lexarch.ColumnAdvanceRune(4),
		lexarch.RunesToBytesDefault(),
		lexarch.RuneFormatterDefault(),
		domain,
		func(token string) string {
			return token
		},
	)
	lexerSpec.WithCompilationMode(lexarch.Glushkov)
	lexerSpec.WithScanConfig(comp.config.lexerScanConfig)
	switch comp.config.lexerPositionTracking {
	case LangSpecLexerPositionTrackingGeneric:
		// LexerSpecCreate leaves generic position mode.
	case LangSpecLexerPositionTrackingRuneFast:
		lexerSpec.WithRunePositionTrackingFast(comp.config.lexerRuneTabWidth)
	default:
		lexerSpec.WithRunePositionTrackingFast(4)
	}

	lspecCompiler := compiler{
		factory: pattern.RegulaASTFactoryCreate(domain),
	}
	patternCtx := &patternCompileCtx{
		c:        &lspecCompiler,
		rootNode: rootNode,
		env:      env,
	}
	lspecCompiler.compilePatterns(patternCtx)

	ruleset := lspecCompiler.compileRuleset(patternCtx)
	lexerSpec.WithRuleset("default", *ruleset)

	grammarPackage, ruleRegistry, rootNodeKind, skipRoles, sourceMap := getParserSpecInfo(rootNode, env, dslName, dslVersion)

	grammarPkg := new(syntaxa.GrammarPackage[rune, string, string, string, string])
	*grammarPkg = grammarPackage

	analysis := lowering.GetAnalysis(grammarPkg)
	getAnalysis := func() *syntaxa.GrammarAnalysis[string] { return analysis }
	parserSpec := langspec.ParserSpecCreate(
		grammarPkg,
		ruleRegistry,
		rootNodeKind,
		"ERROR_NODE",
		false, // TODO allow configuration of this flag inside LSpec
		getAnalysis,
	)
	parserSpec.WithSkipRoles(skipRoles...)

	toolPragmas := extractPragmas(rootNode)

	return &CompiledLangSpec{
		dslName:               dslName,
		dslVersion:            dslVersion,
		lexerSpec:             lexerSpec,
		parserSpec:            parserSpec,
		eofToken:              eofToken,
		grammarPackage:        *grammarPkg,
		toolPragmas:           toolPragmas,
		targetLangspecVersion: langspecTargetVersion,
		sourceMap:             sourceMap,
	}
}

func getInfoFromHeader(headerNode *Node) (string, string, string) {
	dslName := nodeFormattedContent(headerNode.FindFirstKind(dslspec.NodeDSLName), dslspec.ATTRIBUTE_LITERAL_STRING_VALUE)

	versions := headerNode.FindAllKind(dslspec.NodeVersion)
	dslVersion := lexemeRawContent(versions[0].Tokens()[0])
	langSpecTargetVersion := lexemeRawContent(versions[1].Tokens()[0])

	return dslName, dslVersion, langSpecTargetVersion
}

func getEOFToken(rootNode *Node) string {
	lexerSection := rootNode.FindFirstKind(dslspec.NodeLexSection)

	eofToken := checkForEOFLexeme(lexerSection)
	if eofToken == "" {
		eofToken = "EOF_INJECTED"
	}

	return eofToken
}

func extractPragmas(rootNode *Node) []ToolPragma {
	section := rootNode.FindFirstKind(dslspec.NodePragmaSection)
	if section == nil {
		return nil
	}

	var out []ToolPragma
	for _, block := range section.FindAllKind(dslspec.NodePragmaBlock) {
		if pragma, valid := extractSinglePragmaBlock(block); valid {
			out = append(out, pragma)
		}
	}
	return out
}

func extractSinglePragmaBlock(block *Node) (ToolPragma, bool) {
	keyNode := block.FindFirstKind(dslspec.NodePragmaBlockKey)
	if keyNode == nil {
		return ToolPragma{}, false
	}

	toolName, valid := extractToolName(keyNode)
	if !valid {
		return ToolPragma{}, false
	}

	settings := extractPragmaSettings(block)
	return ToolPragma{
		ToolName: toolName,
		Settings: settings,
	}, true
}

func extractToolName(keyNode *Node) (string, bool) {
	prefixNode := keyNode.FindFirstKind(dslspec.NodePragmaBlockKeyPrefix)
	if prefixNode == nil || dslspec.TrimmedIdentifierNodeContent(prefixNode) != "tool" {
		return "", false
	}

	segments := keyNode.FindAllKind(dslspec.NodePragmaBlockKeySegment)
	if len(segments) != 1 {
		panic(fmt.Errorf("tool pragma key must have exactly 1 segment, got=%d", len(segments)))
	}

	return dslspec.TrimmedIdentifierNodeContent(segments[0]), true
}

func extractPragmaSettings(block *Node) map[string]any {
	settings := make(map[string]any)
	for _, config := range block.FindAllKind(dslspec.NodePragmaConfiguration) {
		key := dslspec.TrimmedIdentifierNodeContent(config.FindFirstKind(dslspec.NodePragmaKey))
		valueNode := config.FindFirstKind(dslspec.NodePragmaValue)

		settings[key] = extractPragmaValue(valueNode)
	}
	return settings
}

func extractPragmaValue(valueNode *Node) any {
	// 1. Check if the value wraps a string array
	arrayNode := valueNode.FindFirstKind(dslspec.NodeStringArray)
	if arrayNode != nil {
		return extractStringArray(arrayNode)
	}

	// 2. Handle scalar string literals
	token := valueNode.Tokens()[0].Token
	if token == dslspec.TokStringLiteral {
		val, exist := AttributeAs[string](valueNode, dslspec.ATTRIBUTE_LITERAL_STRING_VALUE)
		if !exist {
			panic("engine-error: setting value node does not have string attribute")
		}
		return val
	}

	// 3. Fallback for Identifiers (e.g., true, false, or raw unquoted values)
	return dslspec.TrimmedIdentifierNodeContent(valueNode)
}

func extractStringArray(arrayNode *Node) []string {
	var elements []string

	for _, strNode := range arrayNode.FindAllKind(dslspec.NodeStringLiteral) {
		val, exist := AttributeAs[string](strNode, dslspec.ATTRIBUTE_LITERAL_STRING_VALUE)
		if !exist {
			panic("engine-error: array element missing string attribute")
		}
		elements = append(elements, val)
	}

	return elements
}

func (c *compiler) compileRuleset(ctx *patternCompileCtx) *LexerRuleset {
	ruleset := lexarch.LexingRulesetCreate[rune, string, string](
		lexarch.TokenResolutionStepLongestThenPriority[string],
	)

	lexSection := ctx.rootNode.FindFirstKind(dslspec.NodeLexSection)
	lexRules := c.gatherRules(lexSection, ctx)

	for _, rule := range lexRules {
		ruleset.WithRulePriority(rule.tokenPattern, rule.tokenName, rule.tokenRole, rule.priority)
	}

	return ruleset
}

type lexRule struct {
	priority     int
	tokenName    string
	tokenRole    string
	tokenPattern pattern.RegulaAST[rune]
}

func (c *compiler) gatherRules(sectionNode *Node, ctx *patternCompileCtx) []lexRule {
	rules := sectionNode.FindAllKind(dslspec.NodeLexRule)
	out := make([]lexRule, 0, len(rules))

	for _, rule := range rules {
		if !nodeHasMetaByPred(rule, isEOFTrueMetaKVP) {
			out = append(out, c.constructLexRule(rule, ctx))
		}
	}

	return out
}

func (c *compiler) constructLexRule(ruleNode *Node, ctx *patternCompileCtx) lexRule {
	priority := 0
	if priorityNode, ok := nodeContains(ruleNode, dslspec.NodeLexRulePriority); ok {
		priority = extractIntContent(priorityNode)
	}

	tokenName := dslspec.NodeSingleTokenContent(ruleNode.FindFirstKind(dslspec.NodeLexRuleTokenName))
	tokenRole := dslspec.NodeSingleTokenContent(ruleNode.FindFirstKind(dslspec.NodeLexRuleRole))

	var tokenPattern pattern.RegulaAST[rune]
	patternNode := ruleNode.FindFirstKind(dslspec.NodeLexRulePattern)

	if patternNode == nil {
		patternNode = ruleNode.FindFirstKind(dslspec.NodePatternRef)
	}

	if patternNode == nil {
		panic("compiler error: lex rule must have a pattern or a variable reference")
	}

	switch patternNode.Kind() {
	case dslspec.NodePatternRef:
		target := semantics.PatternRefTargetName(patternNode)
		if ctx.env.Patterns[target] == nil {
			panic(fmt.Errorf("unresolved pattern reference: '%s'", target))
		}
		p, ok := ctx.patternTable[target]
		if !ok {
			panic(fmt.Errorf("unresolved pattern reference: '%s'", target))
		}
		tokenPattern = p

	case dslspec.NodeLexRulePattern:
		content := nodeFormattedContent(patternNode, dslspec.ATTRIBUTE_REGEX_LITERAL_VALUE)
		p, err := pattern.RegexToRegula(content, c.factory)
		if err != nil {
			panic(fmt.Errorf("engine error while converting regex to pattern: %w", err))
		}
		tokenPattern = p

	default:
		panic(fmt.Errorf("unsupported node kind for lex rule: %s", patternNode.Kind()))
	}

	return lexRule{
		priority:     priority,
		tokenName:    tokenName,
		tokenRole:    tokenRole,
		tokenPattern: tokenPattern,
	}
}

func (c *compiler) compilePatterns(ctx *patternCompileCtx) {
	out := make(map[string]pattern.RegulaAST[rune])
	temp := make(map[string]pattern.RegulaAST[rune])
	ctx.patternTable = out
	ctx.variables = temp

	patternSection := ctx.rootNode.FindFirstKind(dslspec.NodePatternSection)
	definitions := patternSection.FindAllKind(dslspec.NodePatternDefinition)

	for _, definition := range definitions {
		defName := lexemeRawContent(definition.FindFirstKind(dslspec.NodePatternDefName).Tokens()[0])

		var exprNode *Node
		for _, child := range definition.ChildrenUnsafe() {
			kind := child.Kind()
			if kind != dslspec.NodePatternDefName && kind != dslspec.NodeLocalVariable {
				exprNode = child
				break
			}
		}

		if exprNode == nil {
			panic(fmt.Sprintf("invariant violated: definition '%s' has no expression", defName))
		}

		pat := compilePatternExpression(ctx, exprNode)

		temp[defName] = pat
		if !ctx.env.LocalPatterns[defName] {
			out[defName] = pat
		}
	}

	ctx.variables = nil
}

func compilePatternExpression(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	c := ctx.c
	kind := node.Kind()

	switch kind {
	case dslspec.NodeCharLiteral:
		return c.charLiteralToPattern(node)
	case dslspec.NodeStringLiteral:
		return c.stringLiteralToPattern(node)
	case dslspec.NodePatternRegEx:
		return c.regexLiteralToPattern(node)
	case dslspec.NodePatternRef:
		return patternRefToPattern(ctx, node)
	case dslspec.NodePatternConcat:
		return concatToPattern(ctx, node)
	case dslspec.NodePatternAlternation:
		return alternationToPattern(ctx, node)
	case dslspec.NodePatternRange:
		return ctx.c.rangeToPattern(node)
	case dslspec.NodePatternStar:
		return starToPattern(ctx, node)
	case dslspec.NodePatternPlus:
		return plusToPattern(ctx, node)
	case dslspec.NodePatternOptional:
		return optionalToPattern(ctx, node)
	case dslspec.NodeRepetition:
		return repetitionToPattern(ctx, node)
	case dslspec.NodePatternGroup:
		return groupToPattern(ctx, node)
	case dslspec.NodePatternNegation:
		return ctx.c.negationToPattern(node)
	case dslspec.NodePatternAny:
		return c.factory.NegatedClass()
	case dslspec.NodePatternClass:
		return classToPattern(ctx, node)
	default:
		panic(fmt.Errorf("engine error: unsupported pattern kind: '%s'", kind))
	}
}

func concatToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	children := node.ChildrenUnsafe()

	concatPattern := compilePatternExpression(ctx, children[0])

	for i, child := range children {
		if i == 0 {
			continue
		}

		pat := compilePatternExpression(ctx, child)
		concatPattern = concatPattern.Then(pat)
	}

	return concatPattern
}

func alternationToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	children := node.ChildrenUnsafe()

	alternationPattern := compilePatternExpression(ctx, children[0])

	for i, child := range children {
		if i == 0 {
			continue
		}

		pat := compilePatternExpression(ctx, child)
		alternationPattern = alternationPattern.Or(pat)
	}

	return alternationPattern
}

func (c *compiler) rangeToPattern(
	node *Node,
) pattern.RegulaAST[rune] {
	charRange := c.extractPatternRange(node)
	return c.factory.Class(charRange)
}

func starToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	childPattern := compilePatternExpression(ctx, node.RequireSingleChild())
	return childPattern.Star()
}

func plusToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	childPattern := compilePatternExpression(ctx, node.RequireSingleChild())
	return childPattern.Plus()
}

func optionalToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	childPattern := compilePatternExpression(ctx, node.RequireSingleChild())
	return childPattern.Optional()
}

func repetitionToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	children := node.ChildrenUnsafe()
	childPattern := compilePatternExpression(ctx, children[0])
	boundsNode := children[1]

	minVal := 0
	maxVal := -1 // Unbounded

	// Query for Min
	if minNode := boundsNode.FindFirstKind(dslspec.NodeRepetitionMin); minNode != nil {
		minVal = extractIntContent(minNode)
	}

	// Query for Max
	if maxNode := boundsNode.FindFirstKind(dslspec.NodeRepetitionMax); maxNode != nil {
		maxVal = extractIntContent(maxNode)
	}

	return childPattern.Repeat(minVal, maxVal)
}

func groupToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	return compilePatternExpression(ctx, node.RequireSingleChild())
}

func (c *compiler) negationToPattern(
	node *Node,
) pattern.RegulaAST[rune] {
	var ranges []pattern.CharRange[rune]
	c.extractNegationRanges(node.RequireSingleChild(), &ranges)

	return c.factory.NegatedClass(ranges...)
}

func (c *compiler) extractNegationRanges(node *Node, out *[]pattern.CharRange[rune]) {
	kind := node.Kind()

	switch kind {
	case dslspec.NodeCharLiteral:
		*out = append(*out, c.extractCharRange(node))
	case dslspec.NodePatternRange:
		*out = append(*out, c.extractPatternRange(node))
	case dslspec.NodePatternClass:
		c.extractClassRanges(node, out)
	case dslspec.NodePatternGroup:
		c.extractGroupRanges(node, out)
	case dslspec.NodePatternAlternation:
		c.extractAlternationRanges(node, out)
	default:
		panic(fmt.Sprintf("semantic error: negation (!) applied to invalid node kind '%s'", kind))
	}
}

func (c *compiler) extractClassRanges(node *Node, out *[]pattern.CharRange[rune]) {
	for _, child := range node.ChildrenUnsafe() {
		item := child.Unwrap(dslspec.NodePatternClassItem)

		switch item.Kind() {
		case dslspec.NodeCharLiteral:
			lo := c.extractCharLiteralValue(item)
			*out = append(*out, c.factory.Range(lo, lo))

		case dslspec.NodePatternClassItem:
			children := item.ChildrenUnsafe()
			if len(children) == 2 && children[1].Kind() == dslspec.NodePatternRange {

				lo := c.extractCharLiteralValue(children[0])

				// The range tail itself only has 1 child (the RHS char)
				rhsNode := children[1].RequireSingleChild()
				hi := c.extractCharLiteralValue(rhsNode)

				*out = append(*out, c.factory.Range(lo, hi))

			} else {
				panic(fmt.Sprintf("engine error: unexpected class item structure with %d children", len(children)))
			}

		default:
			panic(fmt.Sprintf("engine error: unexpected node kind in class item: %v", item.Kind()))
		}
	}
}

func (c *compiler) extractCharLiteralValue(node *Node) rune {

	// Cleanly drill through any Pratt expression wrappers (Segments or Groups)
	coreNode := node.Unwrap(dslspec.NodePatternSegment, dslspec.NodePatternGroup)

	if coreNode.Kind() != dslspec.NodeCharLiteral {
		panic(fmt.Sprintf("semantic error: expected character literal, got %v", coreNode.Kind()))
	}

	content := nodeFormattedContent(coreNode, dslspec.ATTRIBUTE_CHAR_LITERAL_VALUE)
	runes := []rune(content)

	if len(runes) != 1 {
		panic("semantic error: character literal must resolve to exactly 1 rune")
	}

	return runes[0]
}

func (c *compiler) extractGroupRanges(node *Node, out *[]pattern.CharRange[rune]) {
	c.extractNegationRanges(node.RequireSingleChild(), out)
}

func (c *compiler) extractAlternationRanges(node *Node, out *[]pattern.CharRange[rune]) {
	for _, child := range node.ChildrenUnsafe() {
		c.extractNegationRanges(child, out)
	}
}

func (c *compiler) extractCharRange(node *Node) pattern.CharRange[rune] {
	content := nodeFormattedContent(node, dslspec.ATTRIBUTE_CHAR_LITERAL_VALUE)
	runes := []rune(content)

	if len(runes) != 1 {
		panic("semantic error: character literal in negation must resolve to exactly 1 rune")
	}

	return c.factory.Range(runes[0], runes[0])
}

func (c *compiler) extractPatternRange(node *Node) pattern.CharRange[rune] {
	children := node.ChildrenUnsafe()
	if len(children) != 2 {
		panic(fmt.Sprintf("engine error: infix range node must have exactly 2 children (LHS, RHS), got %d", len(children)))
	}

	lo := c.extractCharLiteralValue(children[0])
	hi := c.extractCharLiteralValue(children[1])

	return c.factory.Range(lo, hi)
}

func classToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	var ranges []pattern.CharRange[rune]
	ctx.c.extractClassRanges(node, &ranges)
	return ctx.c.factory.Class(ranges...)
}

func (c *compiler) charLiteralToPattern(node *Node) pattern.RegulaAST[rune] {
	content := nodeFormattedContent(node, dslspec.ATTRIBUTE_CHAR_LITERAL_VALUE)
	return c.factory.Literal([]rune(content)...)
}

func (c *compiler) stringLiteralToPattern(node *Node) pattern.RegulaAST[rune] {
	content := nodeFormattedContent(node, dslspec.ATTRIBUTE_LITERAL_STRING_VALUE)
	return c.factory.Literal([]rune(content)...)
}

func (c *compiler) regexLiteralToPattern(node *Node) pattern.RegulaAST[rune] {
	content := nodeFormattedContent(node, dslspec.ATTRIBUTE_REGEX_LITERAL_VALUE)
	compiled, err := pattern.RegexToRegula(content, c.factory)
	if err != nil {
		panic(fmt.Errorf("compiler error while converting regex to Regula: %w", err))
	}

	return compiled
}

func patternRefToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	targetName := semantics.PatternRefTargetName(node)
	if ctx.env.Patterns[targetName] == nil {
		panic(fmt.Errorf("error: pattern '%s' cannot be resolved", targetName))
	}
	targetPattern, ok := ctx.variables[targetName]
	if !ok {
		panic(fmt.Errorf("error: pattern '%s' cannot be resolved", targetName))
	}

	return targetPattern
}

func grammarSubLabel(ruleName string, role string, counts map[string]int) syntaxa.GrammarLabel {
	n := counts[role]
	counts[role]++
	if n == 0 {
		return syntaxa.GrammarLabel(ruleName + " " + role)
	}
	return syntaxa.GrammarLabel(fmt.Sprintf("%s %s %d", ruleName, role, n+1))
}

func parseCtxLabel(ctx *parseCompileCtx, role string) syntaxa.GrammarLabel {
	if ctx.rootLevel {
		return ctx.grammarID
	}
	return grammarSubLabel(ctx.ruleName, role, ctx.counts)
}

func collectSyncTokens(ruleNode *Node) []string {
	syncNode := ruleNode.FindFirstKind(dslspec.NodeRuleModifierSync)
	if syncNode == nil {
		return nil
	}
	var out []string
	for _, tNode := range syncNode.FindAllKind(dslspec.NodeSyncToken) {
		out = append(out, dslspec.NodeSingleTokenContent(tNode))
	}
	return out
}

func getParserSpecInfo(
	rootNode *Node,
	env *SemanticEnv,
	langName, langVersion string,
) (
	GrammarPackage,
	RuleRegistry,
	string,
	[]string,
	map[*syntaxa.Grammar[string, string]]*Node,
) {
	ruleBuilder := rule.RuleBuilderCreate[rune, string, string, string, string](
		func(token string) string {
			return token
		},
	)

	registry := ruleBuilder.GetRegistry()

	skipRoles := make([]string, 0)
	if parseSection := rootNode.FindFirstKind(dslspec.NodeParseSection); parseSection != nil {
		if ignoreSection := parseSection.FindFirstKind(dslspec.NodeParseIgnoreSection); ignoreSection != nil {
			for _, roleNode := range ignoreSection.FindAllKind(dslspec.NodeParseIgnoreRole) {
				skipRoles = append(skipRoles, dslspec.NodeSingleTokenContent(roleNode))
			}
		}
	}

	programRuleNode := env.Rules[semantics.ProgramRuleName]
	rootNodeKind := dslspec.NodeSingleTokenContent(programRuleNode.FindFirstKind(dslspec.NodeParseNodeName))

	var entryRule CompiledRule
	var entryOverride syntaxa.GrammarLabel

	sourceMap := make(map[*syntaxa.Grammar[string, string]]*Node)

	for ruleName, ruleNode := range env.Rules {
		ctx := &parseCompileCtx{
			builder:     ruleBuilder,
			env:         env,
			grammarID:   syntaxa.GrammarLabel(ruleName),
			nodeKind:    dslspec.NodeSingleTokenContent(ruleNode.FindFirstKind(dslspec.NodeParseNodeName)),
			ruleName:    ruleName,
			counts:      make(map[string]int),
			rootLevel:   true,
			transparent: ruleNode.FindFirstKind(dslspec.NodeRuleModifierTransparent) != nil,
			sourceMap:   sourceMap,
		}
		compiled, override := compileParseRuleDefinition(ctx, ruleNode)
		if ruleName == semantics.ProgramRuleName {
			entryRule = compiled
			if override != "" {
				entryOverride = override
			}
		}
	}
	if entryOverride != "" {
		entryRule = registry[entryOverride]
	}

	for ruleName, ruleNode := range env.Pratt {
		compilePrattExprDef(ruleBuilder, env, ruleName, ruleNode)
	}

	rootGrammar := entryRule.GetGrammar()

	entryRulePtr := new(CompiledRule)
	*entryRulePtr = entryRule
	grammarPackage := syntaxa.ProducePackage(
		rootGrammar,
		ruleBuilder.GetDefinedGrammars(),
		langName,
		langVersion,
		entryRulePtr,
	)

	return grammarPackage, registry, rootNodeKind, skipRoles, sourceMap
}

func buildParseRuleBodyMapForCompile(rules map[string]*Node) map[string]*Node {
	out := make(map[string]*Node, len(rules))
	for ruleName, ruleNode := range rules {
		bodyNode := ruleNode.FindFirstKind(dslspec.NodeParseRuleBody)
		if bodyNode == nil {
			continue
		}
		rootExpr := getParseRuleBodyRoot(bodyNode)
		bodyChildren := bodyNode.ChildrenUnsafe()
		if rootExpr == nil && len(bodyChildren) > 0 {
			rootExpr = bodyChildren[0]
		}
		if rootExpr != nil {
			out[ruleName] = rootExpr
		}
	}
	return out
}

func compileParseRuleDefinition(ctx *parseCompileCtx, ruleNode *Node) (CompiledRule, syntaxa.GrammarLabel) {
	bodyNode := ruleNode.FindFirstKind(dslspec.NodeParseRuleBody)
	if bodyNode == nil {
		panic("compiler error: parse rule missing body")
	}

	rootExpr := getParseRuleBodyRoot(bodyNode)
	if rootExpr == nil {
		bodyChildren := bodyNode.ChildrenUnsafe()
		if len(bodyChildren) == 0 {
			panic("compiler error: parse rule body has no children")
		}
		rootExpr = bodyChildren[0]
	}

	compiledExpr := compileParseExpression(ctx, rootExpr)
	if syncTokens := collectSyncTokens(ruleNode); len(syncTokens) > 0 {
		compiledExpr = ctx.builder.Rule.RecoverSync(compiledExpr, syncTokens...)
	}
	return ctx.builder.Rule.Define(compiledExpr), ""
}

func getParseRuleBodyRoot(body *Node) *Node {
	if body == nil {
		return nil
	}
	for _, ch := range body.ChildrenUnsafe() {
		if ch == nil {
			continue
		}
		k := ch.Kind()
		if k == dslspec.NodeParseAlternation || k == dslspec.NodeParseConcat || k == dslspec.NodeParseOptional ||
			k == dslspec.NodeParseStar || k == dslspec.NodeParsePlus || k == dslspec.NodeParseSegment || k == dslspec.NodeParseGroup {
			return ch
		}
	}
	return nil
}

func compileParseExpression(ctx *parseCompileCtx, node *Node) CompiledRule {
	kind := node.Kind()
	var compiledRule CompiledRule

	switch kind {
	case dslspec.NodeParseConcat:
		compiledRule = compileConcat(ctx, node)
	case dslspec.NodeParseAlternation:
		compiledRule = compileAlternation(ctx, node)
	case dslspec.NodeParseModifierPredict:
		compiledRule = compilePredict(ctx, node)
	case dslspec.NodeParseOptional:
		compiledRule = compileOptional(ctx, node)
	case dslspec.NodeParseExpressionReference, dslspec.NodeParseTokenReference:
		compiledRule = compileReference(ctx, node)
	case dslspec.NodeParsePlus:
		compiledRule = compilePlus(ctx, node)
	case dslspec.NodeParseStar:
		compiledRule = compileStar(ctx, node)
	case dslspec.NodeParseOpSuppress:
		compiledRule = compileVirtual(ctx, node)
	case dslspec.NodeParseOpNest:
		compiledRule = compileNest(ctx, node)
	case dslspec.NodeParseGroup:
		compiledRule = compileGroup(ctx, node)
	case dslspec.NodeParseSegment:
		compiledRule = compileParseSegment(ctx, node)
	case dslspec.NodeRepetition:
		compiledRule = compileParseRepetition(ctx, node)
	default:
		panic(fmt.Errorf("compiler error: unsupported parse expression kind: '%s'", kind))
	}

	if g := compiledRule.GetGrammar(); g != nil {
		ctx.sourceMap[g] = node
	}

	return compiledRule
}

func compileConcat(ctx *parseCompileCtx, node *Node) CompiledRule {
	flatNodes := node.FlattenByKind(dslspec.NodeParseConcat)

	rules := make([]CompiledRule, 0, len(flatNodes))
	subCtx := *ctx
	subCtx.rootLevel = false
	for _, child := range flatNodes {
		rules = append(rules, compileParseExpression(&subCtx, child))
	}

	if ctx.rootLevel {
		if ctx.ruleName == semantics.ProgramRuleName {
			return ctx.builder.Rule.Root(ctx.grammarID, ctx.nodeKind, false, rules...)
		}
		if ctx.transparent {
			return ctx.builder.Rule.TransparentSequence(ctx.grammarID, rules...)
		}
		return ctx.builder.Rule.Sequence(ctx.grammarID, ctx.nodeKind, rules...)
	}
	return ctx.builder.Rule.TransparentSequence(parseCtxLabel(ctx, "SEQUENCE"), rules...)
}

func compileAlternation(ctx *parseCompileCtx, node *Node) CompiledRule {
	flatNodes := node.FlattenByKind(dslspec.NodeParseAlternation)

	rules := make([]CompiledRule, 0, len(flatNodes))
	subCtx := *ctx
	subCtx.rootLevel = false
	for _, child := range flatNodes {
		compiled := compileParseExpression(&subCtx, child)
		rules = append(rules, compiled)
	}

	choiceRule := ctx.builder.Rule.Choice(parseCtxLabel(&subCtx, "CHOICE"), rules...)

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentSequence(ctx.grammarID, choiceRule)
		}
		return ctx.builder.Rule.Sequence(ctx.grammarID, ctx.nodeKind, choiceRule)
	}

	return choiceRule
}

func compilePredict(ctx *parseCompileCtx, node *Node) CompiledRule {
	children := node.ChildrenUnsafe()
	if len(children) != 2 {
		panic("compiler error: predict modifier must have a lookahead list and a target expression")
	}

	listNode := children[0]
	targetNode := children[1]

	innerRule := compileParseExpression(ctx, targetNode)

	lookaheads := listNode.FindAllKind(dslspec.NodePredictLookahead)
	type prediction struct {
		offset int
		token  string
	}
	var preds []prediction
	for _, la := range lookaheads {
		offset := extractIntContent(la.FindFirstKind(dslspec.NodePredictOffset))
		token := dslspec.NodeSingleTokenContent(la.FindFirstKind(dslspec.NodePredictToken))
		preds = append(preds, prediction{offset, token})
	}
	lookaheadSlice := make([]syntaxa.Lookahead[string], len(preds))
	for i, p := range preds {
		lookaheadSlice[i] = syntaxa.Lookahead[string]{Offset: p.offset, Expected: p.token}
	}
	return ctx.builder.Rule.PredictLookahead(innerRule, lookaheadSlice)
}

func compileOptional(ctx *parseCompileCtx, node *Node) CompiledRule {
	child := node.RequireSingleChild()
	subCtx := *ctx
	subCtx.rootLevel = false
	innerRule := compileParseExpression(&subCtx, child)

	optRule := ctx.builder.Rule.Optional(innerRule)

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentSequence(ctx.grammarID, optRule)
		}
		return ctx.builder.Rule.Sequence(ctx.grammarID, ctx.nodeKind, optRule)
	}

	return optRule
}

func compilePlus(ctx *parseCompileCtx, node *Node) CompiledRule {
	child := node.RequireSingleChild()
	subCtx := *ctx
	subCtx.rootLevel = false
	innerRule := compileParseExpression(&subCtx, child)

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentNOrMore(ctx.grammarID, 1, innerRule)
		}
		return ctx.builder.Rule.OneOrMore(ctx.grammarID, ctx.nodeKind, innerRule)
	}
	return ctx.builder.Rule.TransparentNOrMore(parseCtxLabel(ctx, "REPEAT_PLUS"), 1, innerRule)
}

func compileStar(ctx *parseCompileCtx, node *Node) CompiledRule {
	child := node.RequireSingleChild()
	subCtx := *ctx
	subCtx.rootLevel = false
	innerRule := compileParseExpression(&subCtx, child)

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentZeroOrMore(ctx.grammarID, innerRule)
		}
		return ctx.builder.Rule.ZeroOrMore(ctx.grammarID, ctx.nodeKind, innerRule)
	}
	return ctx.builder.Rule.TransparentZeroOrMore(parseCtxLabel(ctx, "REPEAT_STAR"), innerRule)
}

func compileEmit(ctx *parseCompileCtx, node *Node) CompiledRule {
	outputNodeKindNode := node.FindFirstKind(dslspec.NodeParseSymbolReference)
	if outputNodeKindNode == nil {
		outputNodeKindNode = node.FindFirstKind(dslspec.NodeParseNodeName)
	}
	if outputNodeKindNode == nil {
		panic("compiler error: emit node missing output kind")
	}
	outputNodeKind := dslspec.NodeSingleTokenContent(outputNodeKindNode)
	refNode := node.FindFirstKind(dslspec.NodeParseTokenReference)
	if refNode == nil {
		panic("compiler error: emit node missing reference")
	}
	targetToken := dslspec.NodeSingleTokenContent(refNode)
	return ctx.builder.Token.Expect(parseCtxLabel(ctx, "EMIT"), outputNodeKind, targetToken)
}

func compileEmitOneOfWithCustomName(ctx *parseCompileCtx, groupNode *Node, customNodeKind string) CompiledRule {
	innerExpr := groupNode.RequireSingleChild()

	flatAlts := innerExpr.FlattenByKind(dslspec.NodeParseAlternation)

	tokens := make([]string, 0, len(flatAlts))
	for _, alt := range flatAlts {
		refNode := alt.FindFirstKind(dslspec.NodeParseTokenReference)
		if refNode == nil {
			if alt.Kind() == dslspec.NodeParseTokenReference || alt.Kind() == dslspec.NodeParseExpressionReference {
				refNode = alt
			}
		}

		if refNode == nil {
			panic(fmt.Sprintf("compiler error: emit-one-of alternatives for '%s' must be token references", customNodeKind))
		}

		tokens = append(tokens, dslspec.IdentifierValue(refNode))
	}

	if len(tokens) == 0 {
		panic(fmt.Sprintf("compiler error: emit-one-of for '%s' has no valid tokens", customNodeKind))
	}

	return ctx.builder.Token.ExpectOneOf(parseCtxLabel(ctx, "EMIT_ONE_OF"), customNodeKind, tokens...)
}

func compileParseSegment(ctx *parseCompileCtx, node *Node) CompiledRule {
	nameNode := node.FindFirstKind(dslspec.NodeParseSymbolReference)
	if nameNode == nil {
		nameNode = node.FindFirstKind(dslspec.NodeParseNodeName)
	}
	if nameNode == nil {
		return compileParseExpression(ctx, node.RequireSingleChild())
	}

	targetName := dslspec.IdentifierValue(nameNode)

	groupRef := node.FindFirstKind(dslspec.NodeParseGroup)
	if groupRef != nil {
		return compileEmitOneOfWithCustomName(ctx, groupRef, targetName)
	}

	tokenRef := node.FindFirstKind(dslspec.NodeParseTokenReference)
	if tokenRef != nil {
		return ctx.builder.Token.Expect(parseCtxLabel(ctx, "EMIT"), targetName, dslspec.IdentifierValue(tokenRef))
	}

	return compileRuleReference(ctx, nameNode, targetName)
}

func compileParseRepetition(ctx *parseCompileCtx, node *Node) CompiledRule {
	children := node.ChildrenUnsafe()
	if len(children) < 2 {
		panic("compiler error: repetition node missing target or bounds")
	}

	subCtx := *ctx
	subCtx.rootLevel = false
	innerRule := compileParseExpression(&subCtx, children[0])

	minVal, maxVal := extractRepetitionBounds(children[1])

	return buildRepetitionRule(ctx, innerRule, minVal, maxVal)
}

func extractRepetitionBounds(boundsNode *Node) (int, int) {
	minVal := 0
	maxVal := -1 // Unbounded

	if minNode := boundsNode.FindFirstKind(dslspec.NodeRepetitionMin); minNode != nil {
		minVal = extractIntContent(minNode)
	}

	if maxNode := boundsNode.FindFirstKind(dslspec.NodeRepetitionMax); maxNode != nil {
		maxVal = extractIntContent(maxNode)
	}

	return minVal, maxVal
}

func buildRepetitionRule(ctx *parseCompileCtx, innerRule CompiledRule, min, max int) CompiledRule {
	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentRepeat(ctx.grammarID, min, max, innerRule)
		}
		return ctx.builder.Rule.Repeat(ctx.grammarID, ctx.nodeKind, min, max, innerRule)
	}

	label := parseCtxLabel(ctx, "REPETITION")
	return ctx.builder.Rule.TransparentRepeat(label, min, max, innerRule)
}

func compileVirtual(ctx *parseCompileCtx, node *Node) CompiledRule {
	refNode := node.FindFirstKind(dslspec.NodeParseTokenReference)
	if refNode == nil {
		panic("compiler error: virtual node missing reference")
	}
	targetToken := dslspec.NodeSingleTokenContent(refNode)
	return ctx.builder.Token.ExpectVirtual(parseCtxLabel(ctx, "VIRTUAL"), targetToken)
}

func compileNest(ctx *parseCompileCtx, node *Node) CompiledRule {
	openToken := dslspec.NodeSingleTokenContent(node.FindFirstKind(dslspec.NodeParseNestOpenToken))
	closeToken := dslspec.NodeSingleTokenContent(node.FindFirstKind(dslspec.NodeParseNestCloseToken))

	innerRule := extractNestInnerRule(ctx, node, closeToken)
	if syncTokens := collectSyncTokens(node); len(syncTokens) > 0 {
		innerRule = ctx.builder.Rule.RecoverSync(innerRule, syncTokens...)
	}
	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentNest(ctx.grammarID, openToken, closeToken, innerRule)
		}
		return ctx.builder.Rule.Nest(ctx.grammarID, ctx.nodeKind, openToken, closeToken, innerRule)
	}
	return ctx.builder.Rule.TransparentNest(parseCtxLabel(ctx, "NEST"), openToken, closeToken, innerRule)
}

func extractNestInnerRule(ctx *parseCompileCtx, node *Node, closeToken string) CompiledRule {
	subCtx := *ctx
	subCtx.rootLevel = false

	if bodyNode := node.FindFirstKind(dslspec.NodeParseNestBody); bodyNode != nil {
		return compileParseExpression(&subCtx, bodyNode.RequireSingleChild())
	}

	if refNode := node.FindFirstKind(dslspec.NodeParseExpressionReference); refNode != nil {
		return compileParseExpression(&subCtx, refNode)
	}

	panic("compiler error: nest must contain a body or a reference")
}

func compileReference(ctx *parseCompileCtx, node *Node) CompiledRule {
	targetName := dslspec.NodeSingleTokenContent(node)
	_, isToken := ctx.env.Tokens[targetName]
	_, isRule := ctx.env.Rules[targetName]
	_, isPratt := ctx.env.Pratt[targetName]

	if isToken {
		return compileTokenMatch(ctx, node)
	}
	if isRule || isPratt {
		return compileRuleReference(ctx, node, targetName)
	}
	panic(fmt.Errorf("compiler error: unresolved reference '%s' (not a token, rule, or pratt expression)", targetName))
}

func compileRuleReference(ctx *parseCompileCtx, node *Node, targetRuleName string) CompiledRule {
	targetGrammarID := syntaxa.GrammarLabel(targetRuleName)
	refRule := ctx.builder.Rule.Reference(parseCtxLabel(ctx, "REF"), targetGrammarID)

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentSequence(ctx.grammarID, refRule)
		}
		return ctx.builder.Rule.Sequence(ctx.grammarID, ctx.nodeKind, refRule)
	}

	return refRule
}

// compileGroup compiles a parse group. Uses Unwrap so nested groups (e.g. ( ( expr ) )) yield the innermost expression.
func compileGroup(ctx *parseCompileCtx, node *Node) CompiledRule {
	inner := node.Unwrap(dslspec.NodeParseGroup)
	return compileParseExpression(ctx, inner)
}

func compileTokenMatch(ctx *parseCompileCtx, node *Node) CompiledRule {
	targetToken := dslspec.NodeSingleTokenContent(node)
	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Token.ExpectVirtual(ctx.grammarID, targetToken)
		}
		return ctx.builder.Token.Expect(ctx.grammarID, ctx.nodeKind, targetToken)
	}
	return ctx.builder.Token.Expect(parseCtxLabel(ctx, "TOKEN"), "", targetToken)
}

func compilePrattExprDef(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	ruleName string,
	ruleNode *Node,
) CompiledRule {
	grammarID := syntaxa.GrammarLabel(ruleName)

	bodyNode := ruleNode.FindFirstKind(dslspec.NodePrattExprBody)
	if bodyNode == nil {
		panic("compiler error: pratt rule missing body")
	}

	config := buildPrattConfig(builder, env, grammarID, ruleNode, bodyNode)
	compiledExpr := builder.Pratt.Expression(grammarID, config)

	return builder.Rule.Define(compiledExpr)
}

func buildPrattConfig(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	ruleNode *Node,
	bodyNode *Node,
) rule.PrattConfig[rune, string, string, string, string] {
	config := rule.PrattConfig[rune, string, string, string, string]{}

	for _, actualCat := range bodyNode.ChildrenUnsafe() {
		switch actualCat.Kind() {
		case dslspec.NodePrattPrimary:
			config.Primary = compilePrattPrimary(builder, grammarID, actualCat)
		case dslspec.NodePrattPrefix:
			config.PrefixOps, config.PrefixRuleOps = compilePrattPrefixOps(builder, env, grammarID, actualCat)
		case dslspec.NodePrattInfix:
			config.InfixOps, config.InfixRuleOps = compilePrattInfixOps(builder, env, grammarID, actualCat)
		case dslspec.NodePrattPostfix:
			config.PostfixOps, config.PostfixRuleOps = compilePrattPostfixOps(builder, env, grammarID, actualCat)
		case dslspec.NodePrattImplicit:
			config.ImplicitInfix = compilePrattImplicitOp(actualCat)
		default:
			panic(fmt.Errorf("compiler error: unknown pratt category: %s", actualCat.Kind()))
		}
	}
	return config
}

func compilePrattPrimary(
	builder *CompilerRuleBuilder,
	grammarID syntaxa.GrammarLabel,
	node *Node,
) CompiledRule {
	body := node.FindFirstKind(dslspec.NodePrattPrimaryBody)
	refNode := body.FindFirstKind(dslspec.NodePrattPrimaryRef)
	if refNode == nil {
		panic("compiler error: pratt primary body missing ref")
	}
	if refChild := refNode.FindFirstKind(dslspec.NodeParseExpressionReference); refChild != nil {
		refNode = refChild
	}
	targetRuleName := dslspec.RefName(refNode)
	targetGrammarID := syntaxa.GrammarLabel(targetRuleName)

	return builder.Rule.Reference(grammarID, targetGrammarID)
}

func compilePrattPrefixOps(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	node *Node,
) ([]rule.PrattPrefixOp[string, string], []rule.PrattPrefixRuleOp[rune, string, string, string, string]) {
	bodyNode := node.FindFirstKind(dslspec.NodePrattPrefixBody)

	tokOps := make([]rule.PrattPrefixOp[string, string], 0)
	ruleOps := make([]rule.PrattPrefixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.ChildrenUnsafe() {
		target, rightBP := extractPrefixData(def, env)

		if target.IsRule {
			ruleOps = append(ruleOps, rule.PrattPrefixRuleOp[rune, string, string, string, string]{
				RightBP:  rightBP,
				NodeKind: target.NodeKind,
				Rule:     builder.Rule.Reference(grammarID, syntaxa.GrammarLabel(target.Ref)),
			})
		} else {
			tokOps = append(tokOps, rule.PrattPrefixOp[string, string]{
				Token:             target.Ref,
				RightBP:           rightBP,
				NodeKind:          target.NodeKind,
				TokenGrammarLabel: syntaxa.GrammarLabel(target.Ref),
			})
		}
	}

	return tokOps, ruleOps
}

func compilePrattInfixOps(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	node *Node,
) ([]rule.PrattInfixOp[string, string], []rule.PrattInfixRuleOp[rune, string, string, string, string]) {
	bodyNode := node.FindFirstKind(dslspec.NodePrattInfixBody)

	tokOps := make([]rule.PrattInfixOp[string, string], 0)
	ruleOps := make([]rule.PrattInfixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.ChildrenUnsafe() {
		target, leftBP, rightBP := extractInfixData(def, env)

		if target.IsRule {
			ruleOps = append(ruleOps, rule.PrattInfixRuleOp[rune, string, string, string, string]{
				LeftBP:   leftBP,
				RightBP:  rightBP,
				NodeKind: target.NodeKind,
				Rule:     builder.Rule.Reference(grammarID, syntaxa.GrammarLabel(target.Ref)),
			})
		} else {
			tokOps = append(tokOps, rule.PrattInfixOp[string, string]{
				Token:             target.Ref,
				LeftBP:            leftBP,
				RightBP:           rightBP,
				NodeKind:          target.NodeKind,
				TokenGrammarLabel: syntaxa.GrammarLabel(target.Ref),
			})
		}
	}

	return tokOps, ruleOps
}

func compilePrattPostfixOps(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	node *Node,
) ([]rule.PrattPostfixOp[string, string], []rule.PrattPostfixRuleOp[rune, string, string, string, string]) {
	bodyNode := node.FindFirstKind(dslspec.NodePrattPostfixBody)

	tokOps := make([]rule.PrattPostfixOp[string, string], 0)
	ruleOps := make([]rule.PrattPostfixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.ChildrenUnsafe() {
		target, leftBP := extractPostfixData(def, env)

		if target.IsRule {
			ruleOps = append(ruleOps, rule.PrattPostfixRuleOp[rune, string, string, string, string]{
				LeftBP:   leftBP,
				NodeKind: target.NodeKind,
				Rule:     builder.Rule.Reference(grammarID, syntaxa.GrammarLabel(target.Ref)),
			})
		} else {
			tokOps = append(tokOps, rule.PrattPostfixOp[string, string]{
				Token:             target.Ref,
				LeftBP:            leftBP,
				NodeKind:          target.NodeKind,
				TokenGrammarLabel: syntaxa.GrammarLabel(target.Ref),
			})
		}
	}

	return tokOps, ruleOps
}

func compilePrattImplicitOp(node *Node) *rule.PrattImplicitInfix[string, string] {
	body := node.FindFirstKind(dslspec.NodePrattImplicitBody)
	def := body.FindFirstKind(dslspec.NodePrattImplicitDef)

	nodeKind := dslspec.NodeSingleTokenContent(def.FindFirstKind(dslspec.NodeParseNodeName))
	leftBP := extractIntContent(def.FindFirstKind(dslspec.NodePrattLeftPrecedenceValue))
	rightBP := extractIntContent(def.FindFirstKind(dslspec.NodePrattRightPrecedenceValue))

	return &rule.PrattImplicitInfix[string, string]{
		LeftBP:   leftBP,
		RightBP:  rightBP,
		NodeKind: nodeKind,
	}
}

/*
OperatorTarget is a resolved Pratt operator binding: whether the target is a parse rule or a lexer token,
the symbol name, and the parse node kind for the operator's AST node.
*/
type OperatorTarget struct {
	IsRule   bool
	Ref      string
	NodeKind string
}

func extractOperatorTarget(defNode *Node, env *SemanticEnv) OperatorTarget {
	nodeKind := dslspec.NodeSingleTokenContent(defNode.FindFirstKind(dslspec.NodeParseNodeName))
	refNode := defNode.FindFirstKind(dslspec.NodeParseSymbolReference)
	if refNode == nil {
		panic("compiler error: pratt operator target missing reference")
	}
	refName := dslspec.NodeSingleTokenContent(refNode)

	if env.Tokens[refName] != nil {
		return OperatorTarget{
			IsRule:   false,
			Ref:      refName,
			NodeKind: nodeKind,
		}
	}
	if env.Rules[refName] != nil || env.Pratt[refName] != nil {
		return OperatorTarget{
			IsRule:   true,
			Ref:      refName,
			NodeKind: nodeKind,
		}
	}
	panic(fmt.Errorf("compiler error: unresolved pratt operator target '%s'", refName))
}

func extractPrefixData(defNode *Node, env *SemanticEnv) (OperatorTarget, int) {
	target := extractOperatorTarget(defNode, env)
	rightBP := extractIntContent(defNode.FindFirstKind(dslspec.NodePrattRightPrecedenceValue))
	return target, rightBP
}

func extractPostfixData(defNode *Node, env *SemanticEnv) (OperatorTarget, int) {
	target := extractOperatorTarget(defNode, env)
	leftBP := extractIntContent(defNode.FindFirstKind(dslspec.NodePrattLeftPrecedenceValue))
	return target, leftBP
}

func extractInfixData(defNode *Node, env *SemanticEnv) (OperatorTarget, int, int) {
	target := extractOperatorTarget(defNode, env)
	leftBP := extractIntContent(defNode.FindFirstKind(dslspec.NodePrattLeftPrecedenceValue))
	rightBP := extractIntContent(defNode.FindFirstKind(dslspec.NodePrattRightPrecedenceValue))
	return target, leftBP, rightBP
}

func extractIntContent(node *Node) int {
	rawContent := lexemeRawContent(node.Tokens()[0])
	num, err := strconv.Atoi(rawContent)

	if err != nil {
		panic(fmt.Errorf("engine error: number is not an integer: '%s': %w", rawContent, err))
	}

	return num
}

func nodeContains(node *Node, target LangSpecParserNodeKind) (*Node, bool) {
	got := node.FindFirstKind(target)
	return got, got != nil
}

func nodeHasMetaByPred(node *Node, pred func(metaKVPNode *Node) bool) bool {
	meta, ok := nodeContains(node, dslspec.NodeMetaSection)
	if !ok {
		return false
	}

	kvps := meta.FindAllKind(dslspec.NodeMetaKeyValuePair)
	for _, kvp := range kvps {
		if pred(kvp) {
			return true
		}
	}

	return false
}

func isEOFTrueMetaKVP(kvpNode *Node) bool {
	key := kvpNode.FindFirstKind(dslspec.NodeMetaKey)
	if key == nil || !lexemeRawContentEqualTo(key.Tokens()[0], "EOF") {
		return false
	}

	valueNode := kvpNode.FindFirstKind(dslspec.NodeMetaValue)
	if valueNode == nil {
		return false
	}

	return lexemeKindEqualTo(valueNode.Tokens()[0], dslspec.TokKWTrue)
}

func checkRuleForEOFMeta(rule *Node) string {
	if nodeHasMetaByPred(rule, isEOFTrueMetaKVP) {
		identifier := rule.FindFirstKind(dslspec.NodeLexRuleTokenName).Tokens()[0]
		return lexemeRawContent(identifier)
	}
	return ""
}

func checkForEOFLexeme(lexerSection *Node) string {
	for _, rule := range lexerSection.FindAllKind(dslspec.NodeLexRule) {
		if token := checkRuleForEOFMeta(rule); token != "" {
			return token
		}
	}
	return ""
}

func nodeFormattedContent(node *Node, formatAttribute string) string {
	formatted, ok := AttributeAs[string](node, formatAttribute)
	if !ok {
		panic(fmt.Errorf("engine error encountered: %v not stored for node %v", formatAttribute, node))
	}

	return formatted
}

func lexemeRawContent(
	lexeme lexarch.Lexeme[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
) string {
	return string(lexeme.Raw)
}

func lexemeKindEqualTo(
	lexeme lexarch.Lexeme[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	target LangSpecLexerTokenType,
) bool {
	return lexeme.Token == target
}

func lexemeRawContentEqualTo(
	lexeme lexarch.Lexeme[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	target string,
) bool {
	return strings.EqualFold(string(lexeme.Raw), target)
}
