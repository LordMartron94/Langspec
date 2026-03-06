package dsl

import (
	"autarch/pattern"
	"fmt"
	"langspec"
	"lexarch"
	"strconv"
	"strings"
	"syntaxa"
	"syntaxa/rule"
)

type LexerSpec = langspec.LexerSpec[rune, string, string, string]
type ParserSpec = langspec.ParserSpec[rune, string, string, string, string]

type GrammarPackage = syntaxa.GrammarPackage[rune, string, string, string, string]
type RuleRegistry = syntaxa.RuleRegistry[rune, string, string, string, string]

type LexerRuleset = lexarch.LexingRuleset[rune, string, string]

type CompiledRule = syntaxa.ParserRule[rune, string, string, string, string]
type CompilerRuleBuilder = rule.RuleBuilder[rune, string, string, string, string]

type CompiledLangSpec struct {
	dslName    string
	dslVersion string

	lexerSpec  *LexerSpec
	parserSpec *ParserSpec

	grammarPackage GrammarPackage

	eofToken string
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
}

func compileTree(comp *LangSpecCompiler, rootNode *Node) *CompiledLangSpec {
	dslName, dslVersion := getInfoFromHeader(rootNode.FindFirstKind(NodeHeader))

	env := BuildSemanticEnv(rootNode, nil)

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

	grammarPackage, ruleRegistry, rootNodeKind, skipRoles := getParserSpecInfo(rootNode, env, dslName, dslVersion)

	grammarPkg := new(syntaxa.GrammarPackage[rune, string, string, string, string])
	*grammarPkg = grammarPackage

	parserSpec := langspec.ParserSpecCreate(
		grammarPkg,
		ruleRegistry,
		rootNodeKind,
		"ERROR_NODE",
		false, // TODO allow configuration of this flag inside LSpec
	)
	parserSpec.WithSkipRoles(skipRoles...)

	return &CompiledLangSpec{
		dslName:        dslName,
		dslVersion:     dslVersion,
		lexerSpec:      lexerSpec,
		parserSpec:     parserSpec,
		eofToken:       eofToken,
		grammarPackage: *grammarPkg,
	}
}

// ------------------------------- HEADER -------------------------------

func getInfoFromHeader(headerNode *Node) (string, string) {
	dslName := nodeFormattedContent(headerNode.FindFirstKind(NodeDSLName), ATTRIBUTE_LITERAL_STRING_VALUE)
	dslVersion := lexemeRawContent(headerNode.FindFirstKind(NodeVersion).Tokens()[0])

	return dslName, dslVersion
}

func getEOFToken(rootNode *Node) string {
	lexerSection := rootNode.FindFirstKind(NodeLexSection)

	eofToken := checkForEOFLexeme(lexerSection)
	if eofToken == "" {
		eofToken = "EOF_INJECTED"
	}

	return eofToken
}

// ------------------------------- LEX -------------------------------

