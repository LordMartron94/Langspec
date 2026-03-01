package dsl

import (
	"syntaxa"
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
expectToken returns a rule that expects the token and creates an AST node of that kind.
GrammarID is derived from the node via LangSpecGrammarIDFromNode(node, "").
*/
func (g *GrammarDefiner) expectToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	grammarID := LangSpecGrammarIDFromNode(node, "")
	return g.rb.Token.Expect(grammarID, node, tok)
}

/*
expectTokenWithGrammarID returns a rule that expects the token and creates an AST node,
using the given grammarID. Use for header expectations when the node maps to multiple
grammar IDs (e.g. NodeVersion for DSL vs Langspec version) via GrammarIDOverride.
*/
func (g *GrammarDefiner) expectTokenWithGrammarID(grammarID syntaxa.GrammarID, node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	return g.rb.Token.Expect(grammarID, node, tok)
}

/*
expectVirtual returns a rule that consumes the token without creating an AST node.
Uses VirtualGrammarIDToGrammarID to resolve the virtual ID to syntaxa.GrammarID.
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
	return g.rb.Token.ExpectVirtual(LangSpecGrammarIDFromNode(node, ""), tok)
}

/*
expectOneOf returns a rule that expects one of the given tokens and creates an AST node
of that kind. GrammarID is derived from the node via LangSpecGrammarIDFromNode(node, "").
*/
func (g *GrammarDefiner) expectOneOf(node LangSpecParserNodeKind, tokens ...LangSpecLexerTokenType) Rule {
	grammarID := LangSpecGrammarIDFromNode(node, "")
	return g.rb.Token.ExpectOneOf(grammarID, node, tokens...)
}

/*
expectOneOfWithGrammarID returns a rule that expects one of the tokens and creates an
AST node, using the given grammarID. Use when the node maps to multiple grammar IDs.
*/
func (g *GrammarDefiner) expectOneOfWithGrammarID(grammarID syntaxa.GrammarID, node LangSpecParserNodeKind, tokens ...LangSpecLexerTokenType) Rule {
	return g.rb.Token.ExpectOneOf(grammarID, node, tokens...)
}

/*
expectPair returns a rule that expects two tokens in sequence and creates a single AST node
with both lexemes attached. GrammarID is derived from the node via LangSpecGrammarIDFromNode(node, "").
*/
func (g *GrammarDefiner) expectPair(node LangSpecParserNodeKind, firstToken, secondToken LangSpecLexerTokenType) Rule {
	grammarID := LangSpecGrammarIDFromNode(node, "")
	return g.rb.Token.ExpectPair(grammarID, node, firstToken, secondToken)
}

/*
sequence starts a chainable sequence for the given node kind. GrammarID is
LangSpecGrammarIDFromNode(nodeKind, suffix). Call Build on the returned SequenceBuilder.
*/
func (g *GrammarDefiner) sequence(nodeKind LangSpecParserNodeKind, suffix string) *SequenceBuilder {
	grammarID := LangSpecGrammarIDFromNode(nodeKind, suffix)
	return &SequenceBuilder{
		g:         g,
		grammarID: grammarID,
		nodeKind:  nodeKind,
		rules:     make([]Rule, 0),
	}
}

/*
block creates a standard delimited section:
Keyword -> { -> Body -> } -> ;
GrammarID is derived from node. The bodyRule is typically a TransparentNest or a list of sub-rules.
*/
func (g *GrammarDefiner) block(
	node LangSpecParserNodeKind,
	kwNode LangSpecParserNodeKind,
	kwTok LangSpecLexerTokenType,
	bodyRule Rule,
) Rule {
	grammarID := LangSpecGrammarIDFromNode(node, "")
	return g.rb.Rule.Sequence(
		grammarID,
		node,
		g.expectToken(kwNode, kwTok),
		g.rb.Rule.TransparentNest(
			grammarID,
			TokBraceOpen,
			TokBraceClose,
			bodyRule,
		),
		g.expectVirtualInRule(node, TokSemicolon),
	)
}

// ----------------------------------------------------------- SequenceBuilder

/*
SequenceBuilder accumulates rules for a single sequence and builds a Rule via Build().
Returned by GrammarDefiner.sequence; chain methods append one rule and return the same builder.
*/
type SequenceBuilder struct {
	g         *GrammarDefiner
	grammarID syntaxa.GrammarID
	nodeKind  LangSpecParserNodeKind
	rules     []Rule
}

/*
expectToken appends a rule that expects the token and creates an AST node (derived GrammarID).
*/
func (s *SequenceBuilder) expectToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.expectToken(node, tok))
	return s
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

// ----------------------------------------------------------- Header expectations

/*
langSpecExpectationGrammarID returns the syntaxa.GrammarID for the given header expectation.
Virtual expectations use VirtualID; node-backed use GrammarIDOverride if set, else
LangSpecGrammarIDFromNode(NodeKind, Suffix).
*/
func langSpecExpectationGrammarID(e DSLHeaderExpectation) syntaxa.GrammarID {
	if e.Virtual {
		return VirtualGrammarIDToGrammarID(e.VirtualID)
	}
	if e.GrammarIDOverride != "" {
		return syntaxa.GrammarID(e.GrammarIDOverride)
	}
	return LangSpecGrammarIDFromNode(e.NodeKind, e.Suffix)
}

/*
langSpecBuildHeaderRules builds a slice of rules from the flat header expectations.
Resolves GrammarID from each expectation via langSpecExpectationGrammarID. Virtual
expectations use expectVirtual; node-backed use expectToken/expectOneOf with resolved ID.
*/
func langSpecBuildHeaderRules(g *GrammarDefiner, flat []DSLHeaderExpectation) []Rule {
	out := make([]Rule, 0, len(flat))
	for _, e := range flat {
		id := langSpecExpectationGrammarID(e)
		if e.Virtual {
			if len(e.Tokens) > 0 {
				out = append(out, g.expectVirtual(e.VirtualID, e.Tokens[0]))
			}
			continue
		}
		if len(e.Tokens) == 1 {
			out = append(out, g.expectTokenWithGrammarID(id, e.NodeKind, e.Tokens[0]))
		} else if len(e.Tokens) > 1 {
			out = append(out, g.expectOneOfWithGrammarID(id, e.NodeKind, e.Tokens...))
		}
	}
	return out
}
