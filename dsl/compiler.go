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

	ruleset := lspecCompiler.compileRuleset(rootNode, env)
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

func (c *compiler) compileRuleset(rootNode *Node, env *SemanticEnv) *LexerRuleset {
	patternTable := c.compilePatterns(rootNode, env)

	ruleset := lexarch.LexingRulesetCreate[rune, string, string](
		lexarch.TokenResolutionStepLongestThenPriority[string],
	)

	lexSection := rootNode.FindFirstKind(NodeLexSection)
	lexRules := c.gatherRules(lexSection, patternTable, env)

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

func (c *compiler) gatherRules(
	sectionNode *Node,
	patternTable map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) []lexRule {
	rules := sectionNode.FindAllKind(NodeLexRule)
	out := make([]lexRule, 0, len(rules))

	for _, rule := range rules {
		if !nodeHasMetaByPred(rule, isEOFTrueMetaKVP) {
			out = append(out, c.constructLexRule(rule, patternTable, env))
		}
	}

	return out
}

func (c *compiler) constructLexRule(
	ruleNode *Node,
	patternTable map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) lexRule {
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
		if env.Patterns[target] == nil {
			panic(fmt.Errorf("unresolved pattern reference: '%s'", target))
		}
		p, ok := patternTable[target]
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

func (c *compiler) compilePatterns(rootNode *Node, env *SemanticEnv) map[string]pattern.RegulaAST[rune] {
	out := make(map[string]pattern.RegulaAST[rune])
	temp := make(map[string]pattern.RegulaAST[rune])
	patternSection := rootNode.FindFirstKind(NodePatternSection)
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

		pattern := c.compilePatternExpression(exprNode, temp, env)

		temp[defName] = pattern
		if !env.LocalPatterns[defName] {
			out[defName] = pattern
		}
	}

	return out
}

func (c *compiler) compilePatternExpression(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) pattern.RegulaAST[rune] {

	kind := node.Kind()

	switch kind {
	case NodeCharLiteral:
		return c.charLiteralToPattern(node)
	case NodeVarRef:
		return c.varRefToPattern(node, variables, env)
	case NodePatternConcat:
		return c.concatToPattern(node, variables, env)
	case NodePatternAlternation:
		return c.alternationToPattern(node, variables, env)
	case NodePatternRange:
		return c.rangeToPattern(node)
	case NodePatternStar:
		return c.starToPattern(node, variables, env)
	case NodePatternPlus:
		return c.plusToPattern(node, variables, env)
	case NodePatternOptional:
		return c.optionalToPattern(node, variables, env)
	case NodeRepetition:
		return c.repetitionToPattern(node, variables, env)
	case NodePatternGroup:
		return c.groupToPattern(node, variables, env)
	case NodePatternNegation:
		return c.negationToPattern(node)
	case NodePatternAny:
		return c.factory.NegatedClass()
	default:
		panic(fmt.Errorf("engine error: unsupported pattern kind: '%s'", kind))
	}
}

// --- PATTERNS

func (c *compiler) concatToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) pattern.RegulaAST[rune] {
	children := node.Children()

	concatPattern := c.compilePatternExpression(children[0], variables, env)

	for i, child := range children {
		if i == 0 {
			continue
		}

		pattern := c.compilePatternExpression(child, variables, env)
		concatPattern = concatPattern.Then(pattern)
	}

	return concatPattern
}

func (c *compiler) alternationToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) pattern.RegulaAST[rune] {
	children := node.Children()

	alternationPattern := c.compilePatternExpression(children[0], variables, env)

	for i, child := range children {
		if i == 0 {
			continue
		}

		pattern := c.compilePatternExpression(child, variables, env)
		alternationPattern = alternationPattern.Or(pattern)
	}

	return alternationPattern
}

func (c *compiler) rangeToPattern(
	node *Node,
) pattern.RegulaAST[rune] {
	charRange := c.extractPatternRange(node)
	return c.factory.Class(charRange)
}