func (c *compiler) compileRuleset(ctx *patternCompileCtx) *LexerRuleset {
	ruleset := lexarch.LexingRulesetCreate[rune, string, string](
		lexarch.TokenResolutionStepLongestThenPriority[string],
	)

	lexSection := ctx.rootNode.FindFirstKind(NodeLexSection)
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
	rules := sectionNode.FindAllKind(NodeLexRule)
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
	if priorityNode, ok := nodeContains(ruleNode, NodeLexRulePriority); ok {
		priority = extractIntContent(priorityNode)
	}

	tokenName := nodeSingleTokenContent(ruleNode.FindFirstKind(NodeLexRuleTokenName))
	tokenRole := nodeSingleTokenContent(ruleNode.FindFirstKind(NodeLexRuleRole))

	var tokenPattern pattern.RegulaAST[rune]
	patternNode := ruleNode.FindFirstKind(NodeLexRulePattern)

	if patternNode == nil {
		patternNode = ruleNode.FindFirstKind(NodeVarRef)
	}

	if patternNode == nil {
		panic("compiler error: lex rule must have a pattern or a variable reference")
	}

	switch patternNode.Kind() {
	case NodeVarRef:
		target := VarRefTargetName(patternNode)
		if ctx.env.Patterns[target] == nil {
			panic(fmt.Errorf("unresolved pattern reference: '%s'", target))
		}
		p, ok := ctx.patternTable[target]
		if !ok {
			panic(fmt.Errorf("unresolved pattern reference: '%s'", target))
		}
		tokenPattern = p

	case NodeLexRulePattern:
		content := nodeFormattedContent(patternNode, ATTRIBUTE_REGEX_LITERAL_VALUE)
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

	patternSection := ctx.rootNode.FindFirstKind(NodePatternSection)
	definitions := patternSection.FindAllKind(NodePatternDefinition)

	for _, definition := range definitions {
		defName := lexemeRawContent(definition.FindFirstKind(NodePatternDefName).Tokens()[0])

		var exprNode *Node
		for _, child := range definition.Children() {
			kind := child.Kind()
			if kind != NodePatternDefName && kind != NodeLocalVariable {
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
	case NodeCharLiteral:
		return c.charLiteralToPattern(node)
	case NodeVarRef:
		return varRefToPattern(ctx, node)
	case NodePatternConcat:
		return concatToPattern(ctx, node)
	case NodePatternAlternation:
		return alternationToPattern(ctx, node)
	case NodePatternRange:
		return ctx.c.rangeToPattern(node)
	case NodePatternStar:
		return starToPattern(ctx, node)
	case NodePatternPlus:
		return plusToPattern(ctx, node)
	case NodePatternOptional:
		return optionalToPattern(ctx, node)
	case NodeRepetition:
		return repetitionToPattern(ctx, node)
	case NodePatternGroup:
		return groupToPattern(ctx, node)
	case NodePatternNegation:
		return ctx.c.negationToPattern(node)
	case NodePatternAny:
		return c.factory.NegatedClass()
	default:
		panic(fmt.Errorf("engine error: unsupported pattern kind: '%s'", kind))
	}
}

// --- PATTERNS

func concatToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	children := node.Children()

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
	children := node.Children()

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
	children := node.Children()

	if len(children) != 1 {
		panic("error: star pattern must have exactly 1 child")
	}

	childPattern := compilePatternExpression(ctx, children[0])
	return childPattern.Star()
}

func plusToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 1 {
		panic("error: plus pattern must have exactly 1 child")
	}

	childPattern := compilePatternExpression(ctx, children[0])
	return childPattern.Plus()
}

func optionalToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 1 {
		panic("error: optional pattern must have exactly 1 child")
	}

	childPattern := compilePatternExpression(ctx, children[0])
	return childPattern.Optional()
}

func repetitionToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 2 {
		panic("compiler error: repetition node must have exactly 2 children (pattern and settings)")
	}

	childPattern := compilePatternExpression(ctx, children[0])
	repetitionSettings := children[1]

	minVal := 0
	maxVal := -1

	if minNode, ok := nodeContains(repetitionSettings, NodeRepetitionMin); ok {
		minVal = extractIntContent(minNode)
	}

	if maxNode, ok := nodeContains(repetitionSettings, NodeRepetitionMax); ok {
		maxVal = extractIntContent(maxNode)
	}

	return childPattern.Repeat(minVal, maxVal)
}

func groupToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	children := node.Children()
	if len(children) != 1 {
		panic("engine error: group node must have exactly 1 child")
	}

	return compilePatternExpression(ctx, children[0])
}

func (c *compiler) negationToPattern(
	node *Node,
) pattern.RegulaAST[rune] {
	children := node.Children()
	if len(children) != 1 {
		panic("engine error: negation node must have exactly 1 child")
	}

	var ranges []pattern.CharRange[rune]
	c.extractNegationRanges(children[0], &ranges)

	return c.factory.NegatedClass(ranges...)
}

