package dsl

import (
	"autarch/pattern"
	"fmt"
	"langspec"
	"lexarch"
	"strconv"
	"strings"
)

type LexerSpec = langspec.LexerSpec[rune, string, string, string]
type LexerRuleset = lexarch.LexingRuleset[rune, string, string]

type CompiledLangSpec struct {
	dslName    string
	dslVersion string

	lexerSpec *LexerSpec
	eofToken  string
}

type compiler struct {
	factory *pattern.RegulaASTFactory[rune]
}

func compileTree(comp *LangSpecCompiler, rootNode *Node) *CompiledLangSpec {
	dslName, dslVersion := getInfoFromHeader(rootNode.FindFirstKind(NodeHeader))

	eofToken := getEOFToken(rootNode)
	domain := lexarch.LexarchRuneDomain()
	spec := langspec.LexerSpecCreate[rune, string, string](
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

	ruleset := lspecCompiler.compileRuleset(rootNode)
	spec.WithRuleset("default", *ruleset)

	return &CompiledLangSpec{
		dslName:    dslName,
		dslVersion: dslVersion,
		lexerSpec:  spec,
		eofToken:   eofToken,
	}
}

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

func (c *compiler) compileRuleset(rootNode *Node) *LexerRuleset {
	patternTable := c.compilePatterns(rootNode)

	ruleset := lexarch.LexingRulesetCreate[rune, string, string](
		lexarch.TokenResolutionStepLongestThenPriority[string],
	)

	lexSection := rootNode.FindFirstKind(NodeLexSection)
	lexRules := c.gatherRules(lexSection, patternTable)

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
) []lexRule {
	rules := sectionNode.FindAllKind(NodeLexRule)
	out := make([]lexRule, 0)

	for _, rule := range rules {
		if !nodeHasMetaByPred(rule, func(metaKVPNode *Node) bool {
			key := metaKVPNode.Children()[0]
			if lexemeRawContentEqualTo(key.Tokens()[0], "EOF") {
				value := metaKVPNode.Children()[1]
				if lexemeKindEqualTo(value.Tokens()[0], TokKWTrue) {
					return true
				}
			}

			return false
		}) {
			out = append(out, c.constructLexRule(rule, patternTable))
		}
	}

	return out
}

func (c *compiler) constructLexRule(
	ruleNode *Node,
	patternTable map[string]pattern.RegulaAST[rune],
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
		target := resolveVarRefTargetName(patternNode)
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

func (c *compiler) compilePatterns(rootNode *Node) map[string]pattern.RegulaAST[rune] {
	out := make(map[string]pattern.RegulaAST[rune])
	temp := make(map[string]pattern.RegulaAST[rune])
	patternSection := rootNode.FindFirstKind(NodePatternSection)
	definitions := patternSection.FindAllKind(NodePatternDefinition)

	for _, definition := range definitions {
		defName := lexemeRawContent(definition.FindFirstKind(NodePatternDefName).Tokens()[0])

		// 1. Isolate: Extract the actual expression node by skipping modifiers
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

		// 2. Delegate: Pass the pure expression node to the dispatcher
		pattern := c.compilePatternExpression(exprNode, temp)

		temp[defName] = pattern
		if _, contains := nodeContains(definition, NodeLocalVariable); !contains {
			out[defName] = pattern
		}
	}

	return out
}

func (c *compiler) compilePatternExpression(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
) pattern.RegulaAST[rune] {

	kind := node.Kind()

	switch kind {
	case NodeCharLiteral:
		return c.charLiteralToPattern(node)
	case NodeStringLiteral:
		return c.stringLiteralToPattern(node)
	case NodeVarRef:
		return c.varRefToPattern(node, variables)
	case NodePatternConcat:
		return c.concatToPattern(node, variables)
	case NodePatternAlternation:
		return c.alternationToPattern(node, variables)
	case NodePatternRange:
		return c.rangeToPattern(node)
	case NodePatternStar:
		return c.starToPattern(node, variables)
	case NodePatternPlus:
		return c.plusToPattern(node, variables)
	case NodePatternOptional:
		return c.optionalToPattern(node, variables)
	case NodePatternRepetition:
		return c.repetitionToPattern(node, variables)
	case NodePatternGroup:
		return c.groupToPattern(node, variables)
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
) pattern.RegulaAST[rune] {
	children := node.Children()

	concatPattern := c.compilePatternExpression(children[0], variables)

	for i, child := range children {
		if i == 0 {
			continue
		}

		pattern := c.compilePatternExpression(child, variables)
		concatPattern = concatPattern.Then(pattern)
	}

	return concatPattern
}

func (c *compiler) alternationToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
) pattern.RegulaAST[rune] {
	children := node.Children()

	alternationPattern := c.compilePatternExpression(children[0], variables)

	for i, child := range children {
		if i == 0 {
			continue
		}

		pattern := c.compilePatternExpression(child, variables)
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
) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 1 {
		panic("error: star pattern must have exactly 1 child")
	}

	childPattern := c.compilePatternExpression(children[0], variables)
	return childPattern.Star()
}

func (c *compiler) plusToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 1 {
		panic("error: plus pattern must have exactly 1 child")
	}

	childPattern := c.compilePatternExpression(children[0], variables)
	return childPattern.Plus()
}

func (c *compiler) optionalToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 1 {
		panic("error: optional pattern must have exactly 1 child")
	}

	childPattern := c.compilePatternExpression(children[0], variables)
	return childPattern.Optional()
}

