package dsl

import (
	"syntaxa"
	"syntaxa/rule"
)

/*
GrammarDefiner wraps RuleBuilder and derives GrammarIDs from NodeKind and VirtualGrammarID
so grammar definitions pass only node (and optional suffix) or virtual ID instead of triplets.
Syntaxa remains generic; this layer is langspec-specific.
*/

type GrammarDefiner struct {
	rb *RuleBuilder
}

/*
grammarDefinerCreate returns a new GrammarDefiner that uses the given RuleBuilder and
LangSpecGrammarIDFromNode / VirtualGrammarIDToGrammarID for resolving IDs.
*/
func grammarDefinerCreate(rb *RuleBuilder) *GrammarDefiner {
	return &GrammarDefiner{rb: rb}
}

/*
Expect returns a rule that expects the token and creates an LST node of that kind.
GrammarID is derived from (node, suffix). Use suffix "" for the default slot or a disambiguating suffix (e.g. "DSL", "LANGSPEC").
*/
func (g *GrammarDefiner) Expect(node LangSpecParserNodeKind, suffix string, tok LangSpecLexerTokenType) Rule {
	grammarID := LangSpecGrammarIDFromNodeWithSuffix(node, suffix)
	return g.rb.Token.Expect(grammarID, node, tok)
}

/*
expectToken returns a rule that expects the token and creates an LST node. Shorthand for Expect(node, "", tok).
*/
func (g *GrammarDefiner) expectToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	return g.Expect(node, "", tok)
}

/*
expectTokenWithGrammarID returns a rule that expects the token and creates an LST node,
using the given grammarID. Use for header expectations when the node maps to multiple
grammar IDs (e.g. NodeVersion for DSL vs Langspec version) via GrammarIDOverride.
*/
func (g *GrammarDefiner) expectTokenWithGrammarID(grammarID syntaxa.GrammarLabel, node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	return g.rb.Token.Expect(grammarID, node, tok)
}

/*
expectVirtual returns a rule that consumes the token without creating an LST node.
Uses VirtualGrammarIDToGrammarID to resolve the virtual ID to syntaxa.GrammarLabel.
*/
func (g *GrammarDefiner) expectVirtual(v VirtualGrammarID, tok LangSpecLexerTokenType) Rule {
	return g.rb.Token.ExpectVirtual(VirtualGrammarIDToGrammarID(v), tok)
}

/*
expectVirtualInRule returns a rule that consumes the token without creating a node,
using the same GrammarID as the enclosing rule (derived from node). Use for
punctuation slots inside a sequence (e.g. semicolon after a lex rule).
*/
func (g *GrammarDefiner) expectVirtualInRule(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	return g.rb.Token.ExpectVirtual(LangSpecGrammarIDFromNode(node), tok)
}

/*
expectOneOf returns a rule that expects one of the given tokens and creates an LST node
of that kind. GrammarID is derived from the node via LangSpecGrammarIDFromNode(node, "").
*/
func (g *GrammarDefiner) expectOneOf(node LangSpecParserNodeKind, tokens ...LangSpecLexerTokenType) Rule {
	grammarID := LangSpecGrammarIDFromNode(node)
	return g.rb.Token.ExpectOneOf(grammarID, node, tokens...)
}

/*
expectOneOfWithGrammarID returns a rule that expects one of the tokens and creates an
LST node, using the given grammarID. Use when the node maps to multiple grammar IDs.
*/
func (g *GrammarDefiner) expectOneOfWithGrammarID(grammarID syntaxa.GrammarLabel, node LangSpecParserNodeKind, tokens ...LangSpecLexerTokenType) Rule {
	return g.rb.Token.ExpectOneOf(grammarID, node, tokens...)
}

/*
expectPair returns a rule that expects two tokens in sequence and creates a single LST node
with both lexemes attached. GrammarID is derived from the node via LangSpecGrammarIDFromNode(node, "").
*/
func (g *GrammarDefiner) expectPair(node LangSpecParserNodeKind, firstToken, secondToken LangSpecLexerTokenType) Rule {
	grammarID := LangSpecGrammarIDFromNode(node)
	return g.rb.Token.ExpectPair(grammarID, node, firstToken, secondToken)
}

/*
expectPairWithChildNodes expects two tokens in sequence and creates a single LST node (constructNode kind)
with both lexemes attached. The grammar has three distinct IDs: construct (concat), first token, second token,
so the IR can attach metascope to the construct and distinct scopes to each token.
*/
func (g *GrammarDefiner) expectPairWithChildNodes(
	constructNode LangSpecParserNodeKind,
	firstTokenNode LangSpecParserNodeKind,
	secondTokenNode LangSpecParserNodeKind,
	firstToken, secondToken LangSpecLexerTokenType,
) Rule {
	concatID := LangSpecGrammarIDFromNode(constructNode)
	firstTokenID := LangSpecGrammarIDFromNode(firstTokenNode)
	secondTokenID := LangSpecGrammarIDFromNode(secondTokenNode)
	return g.rb.Token.ExpectPairWithChildGrammarIDs(concatID, firstTokenID, secondTokenID, constructNode, firstToken, secondToken)
}