func (c *compiler) extractNegationRanges(node *Node, out *[]pattern.CharRange[rune]) {
	kind := node.Kind()

	switch kind {
	case NodeCharLiteral:
		*out = append(*out, c.extractCharRange(node))
	case NodePatternRange:
		*out = append(*out, c.extractPatternRange(node))
	case NodePatternGroup:
		c.extractGroupRanges(node, out)
	case NodePatternAlternation:
		c.extractAlternationRanges(node, out)
	default:
		panic(fmt.Sprintf("semantic error: negation (!) applied to invalid node kind '%s'", kind))
	}
}

func (c *compiler) extractGroupRanges(node *Node, out *[]pattern.CharRange[rune]) {
	children := node.Children()
	if len(children) != 1 {
		panic("engine error: group node in negation must have exactly 1 child")
	}
	c.extractNegationRanges(children[0], out)
}

func (c *compiler) extractAlternationRanges(node *Node, out *[]pattern.CharRange[rune]) {
	for _, child := range node.Children() {
		c.extractNegationRanges(child, out)
	}
}

func (c *compiler) extractCharRange(node *Node) pattern.CharRange[rune] {
	content := nodeFormattedContent(node, ATTRIBUTE_CHAR_LITERAL_VALUE)
	runes := []rune(content)

	if len(runes) != 1 {
		panic("semantic error: character literal in negation must resolve to exactly 1 rune")
	}

	return c.factory.Range(runes[0], runes[0])
}

func (c *compiler) extractPatternRange(node *Node) pattern.CharRange[rune] {
	children := node.Children()
	if len(children) != 2 {
		panic("engine error: range node must have exactly 2 children")
	}

	loContent := nodeFormattedContent(children[0], ATTRIBUTE_CHAR_LITERAL_VALUE)
	hiContent := nodeFormattedContent(children[1], ATTRIBUTE_CHAR_LITERAL_VALUE)

	loRunes := []rune(loContent)
	hiRunes := []rune(hiContent)

	if len(loRunes) != 1 || len(hiRunes) != 1 {
		panic("semantic error: bounds in pattern range must resolve to exactly 1 rune each")
	}

	return c.factory.Range(loRunes[0], hiRunes[0])
}

// --- ATOMS

func (c *compiler) charLiteralToPattern(node *Node) pattern.RegulaAST[rune] {
	content := nodeFormattedContent(node, ATTRIBUTE_CHAR_LITERAL_VALUE)
	return c.factory.Literal([]rune(content)...)
}

func varRefToPattern(ctx *patternCompileCtx, node *Node) pattern.RegulaAST[rune] {
	targetName := VarRefTargetName(node)
	if ctx.env.Patterns[targetName] == nil {
		panic(fmt.Errorf("error: pattern '%s' cannot be resolved", targetName))
	}
	targetPattern, ok := ctx.variables[targetName]
	if !ok {
		panic(fmt.Errorf("error: pattern '%s' cannot be resolved", targetName))
	}

	return targetPattern
}

// ------------------------------- PRATT & PARSE -------------------------------