func (c *compiler) repetitionToPattern(
	node *Node,
	variables map[string]pattern.RegulaAST[rune],
) pattern.RegulaAST[rune] {
	children := node.Children()

	if len(children) != 2 {
		panic("compiler error: repetition node must have exactly 2 children (pattern and settings)")
	}

	childPattern := c.compilePatternExpression(children[0], variables)
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
) pattern.RegulaAST[rune] {
	children := node.Children()
	if len(children) != 1 {
		panic("engine error: group node must have exactly 1 child")
	}

	return c.compilePatternExpression(children[0], variables)
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
		// The semantic enforcer. Kills the compilation if a user writes !("string") or !($Var).
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

func (c *compiler) stringLiteralToPattern(node *Node) pattern.RegulaAST[rune] {
	content := nodeFormattedContent(node, ATTRIBUTE_LITERAL_STRING_VALUE)
	return c.factory.Literal([]rune(content)...)
}

func (c *compiler) varRefToPattern(node *Node, variables map[string]pattern.RegulaAST[rune]) pattern.RegulaAST[rune] {
	targetName := resolveVarRefTargetName(node)
	targetPattern, ok := variables[targetName]
	if !ok {
		panic(fmt.Errorf("error: pattern '%s' cannot be resolved", targetName))
	}

	return targetPattern
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
		return false // No meta section, definitely does not have the value
	}

	kvps := meta.FindAllKind(NodeMetaKeyValuePair)
	for _, kvp := range kvps {
		if pred(kvp) {
			return true
		}
	}

	return false
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

func resolveVarRefTargetName(varRef *Node) string {
	tokens := varRef.Tokens()
	if len(tokens) >= 2 {
		return strings.TrimSpace(string(tokens[1].Raw))
	}
	return ""
}

func checkForEOFLexeme(lexerSection *Node) string {
	lexRules := lexerSection.FindAllKind(NodeLexRule)
	for _, rule := range lexRules {
		metaKeyValuePairs := rule.FindAllKind(NodeMetaKeyValuePair)
		for _, keyValuePair := range metaKeyValuePairs {
			key := keyValuePair.FindFirstKind(NodeMetaKey)
			if !lexemeRawContentEqualTo(key.Tokens()[0], "EOF") {
				continue
			}

			valueNode := keyValuePair.FindFirstKind(NodeMetaValue)
			value := valueNode.Tokens()[0]

			if lexemeKindEqualTo(value, TokKWTrue) {
				identifier := rule.FindFirstKind(NodeLexRuleTokenName).Tokens()[0]
				return lexemeRawContent(identifier)
			}
		}
	}

	return ""
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
