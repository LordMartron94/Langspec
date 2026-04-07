package semantics

import . "langspec/dsl/spec"

type ImportLexicalObligation struct {
	RequiredTokens map[string]bool
	RequiredStates map[string]bool
}

func ImportLexicalObligationCreate() *ImportLexicalObligation {
	return &ImportLexicalObligation{
		RequiredTokens: make(map[string]bool),
		RequiredStates: make(map[string]bool),
	}
}

func ImportLexicalObligationClone(src *ImportLexicalObligation) *ImportLexicalObligation {
	dst := ImportLexicalObligationCreate()
	if src == nil {
		return dst
	}
	for token := range src.RequiredTokens {
		dst.RequiredTokens[token] = true
	}
	for state := range src.RequiredStates {
		dst.RequiredStates[state] = true
	}
	return dst
}

func ImportedModuleSymbolsBuild(alias, path string, root *Node) *ImportedModuleSymbols {
	module := &ImportedModuleSymbols{
		Alias:                      alias,
		Path:                       path,
		Root:                       root,
		Tokens:                     make(map[string]*Node),
		ExportedPatterns:           make(map[string]*Node),
		ExportedRules:              make(map[string]*Node),
		ExportedPairs:              make(map[string]*PairDecl),
		ExportedTemplates:          make(map[string]*TemplateDecl),
		ExportedPratt:              make(map[string]*Node),
		ExportedLexicalObligations: make(map[string]*ImportLexicalObligation),
		IgnoreRoles:                make(map[string]bool),
	}

	if root == nil {
		return module
	}

	localEnv := BuildSemanticEnv(root, nil)
	for name, tokenNode := range localEnv.Tokens {
		module.Tokens[name] = tokenNode
	}

	buildImportedExportedPatterns(module, root)
	buildImportedExportedParseSymbols(module, root)
	buildImportedLexicalObligations(module, root, localEnv)
	buildImportedIgnoreRoles(module, root)

	return module
}

func buildImportedExportedPatterns(module *ImportedModuleSymbols, root *Node) {
	for _, def := range root.FindAllKind(NodePatternDefinition) {
		if def.FindFirstKind(NodeExported) == nil {
			continue
		}
		name, _ := ExtractPatternDefName(def)
		if name == "" {
			continue
		}
		module.ExportedPatterns[name] = def
	}
}

func buildImportedExportedParseSymbols(module *ImportedModuleSymbols, root *Node) {
	parseSection := root.FindFirstKind(NodeParseSection)
	if parseSection == nil {
		return
	}
	for _, block := range parseSection.FindAllKind(NodeParseBlock) {
		if block.FindFirstKind(NodeExported) == nil {
			continue
		}
		if rule := block.FindFirstKind(NodeParseRule); rule != nil {
			name := ParseRuleName(rule.FindFirstKind(NodeParseRuleName))
			if name != "" {
				module.ExportedRules[name] = rule
			}
			continue
		}
		if pairNode := block.FindFirstKind(NodeParsePair); pairNode != nil {
			name, openTok, closeTok, ok := ExtractPairDeclaration(pairNode)
			if ok {
				module.ExportedPairs[name] = &PairDecl{
					Node:       pairNode,
					OpenToken:  openTok,
					CloseToken: closeTok,
				}
			}
			continue
		}
		if tplNode := block.FindFirstKind(NodeParseTemplate); tplNode != nil {
			decl, ok := ExtractTemplateDeclaration(tplNode)
			if ok && decl.Name != "" {
				module.ExportedTemplates[decl.Name] = &decl
			}
			continue
		}
	}
}

func buildImportedIgnoreRoles(module *ImportedModuleSymbols, root *Node) {
	parseSection := root.FindFirstKind(NodeParseSection)
	if parseSection == nil {
		return
	}
	ignoreSection := parseSection.FindFirstKind(NodeParseIgnoreSection)
	if ignoreSection == nil {
		return
	}
	for _, roleNode := range ignoreSection.FindAllKind(NodeParseIgnoreRole) {
		roleName := IdentifierValue(roleNode)
		if roleName == "" {
			continue
		}
		module.IgnoreRoles[roleName] = true
	}
}

