package semantics

import (
	"sort"
	"strconv"
	"strings"

	. "langspec/dsl/spec"
	dslspec "langspec/dsl/spec"
)

const compiledTokenIDStart = uint32(2)

/*
CompiledSymbolTable holds deterministic uint32 IDs for compiled target-language symbols.
Token IDs start at 2 to avoid lexarch reserved token kinds (0=ERROR, 1=EOF).
Role and node IDs start at 1. ID 0 is reserved as invalid.
*/
type CompiledSymbolTable struct {
	tokenNameByID []string
	roleNameByID  []string
	nodeNameByID  []string

	tokenIDByName map[string]uint32
	roleIDByName  map[string]uint32
	nodeIDByName  map[string]uint32
}

/*
CompiledSymbolTableBuild assigns sorted stable IDs. Each input slice must be sorted
and deduplicated; use CollectCompiledSymbolStrings.
*/
func CompiledSymbolTableBuild(tokens, roles, nodeKinds []string) *CompiledSymbolTable {
	t := &CompiledSymbolTable{
		tokenIDByName: make(map[string]uint32, len(tokens)),
		roleIDByName:  make(map[string]uint32, len(roles)),
		nodeIDByName:  make(map[string]uint32, len(nodeKinds)),
	}
	t.tokenNameByID = make([]string, len(tokens))
	for i, name := range tokens {
		id := compiledTokenIDStart + uint32(i)
		t.tokenNameByID[i] = name
		t.tokenIDByName[name] = id
	}
	t.roleNameByID = make([]string, len(roles))
	for i, name := range roles {
		id := uint32(i + 1)
		t.roleNameByID[i] = name
		t.roleIDByName[name] = id
	}
	t.nodeNameByID = make([]string, len(nodeKinds))
	for i, name := range nodeKinds {
		id := uint32(i + 1)
		t.nodeNameByID[i] = name
		t.nodeIDByName[name] = id
	}
	return t
}

func (t *CompiledSymbolTable) TokenID(name string) uint32 {
	if t == nil {
		return 0
	}
	return t.tokenIDByName[name]
}

func (t *CompiledSymbolTable) TokenName(id uint32) string {
	if t == nil || id < compiledTokenIDStart || int(id-compiledTokenIDStart+1) > len(t.tokenNameByID) {
		return "Token(" + strconv.FormatUint(uint64(id), 10) + ")"
	}
	return t.tokenNameByID[id-compiledTokenIDStart]
}

func (t *CompiledSymbolTable) RoleID(name string) uint32 {
	if t == nil {
		return 0
	}
	return t.roleIDByName[name]
}

func (t *CompiledSymbolTable) RoleName(id uint32) string {
	if t == nil || id == 0 || int(id) > len(t.roleNameByID) {
		return "Role(" + strconv.FormatUint(uint64(id), 10) + ")"
	}
	return t.roleNameByID[id-1]
}

func (t *CompiledSymbolTable) NodeKindID(name string) uint32 {
	if t == nil {
		return 0
	}
	return t.nodeIDByName[name]
}

func (t *CompiledSymbolTable) NodeKindName(id uint32) string {
	if t == nil || id == 0 || int(id) > len(t.nodeNameByID) {
		return "NodeKind(" + strconv.FormatUint(uint64(id), 10) + ")"
	}
	return t.nodeNameByID[id-1]
}

/* TokenNames returns a copy of token names in ID order (for codegen). */
func (t *CompiledSymbolTable) TokenNames() []string {
	if t == nil {
		return nil
	}
	out := make([]string, len(t.tokenNameByID))
	copy(out, t.tokenNameByID)
	return out
}

/* NodeKindNames returns a copy of node kind names in ID order (for codegen). */
func (t *CompiledSymbolTable) NodeKindNames() []string {
	if t == nil {
		return nil
	}
	out := make([]string, len(t.nodeNameByID))
	copy(out, t.nodeNameByID)
	return out
}

