package semantics

import (
	"fmt"
	"strings"

	. "langspec/dsl/spec"
)

/*
walkDeclaredOutputKindNames records parse output node kind names from NodeParseNodeName and from
NodeParseSymbolReference in bare-grammar segments inside a parse expression subtree.
*/
func walkDeclaredOutputKindNames(body *Node, out map[string]struct{}) {
	if body == nil {
		return
	}
	body.WalkPre(func(n *Node) (bool, bool) {
		if n == nil {
			return false, false
		}
		switch n.Kind() {
		case NodeParseNodeName:
			if s := strings.TrimSpace(NodeSingleTokenContent(n)); s != "" {
				out[s] = struct{}{}
			}
		case NodeParseSymbolReference:
			if !ParseSegmentLeadingSymbolRefIsBareGrammarSite(n.Parent()) {
				return false, false
			}
			if s := RefName(n); s != "" {
				out[s] = struct{}{}
			}
		}
		return false, false
	})
}

/*
collectDeclaredParseOutputNodeKinds gathers node kind strings from parse rule headers
(NodeParseNodeName) and from mapping / emit sites (symbol references and node names in segments).
*/
func collectDeclaredParseOutputNodeKinds(root *Node) map[string]struct{} {
	out := make(map[string]struct{})
	parse := root.FindFirstKind(NodeParseSection)
	if parse == nil {
		return out
	}
	for _, rule := range parse.FindAllKind(NodeParseRule) {
		if nn := rule.FindFirstKind(NodeParseNodeName); nn != nil {
			if s := strings.TrimSpace(NodeSingleTokenContent(nn)); s != "" {
				out[s] = struct{}{}
			}
		}
		if body := rule.FindFirstKind(NodeParseRuleBody); body != nil {
			walkDeclaredOutputKindNames(body, out)
		}
	}
	for _, tpl := range parse.FindAllKind(NodeParseTemplate) {
		bodyRoot := GetTemplateBodyExpressionRoot(tpl)
		if bodyRoot == nil {
			continue
		}
		walkDeclaredOutputKindNames(bodyRoot, out)
	}
	return out
}

func enclosingParseTemplate(n *Node) *Node {
	for cur := n; cur != nil; cur = cur.Parent() {
		if cur.Kind() == NodeParseTemplate {
			return cur
		}
	}
	return nil
}

/*
validateTemplateParameterScoping reports NodeParseTemplateParameterReference nodes that are not
inside a template body, and unknown parameter names inside a template body.
*/
func validateTemplateParameterScoping(ctx *ValidationCtx, env *SemanticEnv) {
	ctx.RootNode.WalkPre(func(n *Node) (bool, bool) {
		if n == nil || n.Kind() != NodeParseTemplateParameterReference {
			return false, false
		}
		tpl := enclosingParseTemplate(n)
		if tpl == nil {
			ctx.ReportError(VALIDATION_TEMPLATE_PARAM_OUTSIDE.String(),
				"template parameter reference is only valid inside a template body", n)
			return false, false
		}
		body := tpl.FindFirstKind(NodeParseTemplateBody)
		if body == nil || !nodeIsUnder(n, body) {
			ctx.ReportError(VALIDATION_TEMPLATE_PARAM_OUTSIDE.String(),
				"template parameter reference is only valid inside a template body", n)
			return false, false
		}
		pname := TemplateParamRefName(n)
		if pname == "" {
			return false, false
		}
		decl, ok := ExtractTemplateDeclaration(tpl)
		if !ok {
			return false, false
		}
		allowedNames := make(map[string]struct{}, len(decl.Params))
		for _, p := range decl.Params {
			allowedNames[p.Name] = struct{}{}
		}
		if _, ok := allowedNames[pname]; !ok {
			ctx.ReportError(VALIDATION_TEMPLATE_UNKNOWN_PARAM.String(),
				fmt.Sprintf("unknown template parameter '%s'", pname), n)
		}
		return false, false
	})
}

func nodeIsUnder(n, ancestor *Node) bool {
	for cur := n; cur != nil; cur = cur.Parent() {
		if cur == ancestor {
			return true
		}
	}
	return false
}