func buildImportedLexicalObligations(module *ImportedModuleSymbols, root *Node, env *SemanticEnv) {
	if module == nil || root == nil || env == nil {
		return
	}
	tokenStates := buildLexTokenRequiredStateMap(root)
	for name, pairDecl := range module.ExportedPairs {
		obligation := ImportLexicalObligationCreate()
		addTokenAndStatesForObligation(obligation, pairDecl.OpenToken, tokenStates)
		addTokenAndStatesForObligation(obligation, pairDecl.CloseToken, tokenStates)
		module.ExportedLexicalObligations[name] = obligation
	}
	for name, ruleNode := range module.ExportedRules {
		obligation := ImportLexicalObligationCreate()
		body := ruleNode.FindFirstKind(NodeParseRuleBody)
		collectParseNodeTokenObligations(body, env.Pairs, obligation.RequiredTokens)
		appendRequiredStatesFromTokens(obligation, tokenStates)
		module.ExportedLexicalObligations[name] = obligation
	}
	for name, templateDecl := range module.ExportedTemplates {
		obligation := ImportLexicalObligationCreate()
		if templateDecl != nil && templateDecl.Node != nil {
			body := templateDecl.Node.FindFirstKind(NodeParseTemplateBody)
			collectParseNodeTokenObligations(body, env.Pairs, obligation.RequiredTokens)
			appendRequiredStatesFromTokens(obligation, tokenStates)
		}
		module.ExportedLexicalObligations[name] = obligation
	}
}

func buildLexTokenRequiredStateMap(root *Node) map[string]map[string]bool {
	out := make(map[string]map[string]bool)
	lexSection := root.FindFirstKind(NodeLexSection)
	if lexSection == nil {
		return out
	}
	for _, rule := range lexSection.FindAllKind(NodeLexRule) {
		tokenNode := rule.FindFirstKind(NodeLexRuleTokenName)
		if tokenNode == nil {
			continue
		}
		tokenName := IdentifierValue(tokenNode)
		if tokenName == "" {
			continue
		}
		mutRoot := rule.FindFirstKind(NodeLexRuleStateMutation)
		if mutRoot == nil {
			continue
		}
		states := out[tokenName]
		if states == nil {
			states = make(map[string]bool)
			out[tokenName] = states
		}
		for _, stateRef := range mutRoot.FindAllKind(NodeStateReference) {
			stateName := IdentifierValue(stateRef)
			if stateName == "" {
				continue
			}
			states[stateName] = true
		}
	}
	return out
}

func collectParseNodeTokenObligations(node *Node, pairs map[string]*PairDecl, tokens map[string]bool) {
	if node == nil {
		return
	}
	node.WalkPre(func(cur *Node) (bool, bool) {
		switch cur.Kind() {
		case NodeParseTokenReference, NodeParseNestOpenToken, NodeParseNestCloseToken:
			tokenName := NodeSingleTokenContent(cur)
			if tokenName != "" {
				tokens[tokenName] = true
			}
		case NodeParseNestPairRef:
			pairName := PairNameFromNestPairRefNode(cur)
			if pair := pairs[pairName]; pair != nil {
				tokens[pair.OpenToken] = true
				tokens[pair.CloseToken] = true
			}
		}
		return false, false
	})
}

func addTokenAndStatesForObligation(obligation *ImportLexicalObligation, tokenName string, tokenStates map[string]map[string]bool) {
	if obligation == nil || tokenName == "" {
		return
	}
	obligation.RequiredTokens[tokenName] = true
	for state := range tokenStates[tokenName] {
		obligation.RequiredStates[state] = true
	}
}

func appendRequiredStatesFromTokens(obligation *ImportLexicalObligation, tokenStates map[string]map[string]bool) {
	if obligation == nil {
		return
	}
	for tokenName := range obligation.RequiredTokens {
		for state := range tokenStates[tokenName] {
			obligation.RequiredStates[state] = true
		}
	}
}
