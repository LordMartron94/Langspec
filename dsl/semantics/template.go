package semantics

import (
	"strings"

	. "langspec/dsl/spec"
	"syntaxa"
)

// TemplateParamType is the type of a template formal parameter (PARSE signature).
type TemplateParamType uint8

const (
	TemplateParamToken TemplateParamType = iota + 1
	TemplateParamNode
	TemplateParamRule
	TemplateParamPair
	TemplateParamPrattExpr
)

// TemplateParam is one formal parameter in a template declaration.
type TemplateParam struct {
	Name string // normalized, without leading '$'
	Type TemplateParamType
}

/*
TemplateDecl records one `template` declaration in the PARSE section.
Node is the NodeParseTemplate root for diagnostics.
*/
type TemplateDecl struct {
	Node   *Node
	Name   string
	Params []TemplateParam
}

// NormalizeTemplateParamName strips a leading '$' from lexer parameter text.
func NormalizeTemplateParamName(raw string) string {
	s := strings.TrimSpace(raw)
	return strings.TrimPrefix(s, "$")
}

// TemplateParamRefName returns the normalized parameter name for a NodeParseTemplateParameterReference, or "".
func TemplateParamRefName(n *Node) string {
	if n == nil || n.Kind() != NodeParseTemplateParameterReference {
		return ""
	}
	toks := n.Tokens()
	if len(toks) == 0 {
		return ""
	}
	return NormalizeTemplateParamName(string(toks[0].Raw))
}

func templateParamTypeFromLexemes(toks []syntaxa.Lexeme) (TemplateParamType, bool) {
	if len(toks) == 0 {
		return 0, false
	}
	switch LangSpecLexerTokenType(toks[0].Token) {
	case TokTypeToken:
		return TemplateParamToken, true
	case TokTypeNode:
		return TemplateParamNode, true
	case TokTypeRule:
		return TemplateParamRule, true
	case TokTypePair:
		return TemplateParamPair, true
	case TokTypePrattExpr:
		return TemplateParamPrattExpr, true
	default:
		return 0, false
	}
}

func templateParamTypeFromDescendants(n *Node) (TemplateParamType, bool) {
	var walk func(*Node) (TemplateParamType, bool)
	walk = func(cur *Node) (TemplateParamType, bool) {
		if cur == nil {
			return 0, false
		}
		if ty, ok := templateParamTypeFromLexemes(cur.Tokens()); ok {
			return ty, true
		}
		for _, ch := range cur.ChildrenUnsafe() {
			if ty, ok := walk(ch); ok {
				return ty, true
			}
		}
		return 0, false
	}
	return walk(n)
}

// TemplateParamTypeFromNode reads NodeParseTemplateParameterType's type keyword (surface or virtual).
func TemplateParamTypeFromNode(n *Node) (TemplateParamType, bool) {
	if n == nil || n.Kind() != NodeParseTemplateParameterType {
		return 0, false
	}
	if ty, ok := templateParamTypeFromLexemes(n.Tokens()); ok {
		return ty, true
	}
	return templateParamTypeFromDescendants(n)
}

/*
SegmentHasExplicitTemplateInvocation is true for segments written as call TemplateName(Arg1, …).
Bare Identifier ( … ) is a parse grouping/mapping shape, not a template call.
*/
func SegmentHasExplicitTemplateInvocation(segment *Node) bool {
	if segment == nil || segment.Kind() != NodeParseSegment {
		return false
	}
	return segment.FindFirstKind(NodeParseTemplateCallKeyword) != nil &&
		segment.FindFirstKind(NodeParseTemplateCallArgs) != nil
}

// TemplateCallArgsNode returns the call-args nest for an explicit template call segment, or nil.
func TemplateCallArgsNode(segment *Node) *Node {
	if !SegmentHasExplicitTemplateInvocation(segment) {
		return nil
	}
	return segment.FindFirstKind(NodeParseTemplateCallArgs)
}

/*
TemplateCallCalleeRef returns the first symbol or parameter reference child after the call keyword
(meta-grammar order: call keyword, callee, args). Nil if the segment is not an explicit template call
or has no such child.
*/
func TemplateCallCalleeRef(segment *Node) *Node {
	if !SegmentHasExplicitTemplateInvocation(segment) {
		return nil
	}
	for _, ch := range segment.ChildrenUnsafe() {
		if ch == nil {
			continue
		}
		k := ch.Kind()
		if k == NodeParseTemplateCallKeyword {
			continue
		}
		if k == NodeParseSymbolReference || k == NodeParseTemplateParameterReference {
			return ch
		}
	}
	return nil
}

// SymbolReferenceIsExplicitTemplateCallCallee is true when ref is the callee identifier of call Name(…).
func SymbolReferenceIsExplicitTemplateCallCallee(ref *Node) bool {
	if ref == nil || ref.Kind() != NodeParseSymbolReference {
		return false
	}
	parent := ref.Parent()
	if parent == nil || parent.Kind() != NodeParseSegment {
		return false
	}
	return ref == TemplateCallCalleeRef(parent)
}