/*
sequence starts a chainable sequence for the given node kind. GrammarID is
LangSpecGrammarIDFromNode(nodeKind, suffix). Call Build on the returned SequenceBuilder.
*/
func (g *GrammarDefiner) sequence(nodeKind LangSpecParserNodeKind, suffix string) *SequenceBuilder {
	grammarID := LangSpecGrammarIDFromNodeWithSuffix(nodeKind, suffix)
	return &SequenceBuilder{
		g:         g,
		grammarID: grammarID,
		nodeKind:  nodeKind,
		rules:     make([]Rule, 0),
	}
}

/*
TransparentNestByNode creates a transparent nest (open ... close) with grammar ID derived from (node, suffix).
*/
func (g *GrammarDefiner) TransparentNestByNode(node LangSpecParserNodeKind, suffix string, open, close LangSpecLexerTokenType, body Rule) Rule {
	grammarID := LangSpecGrammarIDFromNodeWithSuffix(node, suffix)
	return g.rb.Rule.TransparentNest(grammarID, open, close, body)
}

/*
RootByNode builds a root rule with grammar ID derived from node (suffix "").
*/
func (g *GrammarDefiner) RootByNode(node LangSpecParserNodeKind, transparent bool, rules ...Rule) Rule {
	grammarID := LangSpecGrammarIDFromNode(node)
	return g.rb.Rule.Root(grammarID, node, transparent, rules...)
}

/*
TransparentZeroOrMoreByNode builds zero-or-more repetition with grammar ID derived from (node, suffix).
*/
func (g *GrammarDefiner) TransparentZeroOrMoreByNode(node LangSpecParserNodeKind, suffix string, rule Rule) Rule {
	grammarID := LangSpecGrammarIDFromNodeWithSuffix(node, suffix)
	return g.rb.Rule.TransparentZeroOrMore(grammarID, rule)
}

/*
NestByNode builds a nest (open ... close) with grammar ID derived from node (suffix "").
*/
func (g *GrammarDefiner) NestByNode(node LangSpecParserNodeKind, open, close LangSpecLexerTokenType, body Rule) Rule {
	grammarID := LangSpecGrammarIDFromNode(node)
	return g.rb.Rule.Nest(grammarID, node, open, close, body)
}

/*
ChoiceByNode builds a choice over rules with grammar ID derived from node (suffix "").
*/
func (g *GrammarDefiner) ChoiceByNode(node LangSpecParserNodeKind, rules ...Rule) Rule {
	grammarID := LangSpecGrammarIDFromNode(node)
	return g.rb.Rule.Choice(grammarID, rules...)
}

/*
OptionalSuffixByNode builds an optional-suffix rule (rule followed by optional tok) with grammar ID and node derived from node (suffix "").
*/
func (g *GrammarDefiner) OptionalSuffixByNode(node LangSpecParserNodeKind, rule Rule, tok LangSpecLexerTokenType) Rule {
	grammarID := LangSpecGrammarIDFromNode(node)
	return g.rb.Rule.OptionalSuffix(grammarID, node, rule, tok)
}

/*
InfixOp returns a Pratt infix operator descriptor with TokenGrammarLabel and NodeKind derived from node (suffix "").
Use when building PrattConfig.InfixOps so call sites pass only (tok, leftBP, rightBP, node).
*/
func (g *GrammarDefiner) InfixOp(tok LangSpecLexerTokenType, leftBP, rightBP int, node LangSpecParserNodeKind) rule.PrattInfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind] {
	return rule.PrattInfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
		Token:             tok,
		LeftBP:            leftBP,
		RightBP:           rightBP,
		NodeKind:          node,
		TokenGrammarLabel: LangSpecGrammarIDFromNode(node),
	}
}

func (g *GrammarDefiner) PrefixOp(tok LangSpecLexerTokenType, rightBP int, node LangSpecParserNodeKind) rule.PrattPrefixOp[LangSpecLexerTokenType, LangSpecParserNodeKind] {
	return rule.PrattPrefixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
		Token:             tok,
		RightBP:           rightBP,
		NodeKind:          node,
		TokenGrammarLabel: LangSpecGrammarIDFromNode(node),
	}
}

func (g *GrammarDefiner) PostfixOp(tok LangSpecLexerTokenType, leftBP int, node LangSpecParserNodeKind) rule.PrattPostfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind] {
	return rule.PrattPostfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
		Token:             tok,
		LeftBP:            leftBP,
		NodeKind:          node,
		TokenGrammarLabel: LangSpecGrammarIDFromNode(node),
	}
}