/*
ExpandedTemplateBodyRootForCallSegment returns the parse expression root of the callee template’s body
when segment is call Name(…), Name is an identifier, and env declares that template with a non-empty body.
*/
func ExpandedTemplateBodyRootForCallSegment(segment *Node, env *SemanticEnv) (bodyRoot *Node, callee string, ok bool) {
	if segment == nil || env == nil {
		return nil, "", false
	}
	if !SegmentHasExplicitTemplateInvocation(segment) {
		return nil, "", false
	}
	calleeName, _, identOk := TemplateCallCalleeName(segment)
	if !identOk {
		return nil, calleeName, false
	}
	decl := env.Templates[calleeName]
	if decl == nil {
		return nil, calleeName, false
	}
	root := GetTemplateBodyExpressionRoot(decl.Node)
	if root == nil {
		return nil, calleeName, false
	}
	return root, calleeName, true
}

func validateTemplateCallSites(ctx *ValidationCtx, env *SemanticEnv, nodeKinds map[string]struct{}) {
	graph := make(map[string][]string)

	var collectEdges func(tplName string, bodyRoot *Node)
	collectEdges = func(tplName string, bodyRoot *Node) {
		if bodyRoot == nil {
			return
		}
		bodyRoot.WalkPre(func(n *Node) (bool, bool) {
			if n == nil || n.Kind() != NodeParseSegment {
				return false, false
			}
			if !SegmentHasExplicitTemplateInvocation(n) {
				return false, false
			}
			_, callee, expandedOk := ExpandedTemplateBodyRootForCallSegment(n, env)
			if expandedOk {
				graph[tplName] = append(graph[tplName], callee)
			} else {
				calleeName, _, identOk := TemplateCallCalleeName(n)
				if !identOk {
					ctx.ReportError(VALIDATION_TEMPLATE_CALLEE_NOT_IDENT.String(),
						"template call callee must be an identifier (not a parameter placeholder)", n)
				} else if _, exists := env.Templates[calleeName]; exists {
					graph[tplName] = append(graph[tplName], calleeName)
				}
			}
			return false, false
		})
	}

	for name, decl := range env.Templates {
		root := GetTemplateBodyExpressionRoot(decl.Node)
		collectEdges(name, root)
	}

	if c := templateExpansionCyclePath(graph); len(c) > 0 {
		reportNode := ctx.RootNode
		if decl := env.Templates[c[0]]; decl != nil && decl.Node != nil {
			reportNode = decl.Node
		}
		ctx.ReportError(VALIDATION_TEMPLATE_CYCLE.String(),
			fmt.Sprintf("template expansion cycle: %s", formatCycle(c)), reportNode)
	}

	ctx.RootNode.WalkPre(func(n *Node) (bool, bool) {
		if n == nil || n.Kind() != NodeParseSegment {
			return false, false
		}
		if !SegmentHasExplicitTemplateInvocation(n) {
			return false, false
		}
		callArgs := TemplateCallArgsNode(n)
		callee, calleeNode, ok := TemplateCallCalleeName(n)
		if !ok {
			return false, false
		}
		decl, exists := env.Templates[callee]
		if !exists {
			ctx.ReportError(VALIDATION_TEMPLATE_UNRESOLVED_CALLEE.String(),
				fmt.Sprintf("call to unknown template '%s'", callee), calleeNode)
			return false, false
		}
		args := TemplateCallArgumentNodes(callArgs)
		if len(args) != len(decl.Params) {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARITY.String(),
				fmt.Sprintf("template '%s' expects %d argument(s), got %d", callee, len(decl.Params), len(args)),
				callArgs)
			return false, false
		}
		for i, p := range decl.Params {
			validateTemplateCallArg(ctx, env, nodeKinds, p.Type, args[i])
		}
		return false, false
	})
}

