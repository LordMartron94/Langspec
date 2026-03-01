package dsl

import (
	"fmt"
	"syntaxa"
)

/*
GrammarDefiner wraps RuleBuilder and the langspec NodeKind→GrammarID registry so grammar
definitions can pass only node and token (or virtual GrammarID + token) instead of triplets.
Syntaxa remains generic; this layer is langspec-specific.
*/

type GrammarDefiner struct {
	rb *RuleBuilder
}

/*
grammarDefinerCreate returns a new GrammarDefiner that uses the given RuleBuilder and
the package-level LangSpecGrammarIDForNode lookup for ExpectToken/ExpectOneOf.
*/
func grammarDefinerCreate(rb *RuleBuilder) *GrammarDefiner {
	return &GrammarDefiner{rb: rb}
}

/*
expectToken resolves the GrammarID for node via the registry and returns a rule that
expects the single token and creates an AST node of that kind. Panics if the node has
no registered GrammarID.
*/
func (g *GrammarDefiner) expectToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	grammarID, ok := langSpecGrammarIDForNode(node)
	if !ok {
		panic(fmt.Sprintf("dsl: no GrammarID registered for node %v", node))
	}
	return g.rb.Token.Expect(grammarID, node, tok)
}

/*
expectTokenWithGrammarID returns a rule that expects the token and creates an AST node,
using the given grammarID (no registry lookup). Use for header expectations or when the
node maps to multiple grammar IDs (e.g. NodeVersion for DSL vs Langspec version).
*/
func (g *GrammarDefiner) expectTokenWithGrammarID(grammarID syntaxa.GrammarID, node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	return g.rb.Token.Expect(grammarID, node, tok)
}

/*
expectVirtual returns a rule that consumes the token without creating an AST node.
Virtual expectations always pass GrammarID explicitly.
*/
func (g *GrammarDefiner) expectVirtual(id syntaxa.GrammarID, tok LangSpecLexerTokenType) Rule {
	return g.rb.Token.ExpectVirtual(id, tok)
}

/*
expectOneOf resolves the GrammarID for node via the registry and returns a rule that
expects one of the given tokens and creates an AST node of that kind. Panics if the node
has no registered GrammarID.
*/
func (g *GrammarDefiner) expectOneOf(node LangSpecParserNodeKind, tokens ...LangSpecLexerTokenType) Rule {
	grammarID, ok := langSpecGrammarIDForNode(node)
	if !ok {
		panic(fmt.Sprintf("dsl: no GrammarID registered for node %v", node))
	}
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
sequence starts a chainable sequence for the given grammar ID and node kind. Call
Build on the returned SequenceBuilder to produce the Rule.
*/
func (g *GrammarDefiner) sequence(grammarID syntaxa.GrammarID, nodeKind LangSpecParserNodeKind) *SequenceBuilder {
	return &SequenceBuilder{
		g:         g,
		grammarID: grammarID,
		nodeKind:  nodeKind,
		rules:     make([]Rule, 0),
	}
}

/*
Block creates a standard delimited section:
Keyword -> { -> Body -> } -> ;
The bodyRule is typically a TransparentNest or a list of sub-rules.
*/
func (g *GrammarDefiner) block(
	grammarID syntaxa.GrammarID,
	node LangSpecParserNodeKind,
	kwNode LangSpecParserNodeKind,
	kwTok LangSpecLexerTokenType,
	bodyRule Rule,
) Rule {
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
		g.expectVirtual(grammarID, TokSemicolon),
	)
}

// ----------------------------------------------------------- SequenceBuilder

/*
SequenceBuilder accumulates rules for a single sequence and builds a Rule via Build().
Returned by GrammarDefiner.Sequence; chain methods append one rule and return the same builder.
*/
type SequenceBuilder struct {
	g         *GrammarDefiner
	grammarID syntaxa.GrammarID
	nodeKind  LangSpecParserNodeKind
	rules     []Rule
}

/*
expectToken appends a rule that expects the token and creates an AST node (registry lookup).
*/
func (s *SequenceBuilder) expectToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.expectToken(node, tok))
	return s
}

/*
expectVirtual appends a rule that consumes the token without creating a node.
*/
func (s *SequenceBuilder) expectVirtual(id syntaxa.GrammarID, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.expectVirtual(id, tok))
	return s
}

/*
optionalToken appends an optional expectation of the token (registry lookup).
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
langSpecBuildHeaderRules builds a slice of rules from the flat header expectations.
Virtual expectations yield ExpectVirtual; single-token yields Expect with explicit
GrammarID and NodeKind; multiple tokens yield ExpectOneOf. Caller uses these rules
inside a Sequence for header content.
*/
func langSpecBuildHeaderRules(g *GrammarDefiner, flat []DSLHeaderExpectation) []Rule {
	out := make([]Rule, 0, len(flat))
	for _, e := range flat {
		if e.Virtual {
			if len(e.Tokens) > 0 {
				out = append(out, g.expectVirtual(e.GrammarID, e.Tokens[0]))
			}
			continue
		}
		if len(e.Tokens) == 1 {
			out = append(out, g.expectTokenWithGrammarID(e.GrammarID, e.NodeKind, e.Tokens[0]))
		} else if len(e.Tokens) > 1 {
			out = append(out, g.expectOneOfWithGrammarID(e.GrammarID, e.NodeKind, e.Tokens...))
		}
	}
	return out
}
