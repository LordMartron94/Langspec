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