/*
CollectCompiledSymbolStrings gathers every token name, role name, and node kind string
that the compiler may reference so IDs are stable before lowering.
*/
func CollectCompiledSymbolStrings(root *Node, env *SemanticEnv, eofTokenName string) (tokens, roles, nodeKinds []string) {
	ts := make(map[string]struct{})
	rs := make(map[string]struct{})
	nk := make(map[string]struct{})

	for name := range env.Tokens {
		ts[name] = struct{}{}
	}
	collectReferencedImportedSymbols(root, env, ts, nk)
	collectEmbeddedImportSymbols(env, ts, rs)
	collectImportedTokens(env, ts)
	collectImportedNodeKinds(env, nk)
	ts[eofTokenName] = struct{}{}

	if lex := root.FindFirstKind(NodeLexSection); lex != nil {
		for _, rule := range lex.FindAllKind(NodeLexRule) {
			roleNode := rule.FindFirstKind(NodeLexRuleRole)
			if roleNode != nil {
				rs[dslspec.NodeSingleTokenContent(roleNode)] = struct{}{}
			}
		}
	}

	if parse := root.FindFirstKind(NodeParseSection); parse != nil {
		if ign := parse.FindFirstKind(NodeParseIgnoreSection); ign != nil {
			for _, roleNode := range ign.FindAllKind(NodeParseIgnoreRole) {
				rs[dslspec.NodeSingleTokenContent(roleNode)] = struct{}{}
			}
		}
		for _, pr := range parse.FindAllKind(NodeParseRule) {
			if nn := pr.FindFirstKind(NodeParseNodeName); nn != nil {
				nk[dslspec.NodeSingleTokenContent(nn)] = struct{}{}
			}
			if body := pr.FindFirstKind(NodeParseRuleBody); body != nil {
				walkParseTreeForSymbols(body, env, ts, nk)
			}
		}
		for _, tpl := range parse.FindAllKind(NodeParseTemplate) {
			if body := tpl.FindFirstKind(NodeParseTemplateBody); body != nil {
				walkParseTreeForSymbols(body, env, ts, nk)
			}
		}
	}

	if pratt := root.FindFirstKind(NodePrattSection); pratt != nil {
		for _, expr := range pratt.FindAllKind(NodePrattExprDef) {
			if body := expr.FindFirstKind(NodePrattExprBody); body != nil {
				for _, cat := range body.ChildrenUnsafe() {
					walkPrattCategoryForSymbols(cat, env, ts, nk)
				}
			}
		}
	}

	nk["ERROR_NODE"] = struct{}{}

	return sortedStringKeys(ts), sortedStringKeys(rs), sortedStringKeys(nk)
}

func collectEmbeddedImportSymbols(env *SemanticEnv, ts map[string]struct{}, rs map[string]struct{}) {
	if env == nil {
		return
	}
	for alias, module := range env.Imports {
		if module == nil || !module.IsEmbed {
			continue
		}
		for tokenName := range module.Tokens {
			ts[alias+"__"+tokenName] = struct{}{}
		}
		if module.Root == nil {
			continue
		}
		if lex := module.Root.FindFirstKind(NodeLexSection); lex != nil {
			for _, rule := range lex.FindAllKind(NodeLexRule) {
				roleNode := rule.FindFirstKind(NodeLexRuleRole)
				if roleNode == nil {
					continue
				}
				rs[alias+"__"+NodeSingleTokenContent(roleNode)] = struct{}{}
			}
		}
	}
}

func collectImportedNodeKinds(env *SemanticEnv, nk map[string]struct{}) {
	if env == nil || nk == nil {
		return
	}
	for _, alias := range collectDirectImportAliases(env) {
		collectImportedNodeKindsRecursive(env, env.Imports[alias], alias, nk, map[string]bool{})
	}
}

func collectImportedTokens(env *SemanticEnv, ts map[string]struct{}) {
	if env == nil || ts == nil {
		return
	}
	for _, alias := range collectDirectImportAliases(env) {
		collectImportedTokensRecursive(env, env.Imports[alias], alias, ts, map[string]bool{})
	}
}