func grammarSubLabel(ruleName string, role string, counts map[string]int) syntaxa.GrammarLabel {
	n := counts[role]
	counts[role]++
	if n == 0 {
		return syntaxa.GrammarLabel(ruleName + " " + role)
	}
	return syntaxa.GrammarLabel(fmt.Sprintf("%s %s %d", ruleName, role, n+1))
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
) {
	ruleBuilder := rule.RuleBuilderCreate[rune, string, string, string, string](
		func(token string) string {
			return token
		},
	)

	registry := ruleBuilder.GetRegistry()

	skipRoles := make([]string, 0)
	if parseSection := rootNode.FindFirstKind(NodeParseSection); parseSection != nil {
		if ignoreSection := parseSection.FindFirstKind(NodeParseIgnoreSection); ignoreSection != nil {
			for _, roleNode := range ignoreSection.FindAllKind(NodeParseIgnoreRole) {
				skipRoles = append(skipRoles, nodeSingleTokenContent(roleNode))
			}
		}
	}

	programRuleNode := env.Rules[programRuleName]
	rootNodeKind := nodeSingleTokenContent(programRuleNode.FindFirstKind(NodeParseNodeName))

	var entryRule CompiledRule
	var entryOverride syntaxa.GrammarLabel

	for ruleName, ruleNode := range env.Rules {
		ctx := &parseCompileCtx{
			builder:     ruleBuilder,
			env:         env,
			grammarID:   syntaxa.GrammarLabel(ruleName),
			nodeKind:    nodeSingleTokenContent(ruleNode.FindFirstKind(NodeParseNodeName)),
			ruleName:    ruleName,
			counts:      make(map[string]int),
			rootLevel:   true,
			transparent: ruleNode.FindFirstKind(NodeRuleModifierTransparent) != nil,
		}
		compiled, override := compileParseRuleDefinition(ctx, ruleNode)
		if ruleName == programRuleName {
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

	return grammarPackage, registry, rootNodeKind, skipRoles
}

func compileParseRuleDefinition(ctx *parseCompileCtx, ruleNode *Node) (CompiledRule, syntaxa.GrammarLabel) {
	bodyNode := ruleNode.FindFirstKind(NodeParseRuleBody)
	if bodyNode == nil {
		panic("compiler error: parse rule missing body")
	}

	rootExpr := getParseRuleBodyRoot(bodyNode)
	if rootExpr == nil {
		if len(bodyNode.Children()) == 0 {
			panic("compiler error: parse rule body has no children")
		}
		rootExpr = bodyNode.Children()[0]
	}

	compiledExpr := compileParseExpression(ctx, rootExpr)

	// Inject Sync modifier wrapping logic
	syncNode := ruleNode.FindFirstKind(NodeRuleModifierSync)
	if syncNode != nil {
		var syncTokens []string
		for _, tNode := range syncNode.FindAllKind(NodeSyncToken) {
			syncTokens = append(syncTokens, nodeSingleTokenContent(tNode))
		}
		if len(syncTokens) > 0 {
			compiledExpr = ctx.builder.Rule.RecoverSync(compiledExpr, syncTokens...)
		}
	}

	return ctx.builder.Rule.Define(compiledExpr), ""
}

func compileParseExpression(ctx *parseCompileCtx, node *Node) CompiledRule {
	kind := node.Kind()

	switch kind {
	case NodeParseConcat:
		return compileConcat(ctx, node)
	case NodeParseAlternation:
		return compileAlternation(ctx, node)
	case NodeParseModifierPredict:
		return compilePredict(ctx, node)
	case NodeParseOptional:
		return compileOptional(ctx, node)
	case NodeParseOpRef:
		return compileReference(ctx, node)
	case NodeIdentifier:
		return compileTokenMatch(ctx, node)
	case NodeParsePlus:
		return compilePlus(ctx, node)
	case NodeParseStar:
		return compileStar(ctx, node)
	case NodeParseOpEmit:
		return compileEmit(ctx, node)
	case NodeParseOpSuppress:
		return compileVirtual(ctx, node)
	case NodeParseOpNest:
		return compileNest(ctx, node)
	case NodeParseGroup:
		return compileGroup(ctx, node)
	default:
		panic(fmt.Errorf("compiler error: unsupported parse expression kind: '%s'", kind))
	}
}

// ------------------------------- EXPRESSION HANDLERS -------------------------------

func flattenNodesByKind(node *Node, kind LangSpecParserNodeKind) []*Node {
	if node.Kind() != kind {
		return []*Node{node}
	}

	var flat []*Node
	for _, child := range node.Children() {
		flat = append(flat, flattenNodesByKind(child, kind)...)
	}
	return flat
}

func compileConcat(ctx *parseCompileCtx, node *Node) CompiledRule {
	flatNodes := flattenNodesByKind(node, NodeParseConcat)

	rules := make([]CompiledRule, 0, len(flatNodes))
	subCtx := *ctx
	subCtx.rootLevel = false
	for _, child := range flatNodes {
		rules = append(rules, compileParseExpression(&subCtx, child))
	}

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentSequence(ctx.grammarID, rules...)
		}
		return ctx.builder.Rule.Sequence(ctx.grammarID, ctx.nodeKind, rules...)
	}

	label := grammarSubLabel(ctx.ruleName, "SEQUENCE", ctx.counts)
	return ctx.builder.Rule.TransparentSequence(label, rules...)
}