/*
PostfixRuleOp returns a Pratt postfix rule operator descriptor.
It binds a trigger token to an executable sub-rule (e.g., for composite bounds like {min,max}).
*/
func (g *GrammarDefiner) PostfixRuleOp(
	tok LangSpecLexerTokenType,
	leftBP int,
	node LangSpecParserNodeKind,
	r Rule,
) rule.PrattPostfixRuleOp[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind] {
	return rule.PrattPostfixRuleOp[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
		TriggerToken:      tok,
		LeftBP:            leftBP,
		NodeKind:          node,
		Rule:              r,
		TokenGrammarLabel: LangSpecGrammarIDFromNode(node),
	}
}

/*
block creates a standard delimited section:
Keyword -> { -> Body -> } -> [;]
Semicolon after the closing brace is optional (consumed if present). GrammarID is derived from node.
*/
func (g *GrammarDefiner) block(
	node LangSpecParserNodeKind,
	kwNode LangSpecParserNodeKind,
	kwTok LangSpecLexerTokenType,
	bodyRule Rule,
) Rule {
	grammarID := LangSpecGrammarIDFromNode(node)
	return g.rb.Rule.Sequence(
		grammarID,
		node,
		g.expectToken(kwNode, kwTok),
		g.rb.Rule.TransparentNest(
			LangSpecGrammarIDFromNodeWithSuffix(node, "BODY"),
			TokBraceOpen,
			TokBraceClose,
			bodyRule,
		),
		g.rb.Rule.Optional(g.expectVirtualInRule(node, TokSemicolon)),
	)
}

/*
blockByRule creates a standard delimited section:
keywordRule -> { -> Body -> } -> [;]
Semicolon after the closing brace is optional (consumed if present). GrammarID is derived from node.
*/
func (g *GrammarDefiner) blockByRule(
	node LangSpecParserNodeKind,
	kwRule Rule,
	bodyRule Rule,
) Rule {
	grammarID := LangSpecGrammarIDFromNode(node)
	return g.rb.Rule.Sequence(
		grammarID,
		node,
		kwRule,
		g.rb.Rule.TransparentNest(
			LangSpecGrammarIDFromNodeWithSuffix(node, "BODY"),
			TokBraceOpen,
			TokBraceClose,
			bodyRule,
		),
		g.rb.Rule.Optional(g.expectVirtualInRule(node, TokSemicolon)),
	)
}

// ----------------------------------------------------------- SequenceBuilder

/*
SequenceBuilder accumulates rules for a single sequence and builds a Rule via Build().
Returned by GrammarDefiner.sequence; chain methods append one rule and return the same builder.
*/
type SequenceBuilder struct {
	g         *GrammarDefiner
	grammarID syntaxa.GrammarLabel
	nodeKind  LangSpecParserNodeKind
	rules     []Rule
}

/*
expect appends a rule that expects the token and creates an LST node. GrammarID is derived from (node, suffix).
Use suffix "" for the default slot.
*/
func (s *SequenceBuilder) expect(node LangSpecParserNodeKind, suffix string, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.Expect(node, suffix, tok))
	return s
}

/*
expectToken appends a rule that expects the token and creates an LST node. Shorthand for expect(node, "", tok).
*/
func (s *SequenceBuilder) expectToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) *SequenceBuilder {
	return s.expect(node, "", tok)
}

/*
expectVirtual appends a rule that consumes the token without creating a node (virtual ID).
*/
func (s *SequenceBuilder) expectVirtual(v VirtualGrammarID, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.expectVirtual(v, tok))
	return s
}

/*
expectVirtualInRule appends a rule that consumes the token using the sequence's derived ID.
*/
func (s *SequenceBuilder) expectVirtualInRule(tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.expectVirtualInRule(s.nodeKind, tok))
	return s
}

/*
optionalToken appends an optional expectation of the token (derived GrammarID).
*/
func (s *SequenceBuilder) optionalToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.rb.Rule.Optional(s.g.expectToken(node, tok)))
	return s
}

/*
optionalRule appends an optional sub-rule.
*/
func (s *SequenceBuilder) optionalRule(rule Rule) *SequenceBuilder {
	s.rules = append(s.rules, s.g.rb.Rule.Optional(rule))
	return s
}

/*
requiredRule appends a required sub-rule; on FailureNoMatch the given message is reported.
*/
func (s *SequenceBuilder) requiredRule(rule Rule, msg string) *SequenceBuilder {
	s.rules = append(s.rules, s.g.rb.Rule.Required(rule, msg))
	return s
}

/* rule just appends a rule as-is. */
func (s *SequenceBuilder) rule(rule Rule) *SequenceBuilder {
	s.rules = append(s.rules, rule)
	return s
}

/*
build returns the Sequence rule with the accumulated rules. The builder must not be used after build.
*/
func (s *SequenceBuilder) build() Rule {
	return s.g.rb.Rule.Sequence(s.grammarID, s.nodeKind, s.rules...)
}