func collectDirectImportAliases(env *SemanticEnv) []string {
	if env == nil {
		return nil
	}
	out := make([]string, 0, len(env.Imports))
	for alias := range env.Imports {
		if alias == "" || strings.Contains(alias, "__") {
			continue
		}
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}

func collectImportAliasesFromRoot(root *Node) []string {
	if root == nil {
		return nil
	}
	importSection := root.FindFirstKind(NodeImportSection)
	if importSection == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, imp := range importSection.FindAllKind(NodeImportDefinition) {
		aliasNode := imp.FindFirstKind(NodeImportAlias)
		if aliasNode == nil {
			continue
		}
		alias := IdentifierValue(aliasNode)
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}

func collectImportedTokensRecursive(env *SemanticEnv, module *ImportedModuleSymbols, prefix string, ts map[string]struct{}, seen map[string]bool) {
	if env == nil || module == nil || module.Root == nil || prefix == "" {
		return
	}
	key := module.Path + "|" + prefix
	if seen[key] {
		return
	}
	seen[key] = true

	moduleEnv := BuildSemanticEnvWithImports(module.Root, env.Imports, nil)
	for tokName := range moduleEnv.Tokens {
		ts[prefix+"__"+tokName] = struct{}{}
	}
	for _, childAlias := range collectImportAliasesFromRoot(module.Root) {
		child := env.Imports[childAlias]
		if child == nil {
			continue
		}
		collectImportedTokensRecursive(env, child, prefix+"__"+childAlias, ts, seen)
	}
}

func collectImportedNodeKindsRecursive(env *SemanticEnv, module *ImportedModuleSymbols, prefix string, nk map[string]struct{}, seen map[string]bool) {
	if env == nil || module == nil || module.Root == nil || prefix == "" {
		return
	}
	key := module.Path + "|" + prefix
	if seen[key] {
		return
	}
	seen[key] = true

	moduleEnv := BuildSemanticEnvWithImports(module.Root, env.Imports, nil)
	for _, ruleNode := range moduleEnv.Rules {
		if ruleNode == nil {
			continue
		}
		if nodeNameNode := ruleNode.FindFirstKind(NodeParseNodeName); nodeNameNode != nil {
			nk[prefix+"__"+NodeSingleTokenContent(nodeNameNode)] = struct{}{}
		}
	}
	for _, childAlias := range collectImportAliasesFromRoot(module.Root) {
		child := env.Imports[childAlias]
		if child == nil {
			continue
		}
		collectImportedNodeKindsRecursive(env, child, prefix+"__"+childAlias, nk, seen)
	}
}

func collectReferencedImportedTokens(root *Node, env *SemanticEnv, ts map[string]struct{}) {
	collectReferencedImportedSymbols(root, env, ts, nil)
}

func collectReferencedImportedSymbols(root *Node, env *SemanticEnv, ts map[string]struct{}, nk map[string]struct{}) {
	if root == nil || env == nil {
		return
	}
	for _, usingRef := range root.FindAllKind(NodeLexRulePatternUsing) {
		moduleNode := usingRef.FindFirstKind(NodeModuleReference)
		symbolNode := usingRef.FindFirstKind(NodePatternExternalPatternReference)
		if moduleNode == nil || symbolNode == nil {
			continue
		}
		moduleName := IdentifierValue(moduleNode)
		symbolName := IdentifierValue(symbolNode)
		module, ok := env.Imports[moduleName]
		if !ok || module == nil {
			continue
		}
		if pairDecl := module.ExportedPairs[symbolName]; pairDecl != nil {
			ts[pairDecl.OpenToken] = struct{}{}
			ts[pairDecl.CloseToken] = struct{}{}
			continue
		}
		if ruleNode := module.ExportedRules[symbolName]; ruleNode != nil {
			localModuleEnv := BuildSemanticEnvWithImports(module.Root, env.Imports, nil)
			if nk != nil {
				if nodeNameNode := ruleNode.FindFirstKind(NodeParseNodeName); nodeNameNode != nil {
					nk[moduleName+"__"+NodeSingleTokenContent(nodeNameNode)] = struct{}{}
				}
			}
			body := ruleNode.FindFirstKind(NodeParseRuleBody)
			if body != nil {
				collectTokenAndNodeRefsFromParseNodeWithAlias(body, localModuleEnv, moduleName, ts, nk)
			}
			continue
		}
		if tplDecl := module.ExportedTemplates[symbolName]; tplDecl != nil && tplDecl.Node != nil {
			body := tplDecl.Node.FindFirstKind(NodeParseTemplateBody)
			if body != nil {
				localModuleEnv := BuildSemanticEnvWithImports(module.Root, env.Imports, nil)
				collectTokenAndNodeRefsFromParseNodeWithAlias(body, localModuleEnv, moduleName, ts, nk)
			}
		}
	}
}

func collectTokenAndNodeRefsFromParseNodeWithAlias(node *Node, env *SemanticEnv, moduleAlias string, ts map[string]struct{}, nk map[string]struct{}) {
	localTokens := make(map[string]struct{})
	localNodeKinds := make(map[string]struct{})
	collectTokenRefsFromParseNode(node, env.Pairs, localTokens)
	walkParseTreeForSymbols(node, env, localTokens, localNodeKinds)

	for tokenName := range localTokens {
		ts[tokenName] = struct{}{}
		if moduleAlias != "" {
			ts[moduleAlias+"__"+tokenName] = struct{}{}
		}
	}
	if nk == nil {
		return
	}
	for nodeName := range localNodeKinds {
		nk[moduleAlias+"__"+nodeName] = struct{}{}
	}
}

func collectTokenRefsFromParseNode(node *Node, pairs map[string]*PairDecl, ts map[string]struct{}) {
	if node == nil {
		return
	}
	node.WalkPre(func(current *Node) (bool, bool) {
		switch current.Kind() {
		case NodeParseTokenReference, NodeParseNestOpenToken, NodeParseNestCloseToken:
			name := NodeSingleTokenContent(current)
			if name != "" {
				ts[name] = struct{}{}
			}
		case NodeParseNestPairRef:
			pairName := PairNameFromNestPairRefNode(current)
			if pair := pairs[pairName]; pair != nil {
				ts[pair.OpenToken] = struct{}{}
				ts[pair.CloseToken] = struct{}{}
			}
		}
		return false, false
	})
}

func sortedStringKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func walkParseTreeForSymbols(node *Node, env *SemanticEnv, ts, nk map[string]struct{}) {
	if node == nil {
		return
	}
	if node.Kind() == NodeParseSegment && SegmentHasExplicitTemplateInvocation(node) {
		collectTemplateCallNodeArgKinds(node, env, nk)
	}
	switch node.Kind() {
	case NodeParseTokenReference:
		ts[dslspec.NodeSingleTokenContent(node)] = struct{}{}
	case NodeParseExpressionReference:
		name := dslspec.NodeSingleTokenContent(node)
		if env.Tokens[name] != nil {
			ts[name] = struct{}{}
		}
	case NodePredictToken:
		ts[dslspec.NodeSingleTokenContent(node)] = struct{}{}
	case NodeSyncToken:
		ts[dslspec.NodeSingleTokenContent(node)] = struct{}{}
	case NodeParseNestOpenToken, NodeParseNestCloseToken:
		ts[dslspec.NodeSingleTokenContent(node)] = struct{}{}
	case NodeParseNodeName:
		nk[dslspec.NodeSingleTokenContent(node)] = struct{}{}
	case NodeParseSymbolReference:
		nk[dslspec.IdentifierValue(node)] = struct{}{}
	case NodeParseTemplateReference:
		// Callee of call Name(…); template name is not a parse output node kind.
	}
	for _, ch := range node.ChildrenUnsafe() {
		walkParseTreeForSymbols(ch, env, ts, nk)
	}
}

func collectTemplateCallNodeArgKinds(segment *Node, env *SemanticEnv, nk map[string]struct{}) {
	if segment == nil || env == nil || nk == nil {
		return
	}
	callArgs := TemplateCallArgsNode(segment)
	if callArgs == nil {
		return
	}
	callee, _, ok := TemplateCallCalleeName(segment)
	if !ok {
		return
	}
	decl := env.Templates[callee]
	if decl == nil {
		return
	}
	args := TemplateCallArgumentNodes(callArgs)
	if len(args) != len(decl.Params) {
		return
	}
	for i, param := range decl.Params {
		if param.Type != TemplateParamNode {
			continue
		}
		arg := args[i]
		if arg == nil {
			continue
		}
		toks := arg.Tokens()
		if len(toks) == 0 {
			continue
		}
		if LangSpecLexerTokenType(toks[0].Token) != TokIdentifier {
			continue
		}
		name := strings.TrimSpace(string(toks[0].Raw))
		if name != "" {
			nk[name] = struct{}{}
		}
	}
}

func walkPrattCategoryForSymbols(node *Node, env *SemanticEnv, ts, nk map[string]struct{}) {
	if node == nil {
		return
	}
	switch node.Kind() {
	case NodeParseTokenReference:
		ts[dslspec.NodeSingleTokenContent(node)] = struct{}{}
	case NodeParseExpressionReference:
		name := dslspec.NodeSingleTokenContent(node)
		if env.Tokens[name] != nil {
			ts[name] = struct{}{}
		}
	case NodeParseNodeName:
		nk[dslspec.NodeSingleTokenContent(node)] = struct{}{}
	case NodeParseSymbolReference:
		nk[dslspec.IdentifierValue(node)] = struct{}{}
	}
	for _, ch := range node.ChildrenUnsafe() {
		walkPrattCategoryForSymbols(ch, env, ts, nk)
	}
}