func compileAlternation(ctx *parseCompileCtx, node *Node) CompiledRule {
	flatNodes := flattenNodesByKind(node, NodeParseAlternation)

	rules := make([]CompiledRule, 0, len(flatNodes))
	subCtx := *ctx
	subCtx.rootLevel = false
	for _, child := range flatNodes {
		compiled := compileParseExpression(&subCtx, child)
		rules = append(rules, compiled)
	}

	var label syntaxa.GrammarLabel
	if ctx.rootLevel {
		label = ctx.grammarID
	} else {
		label = grammarSubLabel(ctx.ruleName, "CHOICE", ctx.counts)
	}
	return ctx.builder.Rule.Choice(label, rules...)
}

func compilePredict(ctx *parseCompileCtx, node *Node) CompiledRule {
	children := node.Children()
	if len(children) != 2 {
		panic("compiler error: predict modifier must have a lookahead list and a target expression")
	}

	listNode := children[0]
	targetNode := children[1]

	innerRule := compileParseExpression(ctx, targetNode)

	lookaheads := listNode.FindAllKind(NodePredictLookahead)
	type prediction struct {
		offset int
		token  string
	}
	var preds []prediction
	for _, la := range lookaheads {
		offset := extractIntContent(la.FindFirstKind(NodePredictOffset))
		token := nodeSingleTokenContent(la.FindFirstKind(NodePredictToken))
		preds = append(preds, prediction{offset, token})
	}

	return ctx.builder.Rule.Predict(innerRule, func(selectCtx *syntaxa.SelectRuleContext[rune, string, string]) bool {
		for _, p := range preds {
			if selectCtx.Peek(p.offset).Token != p.token {
				return false
			}
		}
		return true
	})
}

func compileOptional(ctx *parseCompileCtx, node *Node) CompiledRule {
	child := extractSingleChild(node)
	subCtx := *ctx
	subCtx.rootLevel = false
	innerRule := compileParseExpression(&subCtx, child)

	return ctx.builder.Rule.Optional(innerRule)
}

func compilePlus(ctx *parseCompileCtx, node *Node) CompiledRule {
	child := extractSingleChild(node)
	subCtx := *ctx
	subCtx.rootLevel = false
	innerRule := compileParseExpression(&subCtx, child)

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentNOrMore(ctx.grammarID, 1, innerRule)
		}
		return ctx.builder.Rule.OneOrMore(ctx.grammarID, ctx.nodeKind, innerRule)
	}

	label := grammarSubLabel(ctx.ruleName, "REPEAT_PLUS", ctx.counts)
	return ctx.builder.Rule.TransparentNOrMore(label, 1, innerRule)
}

func compileStar(ctx *parseCompileCtx, node *Node) CompiledRule {
	child := extractSingleChild(node)
	subCtx := *ctx
	subCtx.rootLevel = false
	innerRule := compileParseExpression(&subCtx, child)

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentZeroOrMore(ctx.grammarID, innerRule)
		}
		return ctx.builder.Rule.ZeroOrMore(ctx.grammarID, ctx.nodeKind, innerRule)
	}

	label := grammarSubLabel(ctx.ruleName, "REPEAT_STAR", ctx.counts)
	return ctx.builder.Rule.TransparentZeroOrMore(label, innerRule)
}

