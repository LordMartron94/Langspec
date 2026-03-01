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
GrammarDefinerCreate returns a new GrammarDefiner that uses the given RuleBuilder and
the package-level LangSpecGrammarIDForNode lookup for ExpectToken/ExpectOneOf.
*/
func GrammarDefinerCreate(rb *RuleBuilder) *GrammarDefiner {
	return &GrammarDefiner{rb: rb}
}

/*
ExpectToken resolves the GrammarID for node via the registry and returns a rule that
expects the single token and creates an AST node of that kind. Panics if the node has
no registered GrammarID.
*/
func (g *GrammarDefiner) ExpectToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	grammarID, ok := LangSpecGrammarIDForNode(node)
	if !ok {
		panic(fmt.Sprintf("dsl: no GrammarID registered for node %v", node))
	}
	return g.rb.Token.Expect(grammarID, node, tok)
}

/*
ExpectTokenWithGrammarID returns a rule that expects the token and creates an AST node,
using the given grammarID (no registry lookup). Use for header expectations or when the
node maps to multiple grammar IDs (e.g. NodeVersion for DSL vs Langspec version).
*/
func (g *GrammarDefiner) ExpectTokenWithGrammarID(grammarID syntaxa.GrammarID, node LangSpecParserNodeKind, tok LangSpecLexerTokenType) Rule {
	return g.rb.Token.Expect(grammarID, node, tok)
}

/*
ExpectVirtual returns a rule that consumes the token without creating an AST node.
Virtual expectations always pass GrammarID explicitly.
*/
func (g *GrammarDefiner) ExpectVirtual(id syntaxa.GrammarID, tok LangSpecLexerTokenType) Rule {
	return g.rb.Token.ExpectVirtual(id, tok)
}

/*
ExpectOneOf resolves the GrammarID for node via the registry and returns a rule that
expects one of the given tokens and creates an AST node of that kind. Panics if the node
has no registered GrammarID.
*/
func (g *GrammarDefiner) ExpectOneOf(node LangSpecParserNodeKind, tokens ...LangSpecLexerTokenType) Rule {
	grammarID, ok := LangSpecGrammarIDForNode(node)
	if !ok {
		panic(fmt.Sprintf("dsl: no GrammarID registered for node %v", node))
	}
	return g.rb.Token.ExpectOneOf(grammarID, node, tokens...)
}

/*
ExpectOneOfWithGrammarID returns a rule that expects one of the tokens and creates an
AST node, using the given grammarID. Use when the node maps to multiple grammar IDs.
*/
func (g *GrammarDefiner) ExpectOneOfWithGrammarID(grammarID syntaxa.GrammarID, node LangSpecParserNodeKind, tokens ...LangSpecLexerTokenType) Rule {
	return g.rb.Token.ExpectOneOf(grammarID, node, tokens...)
}

/*
Sequence starts a chainable sequence for the given grammar ID and node kind. Call
Build on the returned SequenceBuilder to produce the Rule.
*/
func (g *GrammarDefiner) Sequence(grammarID syntaxa.GrammarID, nodeKind LangSpecParserNodeKind) *SequenceBuilder {
	return &SequenceBuilder{
		g:         g,
		grammarID: grammarID,
		nodeKind:  nodeKind,
		rules:     make([]Rule, 0),
	}
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
ExpectToken appends a rule that expects the token and creates an AST node (registry lookup).
*/
func (s *SequenceBuilder) ExpectToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.ExpectToken(node, tok))
	return s
}

/*
ExpectVirtual appends a rule that consumes the token without creating a node.
*/
func (s *SequenceBuilder) ExpectVirtual(id syntaxa.GrammarID, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.ExpectVirtual(id, tok))
	return s
}

/*
OptionalToken appends an optional expectation of the token (registry lookup).
*/
func (s *SequenceBuilder) OptionalToken(node LangSpecParserNodeKind, tok LangSpecLexerTokenType) *SequenceBuilder {
	s.rules = append(s.rules, s.g.rb.Rule.Optional(s.g.ExpectToken(node, tok)))
	return s
}

/*
OptionalRule appends an optional sub-rule.
*/
func (s *SequenceBuilder) OptionalRule(rule Rule) *SequenceBuilder {
	s.rules = append(s.rules, s.g.rb.Rule.Optional(rule))
	return s
}

/*
RequiredRule appends a required sub-rule; on FailureNoMatch the given message is reported.
*/
func (s *SequenceBuilder) RequiredRule(rule Rule, msg string) *SequenceBuilder {
	s.rules = append(s.rules, s.g.rb.Rule.Required(rule, msg))
	return s
}

/*
Build returns the Sequence rule with the accumulated rules. The builder must not be used after Build.
*/
func (s *SequenceBuilder) Build() Rule {
	return s.g.rb.Rule.Sequence(s.grammarID, s.nodeKind, s.rules...)
}

// ----------------------------------------------------------- Header expectations

/*
LangSpecBuildHeaderRules builds a slice of rules from the flat header expectations.
Virtual expectations yield ExpectVirtual; single-token yields Expect with explicit
GrammarID and NodeKind; multiple tokens yield ExpectOneOf. Caller uses these rules
inside a Sequence for header content.
*/
func LangSpecBuildHeaderRules(g *GrammarDefiner, flat []DSLHeaderExpectation) []Rule {
	out := make([]Rule, 0, len(flat))
	for _, e := range flat {
		if e.Virtual {
			if len(e.Tokens) > 0 {
				out = append(out, g.ExpectVirtual(e.GrammarID, e.Tokens[0]))
			}
			continue
		}
		if len(e.Tokens) == 1 {
			out = append(out, g.ExpectTokenWithGrammarID(e.GrammarID, e.NodeKind, e.Tokens[0]))
		} else if len(e.Tokens) > 1 {
			out = append(out, g.ExpectOneOfWithGrammarID(e.GrammarID, e.NodeKind, e.Tokens...))
		}
	}
	return out
}
