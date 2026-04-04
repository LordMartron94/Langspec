package semantics

import (
	"sort"
	"strconv"

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