func compileEmit(ctx *parseCompileCtx, node *Node) CompiledRule {
	outputNodeKind := nodeSingleTokenContent(node.FindFirstKind(NodeParseNodeName))
	targetToken := nodeSingleTokenContent(node.FindFirstKind(NodeParseTokenReference))
	var label syntaxa.GrammarLabel
	if ctx.rootLevel {
		label = ctx.grammarID
	} else {
		label = grammarSubLabel(ctx.ruleName, "EMIT", ctx.counts)
	}
	return ctx.builder.Token.Expect(label, outputNodeKind, targetToken)
}

func compileVirtual(ctx *parseCompileCtx, node *Node) CompiledRule {
	targetToken := nodeSingleTokenContent(node.FindFirstKind(NodeParseTokenReference))
	var label syntaxa.GrammarLabel
	if ctx.rootLevel {
		label = ctx.grammarID
	} else {
		label = grammarSubLabel(ctx.ruleName, "VIRTUAL", ctx.counts)
	}
	return ctx.builder.Token.ExpectVirtual(label, targetToken)
}

func compileNest(ctx *parseCompileCtx, node *Node) CompiledRule {
	openToken := nodeSingleTokenContent(node.FindFirstKind(NodeParseNestOpenToken))
	closeToken := nodeSingleTokenContent(node.FindFirstKind(NodeParseNestCloseToken))

	bodyNode := node.FindFirstKind(NodeParseNestBody)
	subCtx := *ctx
	subCtx.rootLevel = false
	innerRule := compileParseExpression(&subCtx, extractSingleChild(bodyNode))

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Rule.TransparentNest(ctx.grammarID, openToken, closeToken, innerRule)
		}
		return ctx.builder.Rule.Nest(ctx.grammarID, ctx.nodeKind, openToken, closeToken, innerRule)
	}

	label := grammarSubLabel(ctx.ruleName, "NEST", ctx.counts)
	return ctx.builder.Rule.TransparentNest(label, openToken, closeToken, innerRule)
}

func compileReference(ctx *parseCompileCtx, node *Node) CompiledRule {
	refNode := node.FindFirstKind(NodeParseRuleReference)
	if refNode == nil {
		panic("compiler error: NodeParseOpRef missing NodeParseRuleReference child")
	}

	targetRuleName := nodeSingleTokenContent(refNode)
	targetGrammarID := syntaxa.GrammarLabel(targetRuleName)

	var label syntaxa.GrammarLabel
	if ctx.rootLevel {
		label = ctx.grammarID
	} else {
		label = grammarSubLabel(ctx.ruleName, "REF", ctx.counts)
	}
	return ctx.builder.Rule.Reference(label, targetGrammarID)
}

func compileGroup(ctx *parseCompileCtx, node *Node) CompiledRule {
	child := extractSingleChild(node)
	return compileParseExpression(ctx, child)
}

func compileTokenMatch(ctx *parseCompileCtx, node *Node) CompiledRule {
	targetToken := nodeSingleTokenContent(node)

	if ctx.rootLevel {
		if ctx.transparent {
			return ctx.builder.Token.ExpectVirtual(ctx.grammarID, targetToken)
		}
		return ctx.builder.Token.Expect(ctx.grammarID, ctx.nodeKind, targetToken)
	}

	label := grammarSubLabel(ctx.ruleName, "TOKEN", ctx.counts)
	return ctx.builder.Token.Expect(label, "", targetToken)
}

func extractSingleChild(node *Node) *Node {
	children := node.Children()
	if len(children) != 1 {
		panic(fmt.Errorf("compiler error: expected 1 child for %s, got %d", node.Kind(), len(children)))
	}
	return children[0]
}

