package semantics

import (
	"syntaxa"

	. "langspec/dsl/spec"
)

/*
SubstituteTemplateParameterReferences returns a deep clone of root with each
NodeParseTemplateParameterReference replaced by a deep clone of bindings[name].
Unknown parameter names leave the reference node unchanged (validator should prevent this).
*/
func SubstituteTemplateParameterReferences(root *Node, bindings map[string]*Node) *Node {
	if root == nil {
		return nil
	}
	root = syntaxa.CloneLSTSubtreeDetached(root)
	if root.Kind() == NodeParseTemplateParameterReference {
		name := TemplateParamRefName(root)
		if rep, ok := bindings[name]; ok && rep != nil {
			return syntaxa.CloneLSTSubtreeDetached(rep)
		}
		return root
	}
	if replacement, ok := substituteTemplateCallArgumentTokenParameter(root, bindings); ok {
		return replacement
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		for _, c := range n.ChildrenUnsafe() {
			walk(c)
		}
		for _, slotName := range n.SlotNames() {
			walk(n.Slot(slotName))
		}
		if n.Kind() != NodeParseTemplateParameterReference {
			if replacement, ok := substituteTemplateCallArgumentTokenParameter(n, bindings); ok {
				if n.Parent() == nil {
					return
				}
				syntaxa.ReplaceChildInTree(n, replacement)
			}
			return
		}
		if n.Parent() == nil {
			return
		}
		name := TemplateParamRefName(n)
		rep, ok := bindings[name]
		if !ok || rep == nil {
			return
		}
		newRep := syntaxa.CloneLSTSubtreeDetached(rep)
		syntaxa.ReplaceChildInTree(n, newRep)
	}
	walk(root)
	return root
}

func substituteTemplateCallArgumentTokenParameter(n *Node, bindings map[string]*Node) (*Node, bool) {
	if n == nil || n.Kind() != NodeParseTemplateCallArgument {
		return nil, false
	}
	toks := n.Tokens()
	if len(toks) != 1 || LangSpecLexerTokenType(toks[0].Token) != TokParameter {
		return nil, false
	}
	name := NormalizeTemplateParamName(string(toks[0].Raw))
	rep, ok := bindings[name]
	if !ok || rep == nil {
		return nil, false
	}
	ed := syntaxa.LSTEditor[LangSpecParserNodeKind]{}
	out := ed.NewNode(NodeParseTemplateCallArgument)
	ed.AttachChild(out, syntaxa.CloneLSTSubtreeDetached(rep))
	return out, true
}