func (c *compiler) starToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 1 {
		panic("error: star pattern must have exactly 1 child")
	}

	childPattern := c.compilePatternExpression(children[0], variables, env)
	return childPattern.Star()
}

func (c *compiler) plusToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 1 {
		panic("error: plus pattern must have exactly 1 child")
	}

	childPattern := c.compilePatternExpression(children[0], variables, env)
	return childPattern.Plus()
}

func (c *compiler) optionalToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 1 {
		panic("error: optional pattern must have exactly 1 child")
	}

	childPattern := c.compilePatternExpression(children[0], variables, env)
	return childPattern.Optional()
}

func (c *compiler) repetitionToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 2 {
		panic("compiler error: repetition node must have exactly 2 children (pattern and settings)")
	}

	childPattern := c.compilePatternExpression(children[0], variables, env)
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

func (c *compiler) groupToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
	env *SemanticEnv,
) pattern.RegulaAST[rune] {
	children := node.Children()
	if len(children) != 1 {
		panic("engine error: group node must have exactly 1 child")
	}

	return c.compilePatternExpression(children[0], variables, env)
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

func (c *compiler) varRefToPattern(node *Node, variables map[string]pattern.RegulaAST[rune], env *SemanticEnv) pattern.RegulaAST[rune] {
	targetName := VarRefTargetName(node)
	if env.Patterns[targetName] == nil {
		panic(fmt.Errorf("error: pattern '%s' cannot be resolved", targetName))
	}
	targetPattern, ok := variables[targetName]
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
		compiled, override := compileParseRuleDefinition(ruleBuilder, env, ruleName, ruleNode)
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

func compileParseRuleDefinition(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	ruleName string,
	ruleNode *Node,
) (CompiledRule, syntaxa.GrammarLabel) {
	grammarID := syntaxa.GrammarLabel(ruleName)
	nodeNameNode := ruleNode.FindFirstKind(NodeParseNodeName)

	nodeKind := nodeSingleTokenContent(nodeNameNode)

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
	counts := make(map[string]int)

	transparent := ruleNode.FindFirstKind(NodeRuleModifierTransparent) != nil
	compiledExpr := compileParseExpression(builder, env, grammarID, nodeKind, rootExpr, ruleName, counts, true, transparent)

	// Inject Sync modifier wrapping logic
	syncNode := ruleNode.FindFirstKind(NodeRuleModifierSync)
	if syncNode != nil {
		var syncTokens []string
		for _, tNode := range syncNode.FindAllKind(NodeSyncToken) {
			syncTokens = append(syncTokens, nodeSingleTokenContent(tNode))
		}
		if len(syncTokens) > 0 {
			compiledExpr = builder.Rule.RecoverSync(compiledExpr, syncTokens...)
		}
	}

	return builder.Rule.Define(compiledExpr), ""
}

func compileParseExpression(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	kind := node.Kind()

	switch kind {
	case NodeParseConcat:
		return compileConcat(builder, env, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
	case NodeParseAlternation:
		return compileAlternation(builder, env, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
	case NodeParseModifierPredict:
		return compilePredict(builder, env, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
	case NodeParseOptional:
		return compileOptional(builder, env, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
	case NodeParseOpRef:
		return compileReference(builder, grammarID, node, ruleName, counts, rootLevel)
	case NodeIdentifier:
		return compileTokenMatch(builder, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
	case NodeParsePlus:
		return compilePlus(builder, env, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
	case NodeParseStar:
		return compileStar(builder, env, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
	case NodeParseOpEmit:
		return compileEmit(builder, grammarID, node, ruleName, counts, rootLevel)
	case NodeParseOpSuppress:
		return compileVirtual(builder, grammarID, node, ruleName, counts, rootLevel)
	case NodeParseOpNest:
		return compileNest(builder, env, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
	case NodeParseGroup:
		return compileGroup(builder, env, grammarID, nodeKind, node, ruleName, counts, rootLevel, transparent)
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

func compileConcat(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	flatNodes := flattenNodesByKind(node, NodeParseConcat)

	rules := make([]CompiledRule, 0, len(flatNodes))
	for _, child := range flatNodes {
		rules = append(rules, compileParseExpression(builder, env, grammarID, nodeKind, child, ruleName, counts, false, transparent))
	}

	if rootLevel {
		if transparent {
			return builder.Rule.TransparentSequence(grammarID, rules...)
		}
		return builder.Rule.Sequence(grammarID, nodeKind, rules...)
	}

	label := grammarSubLabel(ruleName, "SEQUENCE", counts)
	return builder.Rule.TransparentSequence(label, rules...)
}

func compileAlternation(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	flatNodes := flattenNodesByKind(node, NodeParseAlternation)

	rules := make([]CompiledRule, 0, len(flatNodes))
	for _, child := range flatNodes {
		// Notice how clean this is now. No dynamic AST guessing needed.
		compiled := compileParseExpression(builder, env, grammarID, nodeKind, child, ruleName, counts, false, transparent)
		rules = append(rules, compiled)
	}

	var label syntaxa.GrammarLabel
	if rootLevel {
		label = grammarID
	} else {
		label = grammarSubLabel(ruleName, "CHOICE", counts)
	}
	return builder.Rule.Choice(label, rules...)
}

func compilePredict(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	children := node.Children()
	if len(children) != 2 {
		panic("compiler error: predict modifier must have a lookahead list and a target expression")
	}

	listNode := children[0]
	targetNode := children[1]

	innerRule := compileParseExpression(builder, env, grammarID, nodeKind, targetNode, ruleName, counts, rootLevel, transparent)

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

	return builder.Rule.Predict(innerRule, func(ctx *syntaxa.SelectRuleContext[rune, string, string]) bool {
		for _, p := range preds {
			if ctx.Peek(p.offset).Token != p.token {
				return false
			}
		}
		return true
	})
}

func compileOptional(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	child := extractSingleChild(node)
	innerRule := compileParseExpression(builder, env, grammarID, nodeKind, child, ruleName, counts, false, transparent)

	return builder.Rule.Optional(innerRule)
}

func compilePlus(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	child := extractSingleChild(node)
	innerRule := compileParseExpression(builder, env, grammarID, nodeKind, child, ruleName, counts, false, transparent)

	if rootLevel {
		if transparent {
			return builder.Rule.TransparentNOrMore(grammarID, 1, innerRule)
		}
		return builder.Rule.OneOrMore(grammarID, nodeKind, innerRule)
	}

	label := grammarSubLabel(ruleName, "REPEAT_PLUS", counts)
	return builder.Rule.TransparentNOrMore(label, 1, innerRule)
}

func compileStar(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	child := extractSingleChild(node)
	innerRule := compileParseExpression(builder, env, grammarID, nodeKind, child, ruleName, counts, false, transparent)

	if rootLevel {
		if transparent {
			return builder.Rule.TransparentZeroOrMore(grammarID, innerRule)
		}
		return builder.Rule.ZeroOrMore(grammarID, nodeKind, innerRule)
	}

	label := grammarSubLabel(ruleName, "REPEAT_STAR", counts)
	return builder.Rule.TransparentZeroOrMore(label, innerRule)
}

func compileEmit(builder *CompilerRuleBuilder, grammarID syntaxa.GrammarLabel, node *Node, ruleName string, counts map[string]int, rootLevel bool) CompiledRule {
	outputNodeKind := nodeSingleTokenContent(node.FindFirstKind(NodeParseNodeName))
	targetToken := nodeSingleTokenContent(node.FindFirstKind(NodeParseTokenReference))
	var label syntaxa.GrammarLabel
	if rootLevel {
		label = grammarID
	} else {
		label = grammarSubLabel(ruleName, "EMIT", counts)
	}
	return builder.Token.Expect(label, outputNodeKind, targetToken)
}

func compileVirtual(builder *CompilerRuleBuilder, grammarID syntaxa.GrammarLabel, node *Node, ruleName string, counts map[string]int, rootLevel bool) CompiledRule {
	targetToken := nodeSingleTokenContent(node.FindFirstKind(NodeParseTokenReference))
	var label syntaxa.GrammarLabel
	if rootLevel {
		label = grammarID
	} else {
		label = grammarSubLabel(ruleName, "VIRTUAL", counts)
	}
	return builder.Token.ExpectVirtual(label, targetToken)
}

func compileNest(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	openToken := nodeSingleTokenContent(node.FindFirstKind(NodeParseNestOpenToken))
	closeToken := nodeSingleTokenContent(node.FindFirstKind(NodeParseNestCloseToken))

	bodyNode := node.FindFirstKind(NodeParseNestBody)
	innerRule := compileParseExpression(builder, env, grammarID, nodeKind, extractSingleChild(bodyNode), ruleName, counts, false, transparent)

	if rootLevel {
		if transparent {
			return builder.Rule.TransparentNest(grammarID, openToken, closeToken, innerRule)
		}
		return builder.Rule.Nest(grammarID, nodeKind, openToken, closeToken, innerRule)
	}

	label := grammarSubLabel(ruleName, "NEST", counts)
	return builder.Rule.TransparentNest(label, openToken, closeToken, innerRule)
}

func compileReference(
	builder *CompilerRuleBuilder,
	grammarID syntaxa.GrammarLabel,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
) CompiledRule {
	refNode := node.FindFirstKind(NodeParseRuleReference)
	if refNode == nil {
		panic("compiler error: NodeParseOpRef missing NodeParseRuleReference child")
	}

	targetRuleName := nodeSingleTokenContent(refNode)
	targetGrammarID := syntaxa.GrammarLabel(targetRuleName)

	var label syntaxa.GrammarLabel
	if rootLevel {
		label = grammarID
	} else {
		label = grammarSubLabel(ruleName, "REF", counts)
	}
	return builder.Rule.Reference(label, targetGrammarID)
}

func compileGroup(
	builder *CompilerRuleBuilder,
	env *SemanticEnv,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	child := extractSingleChild(node)
	return compileParseExpression(builder, env, grammarID, nodeKind, child, ruleName, counts, rootLevel, transparent)
}

func compileTokenMatch(
	builder *CompilerRuleBuilder,
	grammarID syntaxa.GrammarLabel,
	nodeKind string,
	node *Node,
	ruleName string,
	counts map[string]int,
	rootLevel bool,
	transparent bool,
) CompiledRule {
	targetToken := nodeSingleTokenContent(node)

	if rootLevel {
		if transparent {
			return builder.Token.ExpectVirtual(grammarID, targetToken)
		}
		return builder.Token.Expect(grammarID, nodeKind, targetToken)
	}

	label := grammarSubLabel(ruleName, "TOKEN", counts)
	return builder.Token.Expect(label, "", targetToken)
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
	bodyNode := node.FindFirstKind(NodePrattOperatorBody)

	tokOps := make([]rule.PrattPrefixOp[string, string], 0)
	ruleOps := make([]rule.PrattPrefixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.Children() {
		data := extractOperatorData(def)

		if data.IsRule {
			ruleOps = append(ruleOps, rule.PrattPrefixRuleOp[rune, string, string, string, string]{
				RightBP:  data.Precedence,
				NodeKind: data.NodeKind,
				Rule:     builder.Rule.Reference(grammarID, syntaxa.GrammarLabel(data.TokenOrRef)),
			})
		} else {
			tokOps = append(tokOps, rule.PrattPrefixOp[string, string]{
				Token:             data.TokenOrRef,
				RightBP:           data.Precedence,
				NodeKind:          data.NodeKind,
				TokenGrammarLabel: syntaxa.GrammarLabel(data.TokenOrRef),
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
	bodyNode := node.FindFirstKind(NodePrattOperatorBody)

	tokOps := make([]rule.PrattInfixOp[string, string], 0)
	ruleOps := make([]rule.PrattInfixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.Children() {
		data := extractOperatorData(def)

		if data.IsRule {
			ruleOps = append(ruleOps, rule.PrattInfixRuleOp[rune, string, string, string, string]{
				LeftBP:   data.Precedence,
				RightBP:  data.Precedence - 1,
				NodeKind: data.NodeKind,
				Rule:     builder.Rule.Reference(grammarID, syntaxa.GrammarLabel(data.TokenOrRef)),
			})
		} else {
			tokOps = append(tokOps, rule.PrattInfixOp[string, string]{
				Token:             data.TokenOrRef,
				LeftBP:            data.Precedence,
				RightBP:           data.Precedence - 1,
				NodeKind:          data.NodeKind,
				TokenGrammarLabel: syntaxa.GrammarLabel(data.TokenOrRef),
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
	bodyNode := node.FindFirstKind(NodePrattOperatorBody)

	tokOps := make([]rule.PrattPostfixOp[string, string], 0)
	ruleOps := make([]rule.PrattPostfixRuleOp[rune, string, string, string, string], 0)

	for _, def := range bodyNode.Children() {
		data := extractOperatorData(def)

		if data.IsRule {
			ruleOps = append(ruleOps, rule.PrattPostfixRuleOp[rune, string, string, string, string]{
				LeftBP:   data.Precedence,
				NodeKind: data.NodeKind,
				Rule:     builder.Rule.Reference(grammarID, syntaxa.GrammarLabel(data.TokenOrRef)),
			})
		} else {
			tokOps = append(tokOps, rule.PrattPostfixOp[string, string]{
				Token:             data.TokenOrRef,
				LeftBP:            data.Precedence,
				NodeKind:          data.NodeKind,
				TokenGrammarLabel: syntaxa.GrammarLabel(data.TokenOrRef),
			})
		}
	}

	return tokOps, ruleOps
}

func compilePrattImplicitOp(node *Node) *rule.PrattImplicitInfix[string, string] {
	body := node.FindFirstKind(NodePrattImplicitBody)
	def := body.FindFirstKind(NodePrattImplicitDef)

	nodeKind := nodeSingleTokenContent(def.FindFirstKind(NodeParseNodeName))
	precedence := extractIntContent(def.FindFirstKind(NodePrattPrecedenceValue))

	return &rule.PrattImplicitInfix[string, string]{
		LeftBP:   precedence,
		RightBP:  precedence - 1,
		NodeKind: nodeKind,
	}
}

// ------------------------------- PRATT OPERATOR BUILDERS -------------------------------

type ExtractedOperator struct {
	IsRule     bool
	TokenOrRef string
	NodeKind   string
	Precedence int
}

func extractOperatorData(defNode *Node) ExtractedOperator {
	nodeKind := nodeSingleTokenContent(defNode.FindFirstKind(NodeParseNodeName))
	precedence := extractIntContent(defNode.FindFirstKind(NodePrattPrecedenceValue))

	if tokenRef := defNode.FindFirstKind(NodeParseTokenReference); tokenRef != nil {
		return ExtractedOperator{
			IsRule:     false,
			TokenOrRef: nodeSingleTokenContent(tokenRef),
			NodeKind:   nodeKind,
			Precedence: precedence,
		}
	}

	if ruleRef := defNode.FindFirstKind(NodeParseRuleReference); ruleRef != nil {
		return ExtractedOperator{
			IsRule:     true,
			TokenOrRef: nodeSingleTokenContent(ruleRef),
			NodeKind:   nodeKind,
			Precedence: precedence,
		}
	}

	panic("compiler error: valid pratt operator target missing")
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