func compilePrattExprDef(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	ruleName string,
	ruleNode *Node,
) CompiledRule {
	grammarID := syntaxa.GrammarLabel(ruleName)

	bodyNode := ruleNode.FindFirstKind(NodePrattExprBody)
	if bodyNode == nil {
		panic("compiler error: pratt rule missing body")
	}

	config := buildPrattConfig(builder, grammarID, bodyNode)
	compiledExpr := builder.Pratt.Expression(grammarID, config)

	return builder.Rule.Define(compiledExpr)
}

func buildPrattConfig(
	builder *CompilerRuleBuilder,
	grammarID syntaxa.GrammarLabel,
	bodyNode *Node,
) rule.PrattConfig[rune, string, string, string, string] {
	config := rule.PrattConfig[rune, string, string, string, string]{}

	for _, actualCat := range bodyNode.Children() {
		switch actualCat.Kind() {
		case NodePrattPrimary:
			config.Primary = compilePrattPrimary(builder, grammarID, actualCat)
		case NodePrattPrefix:
			config.PrefixOps, config.PrefixRuleOps = compilePrattPrefixOps(builder, grammarID, actualCat)
		case NodePrattInfix:
			config.InfixOps, config.InfixRuleOps = compilePrattInfixOps(builder, grammarID, actualCat)
		case NodePrattPostfix:
			config.PostfixOps, config.PostfixRuleOps = compilePrattPostfixOps(builder, grammarID, actualCat)
		case NodePrattImplicit:
			config.ImplicitInfix = compilePrattImplicitOp(actualCat)
		default:
			panic(fmt.Errorf("compiler error: unknown pratt category: %s", actualCat.Kind()))
		}
	}

	return config
}

// ------------------------------- PRATT CATEGORIES -------------------------------

func compilePrattPrimary(
	builder *CompilerRuleBuilder,
	grammarID syntaxa.GrammarLabel,
	node *Node,
) CompiledRule {
	body := node.FindFirstKind(NodePrattPrimaryBody)
	refNode := body.FindFirstKind(NodePrattPrimaryRef)

	targetRuleName := nodeSingleTokenContent(refNode.FindFirstKind(NodeParseRuleReference))
	targetGrammarID := syntaxa.GrammarLabel(targetRuleName)

	return builder.Rule.Reference(grammarID, targetGrammarID)
}

