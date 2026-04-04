package dsl

import (
	"fmt"
	"langspec/dsl/semantics"
	dslspec "langspec/dsl/spec"
	"syntaxa"
)

func newScratchLSTEditor() *syntaxa.LSTEditor[dslspec.LangSpecParserNodeKind] {
	ed := syntaxa.LSTEditor[dslspec.LangSpecParserNodeKind]{}
	return &ed
}

func buildTemplateArgumentParseRoot(
	ed *syntaxa.LSTEditor[dslspec.LangSpecParserNodeKind],
	arg *Node,
	pty semantics.TemplateParamType,
) *Node {
	if arg == nil || len(arg.Tokens()) == 0 {
		panic("compiler error: empty template call argument")
	}
	toks := arg.Tokens()
	switch pty {
	case semantics.TemplateParamToken:
		n := ed.NewNode(dslspec.NodeParseTokenReference)
		ed.SetTokens(n, append([]syntaxa.Lexeme(nil), toks...))
		return n
	case semantics.TemplateParamRule, semantics.TemplateParamNode:
		n := ed.NewNode(dslspec.NodeParseSymbolReference)
		ed.SetTokens(n, append([]syntaxa.Lexeme(nil), toks...))
		return n
	case semantics.TemplateParamPair:
		n := ed.NewNode(dslspec.NodeParseNestPairRef)
		ed.SetTokens(n, append([]syntaxa.Lexeme(nil), toks...))
		return n
	case semantics.TemplateParamPrattExpr:
		n := ed.NewNode(dslspec.NodeParseExpressionReference)
		ed.SetTokens(n, append([]syntaxa.Lexeme(nil), toks...))
		return n
	default:
		panic(fmt.Errorf("compiler error: unknown template parameter type %v", pty))
	}
}

func compileTemplateCallSegment(ctx *parseCompileCtx, segment *Node) CompiledRule {
	callArgs := segment.FindFirstKind(dslspec.NodeParseTemplateCallArgs)
	if callArgs == nil {
		panic("compiler error: compileTemplateCallSegment without call args")
	}
	name, _, ok := semantics.TemplateCallCalleeName(segment)
	if !ok {
		panic("compiler error: template call callee is not a resolved identifier")
	}
	decl := ctx.env.Templates[name]
	if decl == nil {
		panic(fmt.Errorf("compiler error: unknown template %q (validator should have caught this)", name))
	}
	body := semantics.GetTemplateBodyExpressionRoot(decl.Node)
	if body == nil {
		panic(fmt.Errorf("compiler error: template %q has no body", name))
	}
	argNodes := semantics.TemplateCallArgumentNodes(callArgs)
	if len(argNodes) != len(decl.Params) {
		panic("compiler error: template argument count mismatch")
	}
	ed := newScratchLSTEditor()
	bindings := make(map[string]*Node, len(decl.Params))
	for i, p := range decl.Params {
		bindings[p.Name] = buildTemplateArgumentParseRoot(ed, argNodes[i], p.Type)
	}
	substituted := semantics.SubstituteTemplateParameterReferences(body, bindings)
	subCtx := *ctx
	subCtx.rootLevel = false
	compiled := compileParseExpression(&subCtx, substituted)
	if g := compiled.GetGrammar(); g != nil && ctx.sourceMap != nil {
		ctx.sourceMap[g] = segment
	}
	return compiled
}
