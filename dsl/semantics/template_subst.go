package semantics

import (
	"syntaxa"

	. "langspec/dsl/spec"
)

const templateSubstMaxIterations = 64

/*
SubstituteTemplateParameterReferences returns a deep clone of root with each
NodeParseTemplateParameterReference replaced by bindings[name]. Expansions are
applied iteratively until no placeholders remain (or no progress), so nested
forwarded parameters and all sites in the tree are covered.

The previous post-order walk visited children before replacing a parameter node;
that ordering can leave a leading segment placeholder (e.g. $outNode : ( … ))
unexpanded in some trees, which then breaks compileParseSegment.

Unknown parameter names leave the reference node unchanged (validator should prevent this).
*/
func SubstituteTemplateParameterReferences(root *Node, bindings map[string]*Node) *Node {
	return substituteTemplateParameterReferencesIter(root, bindings)
}

func substituteTemplateParameterReferencesIter(root *Node, bindings map[string]*Node) *Node {
	if root == nil {
		return nil
	}
	out := syntaxa.CloneLSTSubtreeDetached(root)

	if repl, ok := substituteTemplateCallArgumentTokenParameter(out, bindings); ok {
		out = repl
	}

	for i := 0; i < templateSubstMaxIterations; i++ {
		if out.Kind() == NodeParseTemplateParameterReference {
			name := TemplateParamRefName(out)
			if name == "" {
				break
			}
			rep, ok := bindings[name]
			if !ok || rep == nil {
				break
			}
			out = substituteTemplateParameterReferencesIter(rep, bindings)
			continue
		}

		ref := out.FindFirstKind(NodeParseTemplateParameterReference)
		if ref == nil {
			break
		}
		name := TemplateParamRefName(ref)
		if name == "" {
			break
		}
		rep, ok := bindings[name]
		if !ok || rep == nil {
			break
		}
		expanded := substituteTemplateParameterReferencesIter(rep, bindings)
		if ref == out {
			out = expanded
			continue
		}
		if !syntaxa.ReplaceChildInTree(ref, expanded) {
			break
		}
	}

	for i := 0; i < templateSubstMaxIterations; i++ {
		progress := false
		_ = out.WalkPre(func(n *Node) (bool, bool) {
			if n == nil || n.Kind() != NodeParseTemplateCallArgument {
				return false, false
			}
			repl, ok := substituteTemplateCallArgumentTokenParameter(n, bindings)
			if !ok || n.Parent() == nil {
				return false, false
			}
			if syntaxa.ReplaceChildInTree(n, repl) {
				progress = true
			}
			return false, false
		})
		if !progress {
			break
		}
	}

	return out
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
	ed.AttachChild(out, substituteTemplateParameterReferencesIter(rep, bindings))
	return out, true
}
