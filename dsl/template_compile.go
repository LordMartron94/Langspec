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
	if arg == nil {
		panic("compiler error: empty template call argument")
	}
	if n := findTemplateArgumentTypedNode(arg, pty); n != nil {
		return syntaxa.CloneLSTSubtreeDetached(n)
	}
	argSource := findTemplateArgumentSourceNode(arg)
	if argSource == nil || len(argSource.Tokens()) == 0 {
		panic("compiler error: empty template call argument")
	}
	toks := argSource.Tokens()
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

func findTemplateArgumentTypedNode(arg *Node, pty semantics.TemplateParamType) *Node {
	if arg == nil {
		return nil
	}
	findKindWithTokens := func(kind dslspec.LangSpecParserNodeKind) *Node {
		return findFirstInSubtree(arg, func(n *Node) bool {
			return n != nil && n.Kind() == kind && len(n.Tokens()) > 0
		})
	}
	switch pty {
	case semantics.TemplateParamToken:
		if n := findKindWithTokens(dslspec.NodeParseTokenReference); n != nil {
			return n
		}
	case semantics.TemplateParamRule, semantics.TemplateParamNode:
		if n := findKindWithTokens(dslspec.NodeParseSymbolReference); n != nil {
			return n
		}
	case semantics.TemplateParamPair:
		if n := findKindWithTokens(dslspec.NodeParseNestPairRef); n != nil {
			return n
		}
	case semantics.TemplateParamPrattExpr:
		if n := findKindWithTokens(dslspec.NodeParseExpressionReference); n != nil {
			return n
		}
	}
	if n := findKindWithTokens(dslspec.NodeParseTemplateParameterReference); n != nil {
		return n
	}
	return nil
}

func findTemplateArgumentSourceNode(arg *Node) *Node {
	if arg == nil {
		return nil
	}
	if len(arg.Tokens()) > 0 {
		return arg
	}
	best := findFirstInSubtree(arg, func(n *Node) bool {
		if n == nil || len(n.Tokens()) == 0 {
			return false
		}
		switch n.Kind() {
		case dslspec.NodeParseTokenReference,
			dslspec.NodeParseExpressionReference,
			dslspec.NodeParseSymbolReference,
			dslspec.NodeParseTemplateParameterReference,
			dslspec.NodeParseNestPairRef:
			return true
		}
		return false
	})
	if best != nil {
		return best
	}
	firstWithTokens := findFirstInSubtree(arg, func(n *Node) bool {
		return n != nil && len(n.Tokens()) > 0
	})
	if firstWithTokens != nil {
		return firstWithTokens
	}
	return nil
}

func findFirstInSubtree(arg *Node, match func(*Node) bool) *Node {
	if arg == nil {
		return nil
	}
	if match(arg) {
		return arg
	}
	for _, ch := range arg.ChildrenUnsafe() {
		if out := findFirstInSubtree(ch, match); out != nil {
			return out
		}
	}
	for _, slotName := range arg.SlotNames() {
		if out := findFirstInSubtree(arg.Slot(slotName), match); out != nil {
			return out
		}
	}
	return nil
}

func compileTemplateCallSegment[TNodeKind ~uint32](ctx *parseCompileCtx[TNodeKind], segment *Node) syntaxa.ParserRule[TNodeKind] {
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
	substituted = normalizeSplitOutputMappingConcats(substituted)
	subCtx := *ctx
	subCtx.rootLevel = false
	compiled := compileParseExpression[TNodeKind](&subCtx, substituted)
	if g := compiled.GetGrammar(); g != nil && ctx.sourceMap != nil {
		ctx.sourceMap[g] = segment
	}
	return compiled
}

/*
normalizeSplitOutputMappingConcats fixes Pratt output where IDENT_MAPPING_OR_CALL is split into two
concat siblings: a leading-only segment (output name / parameter) and a following MAPPING_TAIL
segment. compileParseSegment expects a single segment [leading ref, tail…] (see spec IDENT_MAPPING_OR_CALL).
*/
func normalizeSplitOutputMappingConcats(root *Node) *Node {
	if root == nil {
		return nil
	}
	return normalizeSplitOutputMappingInTree(root)
}

