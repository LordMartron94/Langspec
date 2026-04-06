package semantics

import . "langspec/dsl/spec"

func ImportedModuleSymbolsBuild(alias, path string, root *Node) *ImportedModuleSymbols {
	module := &ImportedModuleSymbols{
		Alias:             alias,
		Path:              path,
		Root:              root,
		Tokens:            make(map[string]*Node),
		ExportedPatterns:  make(map[string]*Node),
		ExportedRules:     make(map[string]*Node),
		ExportedPairs:     make(map[string]*PairDecl),
		ExportedTemplates: make(map[string]*TemplateDecl),
		ExportedPratt:     make(map[string]*Node),
		IgnoreRoles:       make(map[string]bool),
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