func compilePrattPrefixOps(
	builder *CompilerRuleBuilder,
	grammarID syntaxa.GrammarLabel,
	node *Node,
) ([]rule.PrattPrefixOp[string, string], []rule.PrattPrefixRuleOp[rune, string, string, string, string]) {
	bodyNode := node.FindFirstKind(NodePrattPrefixBody)

	tokOps := make([]rule.PrattPrefixOp[string, string], 0)
	ruleOps := make([]rule.PrattPrefixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.Children() {
		target, rightBP := extractPrefixData(def)

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
	grammarID syntaxa.GrammarLabel,
	node *Node,
) ([]rule.PrattInfixOp[string, string], []rule.PrattInfixRuleOp[rune, string, string, string, string]) {
	bodyNode := node.FindFirstKind(NodePrattInfixBody)

	tokOps := make([]rule.PrattInfixOp[string, string], 0)
	ruleOps := make([]rule.PrattInfixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.Children() {
		target, leftBP, rightBP := extractInfixData(def)

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
	grammarID syntaxa.GrammarLabel,
	node *Node,
) ([]rule.PrattPostfixOp[string, string], []rule.PrattPostfixRuleOp[rune, string, string, string, string]) {
	bodyNode := node.FindFirstKind(NodePrattPostfixBody)

	tokOps := make([]rule.PrattPostfixOp[string, string], 0)
	ruleOps := make([]rule.PrattPostfixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.Children() {
		target, leftBP := extractPostfixData(def)

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
	body := node.FindFirstKind(NodePrattImplicitBody)
	def := body.FindFirstKind(NodePrattImplicitDef)

	nodeKind := nodeSingleTokenContent(def.FindFirstKind(NodeParseNodeName))
	leftBP := extractIntContent(def.FindFirstKind(NodePrattLeftPrecedenceValue))
	rightBP := extractIntContent(def.FindFirstKind(NodePrattRightPrecedenceValue))

	return &rule.PrattImplicitInfix[string, string]{
		LeftBP:   leftBP,
		RightBP:  rightBP,
		NodeKind: nodeKind,
	}
}

// ------------------------------- PRATT OPERATOR BUILDERS -------------------------------

type OperatorTarget struct {
	IsRule   bool
	Ref      string
	NodeKind string
}

func extractOperatorTarget(defNode *Node) OperatorTarget {
	nodeKind := nodeSingleTokenContent(defNode.FindFirstKind(NodeParseNodeName))

	if tokenRef := defNode.FindFirstKind(NodeParseTokenReference); tokenRef != nil {
		return OperatorTarget{
			IsRule:   false,
			Ref:      nodeSingleTokenContent(tokenRef),
			NodeKind: nodeKind,
		}
	}

	if ruleRef := defNode.FindFirstKind(NodeParseRuleReference); ruleRef != nil {
		return OperatorTarget{
			IsRule:   true,
			Ref:      nodeSingleTokenContent(ruleRef),
			NodeKind: nodeKind,
		}
	}

	panic("compiler error: valid pratt operator target missing")
}

func extractPrefixData(defNode *Node) (OperatorTarget, int) {
	target := extractOperatorTarget(defNode)
	rightBP := extractIntContent(defNode.FindFirstKind(NodePrattRightPrecedenceValue))
	return target, rightBP
}

func extractPostfixData(defNode *Node) (OperatorTarget, int) {
	target := extractOperatorTarget(defNode)
	leftBP := extractIntContent(defNode.FindFirstKind(NodePrattLeftPrecedenceValue))
	return target, leftBP
}

func extractInfixData(defNode *Node) (OperatorTarget, int, int) {
	target := extractOperatorTarget(defNode)
	leftBP := extractIntContent(defNode.FindFirstKind(NodePrattLeftPrecedenceValue))
	rightBP := extractIntContent(defNode.FindFirstKind(NodePrattRightPrecedenceValue))
	return target, leftBP, rightBP
}

// -------------------------------------------------------- HELPERS

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
	meta, ok := nodeContains(node, NodeMetaSection)
	if !ok {
		return false
	}

	kvps := meta.FindAllKind(NodeMetaKeyValuePair)
	for _, kvp := range kvps {
		if pred(kvp) {
			return true
		}
	}

	return false
}

func isEOFTrueMetaKVP(kvpNode *Node) bool {
	key := kvpNode.FindFirstKind(NodeMetaKey)
	if key == nil || !lexemeRawContentEqualTo(key.Tokens()[0], "EOF") {
		return false
	}

	valueNode := kvpNode.FindFirstKind(NodeMetaValue)
	if valueNode == nil {
		return false
	}

	return lexemeKindEqualTo(valueNode.Tokens()[0], TokKWTrue)
}

func checkRuleForEOFMeta(rule *Node) string {
	if nodeHasMetaByPred(rule, isEOFTrueMetaKVP) {
		identifier := rule.FindFirstKind(NodeLexRuleTokenName).Tokens()[0]
		return lexemeRawContent(identifier)
	}
	return ""
}

func checkForEOFLexeme(lexerSection *Node) string {
	for _, rule := range lexerSection.FindAllKind(NodeLexRule) {
		if token := checkRuleForEOFMeta(rule); token != "" {
			return token
		}
	}
	return ""
}

func nodeSingleTokenContent(node *Node) string {
	tks := node.Tokens()
	if len(tks) != 1 {
		panic("engine error: single token content extraction requires node to have exactly 1 token")
	}

	return lexemeRawContent(tks[0])
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