func normalizeSplitOutputMappingInTree(n *Node) *Node {
	if n == nil {
		return nil
	}
	if n.Kind() != dslspec.NodeParseConcat {
		return n
	}
	raw := n.ChildrenUnsafe()
	kids := make([]*Node, 0, len(raw))
	for _, ch := range raw {
		if ch == nil {
			continue
		}
		kids = append(kids, normalizeSplitOutputMappingInTree(ch))
	}
	out := mergeAdjacentLeadTailSegments(kids)
	if len(out) == 1 {
		return syntaxa.CloneLSTSubtreeDetached(out[0])
	}
	ed := newScratchLSTEditor()
	concat := ed.NewNode(dslspec.NodeParseConcat)
	for _, ch := range out {
		if ch == nil {
			continue
		}
		ed.AttachChild(concat, syntaxa.CloneLSTSubtreeDetached(ch))
	}
	return concat
}

func mergeAdjacentLeadTailSegments(nodes []*Node) []*Node {
	if len(nodes) < 2 {
		return nodes
	}
	changed := true
	for changed {
		changed = false
		next := make([]*Node, 0, len(nodes))
		for i := 0; i < len(nodes); i++ {
			if i+1 < len(nodes) &&
				isLeadingOnlyOutputSegment(nodes[i]) &&
				isMappingTailOnlySegment(nodes[i+1]) {
				next = append(next, mergeLeadAndMappingTailSegments(nodes[i], nodes[i+1]))
				i++
				changed = true
				continue
			}
			next = append(next, nodes[i])
		}
		nodes = next
	}
	return nodes
}

func segmentLogicalChildCount(n *Node) int {
	if n == nil {
		return 0
	}
	c := 0
	for _, ch := range n.ChildrenUnsafe() {
		if ch != nil {
			c++
		}
	}
	for _, sn := range n.SlotNames() {
		if n.Slot(sn) != nil {
			c++
		}
	}
	return c
}

func firstLogicalChild(n *Node) *Node {
	if n == nil {
		return nil
	}
	for _, ch := range n.ChildrenUnsafe() {
		if ch != nil {
			return ch
		}
	}
	for _, sn := range n.SlotNames() {
		if sl := n.Slot(sn); sl != nil {
			return sl
		}
	}
	return nil
}

func isLeadingOnlyOutputSegment(n *Node) bool {
	if n == nil || n.Kind() != dslspec.NodeParseSegment {
		return false
	}
	// A leading output site must not already carry a mapping tail (paren group or token target).
	if n.FindFirstKind(dslspec.NodeParseGroup) != nil || n.FindFirstKind(dslspec.NodeParseTokenReference) != nil {
		return false
	}
	if segmentLogicalChildCount(n) != 1 {
		return false
	}
	lead := firstLogicalChild(n)
	if lead == nil {
		return false
	}
	switch lead.Kind() {
	case dslspec.NodeParseSymbolReference, dslspec.NodeParseNodeName, dslspec.NodeParseTemplateParameterReference:
		return true
	default:
		return false
	}
}

func isMappingTailOnlySegment(n *Node) bool {
	if n == nil || n.Kind() != dslspec.NodeParseSegment {
		return false
	}
	// MAPPING_TAIL segment has a colon + target (group or token) but no leading output ident.
	if n.FindFirstKind(dslspec.NodeParseSymbolReference) != nil {
		return false
	}
	if n.FindFirstKind(dslspec.NodeParseNodeName) != nil {
		return false
	}
	if n.FindFirstKind(dslspec.NodeParseTemplateParameterReference) != nil {
		return false
	}
	return n.FindFirstKind(dslspec.NodeParseGroup) != nil || n.FindFirstKind(dslspec.NodeParseTokenReference) != nil
}

func mergeLeadAndMappingTailSegments(leadSeg, tailSeg *Node) *Node {
	ed := newScratchLSTEditor()
	out := ed.NewNode(dslspec.NodeParseSegment)
	for _, ch := range leadSeg.ChildrenUnsafe() {
		if ch != nil {
			ed.AttachChild(out, syntaxa.CloneLSTSubtreeDetached(ch))
		}
	}
	for _, sn := range leadSeg.SlotNames() {
		if sl := leadSeg.Slot(sn); sl != nil {
			ed.SetSlot(out, sn, syntaxa.CloneLSTSubtreeDetached(sl))
		}
	}
	for _, ch := range tailSeg.ChildrenUnsafe() {
		if ch != nil {
			ed.AttachChild(out, syntaxa.CloneLSTSubtreeDetached(ch))
		}
	}
	for _, sn := range tailSeg.SlotNames() {
		if sl := tailSeg.Slot(sn); sl != nil {
			ed.SetSlot(out, sn, syntaxa.CloneLSTSubtreeDetached(sl))
		}
	}
	return out
}