func validateTemplateCallArg(ctx *ValidationCtx, env *SemanticEnv, nodeKinds map[string]struct{}, pty TemplateParamType, arg *Node) {
	if arg == nil {
		return
	}
	toks := arg.Tokens()
	if len(toks) == 0 {
		ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(), "template call argument is empty", arg)
		return
	}
	raw := toks[0].Raw
	tt := LangSpecLexerTokenType(toks[0].Token)

	switch pty {
	case TemplateParamToken:
		if tt != TokIdentifier {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				"template argument for token parameter must be a lexer token identifier", arg)
			return
		}
		name := strings.TrimSpace(string(raw))
		if env.Tokens[name] == nil {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				fmt.Sprintf("token parameter argument '%s' is not a declared lexer token", name), arg)
		}
	case TemplateParamNode:
		if tt != TokIdentifier {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				"template argument for node parameter must be an identifier", arg)
			return
		}
		name := strings.TrimSpace(string(raw))
		if _, ok := nodeKinds[name]; !ok {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				fmt.Sprintf("node parameter argument '%s' is not a declared output node kind", name), arg)
		}
	case TemplateParamRule:
		if tt != TokIdentifier {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				"template argument for rule parameter must be a parse rule identifier", arg)
			return
		}
		name := strings.TrimSpace(string(raw))
		if env.Rules[name] == nil {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				fmt.Sprintf("rule parameter argument '%s' is not a declared parse rule", name), arg)
		}
	case TemplateParamPair:
		if tt != TokPairReference {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				"template argument for pair parameter must be a pair reference (@Name)", arg)
			return
		}
		pn := PairNameFromTokPairReferenceRaw(raw)
		if pn == "" || env.Pairs[pn] == nil {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				"pair parameter argument does not name a declared pair", arg)
		}
	case TemplateParamPrattExpr:
		if tt != TokIdentifier {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				"template argument for pratt parameter must be a pratt expression identifier", arg)
			return
		}
		name := strings.TrimSpace(string(raw))
		if env.Pratt[name] == nil {
			ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARG_TYPE.String(),
				fmt.Sprintf("pratt parameter argument '%s' is not a declared pratt expression", name), arg)
		}
	}
}

/*
templateExpansionCyclePath returns one directed cycle path in the template call graph, or nil if acyclic.
States: 0 unvisited, 1 on recursion stack, 2 finished.
*/
func templateExpansionCyclePath(graph map[string][]string) []string {
	state := make(map[string]uint8)
	var pathStack []string
	var cycle []string

	var visit func(v string) bool
	visit = func(v string) bool {
		switch state[v] {
		case 1:
			for i := len(pathStack) - 1; i >= 0; i-- {
				if pathStack[i] == v {
					cycle = append([]string{}, pathStack[i:]...)
					cycle = append(cycle, v)
					return true
				}
			}
			return false
		case 2:
			return false
		}
		state[v] = 1
		pathStack = append(pathStack, v)
		for _, w := range graph[v] {
			if visit(w) {
				return true
			}
		}
		pathStack = pathStack[:len(pathStack)-1]
		state[v] = 2
		return false
	}

	for v := range graph {
		if state[v] == 0 && visit(v) {
			return cycle
		}
	}
	return nil
}

/*
staticRuleAndPrattRefsFromTemplateBody collects parse rule and pratt names referenced from a
template body for reachability (ignoring parameter placeholders and dynamic callee names).
*/
func staticRuleAndPrattRefsFromTemplateBody(bodyRoot *Node, env *SemanticEnv, visiting map[string]bool) []string {
	if bodyRoot == nil {
		return nil
	}
	var refs []string
	bodyRoot.WalkPre(func(n *Node) (bool, bool) {
		if n == nil {
			return false, false
		}
		if n.Kind() == NodeParseSegment {
			if SegmentHasExplicitTemplateInvocation(n) {
				if inner, callee, ok := ExpandedTemplateBodyRootForCallSegment(n, env); ok {
					if visiting[callee] {
						return false, false
					}
					visiting[callee] = true
					refs = append(refs, staticRuleAndPrattRefsFromTemplateBody(inner, env, visiting)...)
					delete(visiting, callee)
				}
				return false, false
			}
		}
		if n.Kind() == NodeParseSymbolReference {
			if SymbolReferenceIsExplicitTemplateCallCallee(n) {
				return false, false
			}
			name := RefName(n)
			if name == "" {
				return false, false
			}
			if env.Rules[name] != nil {
				refs = append(refs, name)
			} else if env.Pratt[name] != nil {
				refs = append(refs, name)
			}
		}
		if n.Kind() == NodeParseExpressionReference {
			name := RefName(n)
			if name == "" {
				return false, false
			}
			if env.Rules[name] != nil || env.Pratt[name] != nil {
				refs = append(refs, name)
			}
		}
		return false, false
	})
	return refs
}