/*
ParseSegmentLeadingSymbolRefIsBareGrammarSite is true when a parse segment’s leading identifier
is in bare rule/pratt/token position: no output mapping or grouping tail, and not a call Name(…) segment.
*/
func ParseSegmentLeadingSymbolRefIsBareGrammarSite(parent *Node) bool {
	if parent == nil || parent.Kind() != NodeParseSegment {
		return false
	}
	if parent.FindFirstKind(NodeParseTokenReference) != nil || parent.FindFirstKind(NodeParseGroup) != nil {
		return false
	}
	if SegmentHasExplicitTemplateInvocation(parent) {
		return false
	}
	return true
}

/*
ExtractTemplateDeclaration parses a NodeParseTemplate node.
*/
func ExtractTemplateDeclaration(tplNode *Node) (decl TemplateDecl, ok bool) {
	if tplNode == nil || tplNode.Kind() != NodeParseTemplate {
		return decl, false
	}
	id := tplNode.FindFirstKind(NodeParseTemplateIdentifier)
	if id == nil {
		return decl, false
	}
	decl.Name = strings.TrimSpace(IdentifierValue(id))
	if decl.Name == "" {
		return decl, false
	}
	// Signature: meta-grammar uses TransparentNest for `( … )`, so there is often no
	// NodeParseTemplateSignature in the LST—parameter list hangs directly under the template root.
	var list *Node
	if sig := tplNode.FindFirstKind(NodeParseTemplateSignature); sig != nil {
		list = sig.FindFirstKind(NodeParseTemplateParameterList)
	}
	if list == nil {
		list = tplNode.FindFirstKind(NodeParseTemplateParameterList)
	}
	if list != nil {
		for _, p := range list.FindAllKind(NodeParseTemplateParameter) {
			pi := p.FindFirstKind(NodeParseTemplateParameterIdentifier)
			pt := p.FindFirstKind(NodeParseTemplateParameterType)
			if pi == nil || pt == nil {
				continue
			}
			pname := NormalizeTemplateParamName(IdentifierValue(pi))
			if pname == "" {
				continue
			}
			pty, okTy := TemplateParamTypeFromNode(pt)
			if !okTy {
				continue
			}
			decl.Params = append(decl.Params, TemplateParam{Name: pname, Type: pty})
		}
	}
	decl.Node = tplNode
	return decl, true
}

// GetTemplateBodyExpressionRoot returns the parse expression root inside NodeParseTemplateBody.
func GetTemplateBodyExpressionRoot(tplNode *Node) *Node {
	body := tplNode.FindFirstKind(NodeParseTemplateBody)
	if body == nil {
		return nil
	}
	return getTemplateBodyRootFromBodyNode(body)
}

/*
TemplateCallCalleeName returns the callee template name for a segment written as
call TemplateName(…). ok is false when the callee is not a concrete identifier (e.g. a parameter placeholder).
*/
func TemplateCallCalleeName(segment *Node) (name string, calleeRef *Node, ok bool) {
	if segment == nil || segment.Kind() != NodeParseSegment {
		return "", nil, false
	}
	base := TemplateCallCalleeRef(segment)
	if base == nil {
		return "", nil, false
	}
	if base.Kind() == NodeParseTemplateParameterReference {
		return "", base, false
	}
	return strings.TrimSpace(IdentifierValue(base)), base, true
}

/*
TemplateCallArgumentNodes returns ordered NodeParseTemplateCallArgument children under call args.
*/
func TemplateCallArgumentNodes(args *Node) []*Node {
	if args == nil || args.Kind() != NodeParseTemplateCallArgs {
		return nil
	}
	var out []*Node
	for _, ch := range args.ChildrenUnsafe() {
		if ch != nil && ch.Kind() == NodeParseTemplateCallArgument {
			out = append(out, ch)
		}
	}
	return out
}

func getTemplateBodyRootFromBodyNode(body *Node) *Node {
	if body == nil {
		return nil
	}
	for _, ch := range body.ChildrenUnsafe() {
		if ch == nil {
			continue
		}
		k := ch.Kind()
		if k == NodeParseAlternation || k == NodeParseConcat || k == NodeParseOptional ||
			k == NodeParseStar || k == NodeParsePlus || k == NodeParseSegment || k == NodeParseGroup ||
			k == NodeParseModifierPredict || k == NodeParseOpSuppress || k == NodeParseOpNest ||
			k == NodeRepetition || k == NodeParseExpressionReference || k == NodeParseTokenReference {
			return ch
		}
	}
	if chs := body.ChildrenUnsafe(); len(chs) > 0 {
		return chs[0]
	}
	return nil
}
