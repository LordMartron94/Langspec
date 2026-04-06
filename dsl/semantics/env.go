package semantics

import . "langspec/dsl/spec"

/* SemanticSymbolKind identifies which symbol table a duplicate belongs to. */
type SemanticSymbolKind uint8

const (
	SymbolKindToken SemanticSymbolKind = iota
	SymbolKindPattern
	SymbolKindRule
	SymbolKindPair
	SymbolKindTemplate
	SymbolKindPratt
)

/*
SemanticEnv holds the resolved symbol tables for tokens, patterns, parse rules, and pratt expressions.
Built from the LST by BuildSemanticEnv and used by validation and compilation for reference resolution.
*/
type SemanticEnv struct {
	Tokens    map[string]*Node
	Patterns  map[string]*Node
	Rules     map[string]*Node
	Pairs     map[string]*PairDecl
	Templates map[string]*TemplateDecl
	Pratt     map[string]*Node

	LocalPatterns map[string]bool
	LocalPratt    map[string]bool

	Imports map[string]*ImportedModuleSymbols
}

type ImportedModuleSymbols struct {
	Alias string
	Path  string

	Root   *Node
	Tokens map[string]*Node

	ExportedPatterns  map[string]*Node
	ExportedRules     map[string]*Node
	ExportedPairs     map[string]*PairDecl
	ExportedTemplates map[string]*TemplateDecl
	ExportedPratt     map[string]*Node

	IgnoreRoles map[string]bool
}

/*
OnDuplicateFunc is called when a duplicate symbol is found during env build.
kind identifies the symbol table; name is the symbol name; node is the duplicate declaration.
Pass nil to BuildSemanticEnv to skip reporting (e.g. in the compiler).
*/
type OnDuplicateFunc func(kind SemanticSymbolKind, name string, node *Node)

/* BuildSemanticEnv builds the semantic environment from the LST root. When onDuplicate is non-nil and a duplicate symbol is found, it is called with kind, name, and the duplicate node. */
func BuildSemanticEnv(root *Node, onDuplicate OnDuplicateFunc) *SemanticEnv {
	return BuildSemanticEnvWithImports(root, nil, onDuplicate)
}

func BuildSemanticEnvWithImports(root *Node, imports map[string]*ImportedModuleSymbols, onDuplicate OnDuplicateFunc) *SemanticEnv {
	env := &SemanticEnv{
		Tokens:        make(map[string]*Node),
		Patterns:      make(map[string]*Node),
		Rules:         make(map[string]*Node),
		Pairs:         make(map[string]*PairDecl),
		Templates:     make(map[string]*TemplateDecl),
		Pratt:         make(map[string]*Node),
		LocalPatterns: make(map[string]bool),
		LocalPratt:    make(map[string]bool),
		Imports:       make(map[string]*ImportedModuleSymbols),
	}

	buildEnvTokens(root, env, onDuplicate)
	buildEnvPatterns(root, env, onDuplicate)
	buildEnvRules(root, env, onDuplicate)
	buildEnvPairs(root, env, onDuplicate)
	buildEnvTemplates(root, env, onDuplicate)
	buildEnvPratt(root, env, onDuplicate)
	buildEnvImports(env, imports)

	return env
}

func buildEnvImports(env *SemanticEnv, imports map[string]*ImportedModuleSymbols) {
	if imports == nil {
		return
	}
	for alias, module := range imports {
		if alias == "" || module == nil {
			continue
		}
		env.Imports[alias] = module
	}
}

func buildEnvTokens(root *Node, env *SemanticEnv, onDuplicate OnDuplicateFunc) {
	lex := root.FindFirstKind(NodeLexSection)
	if lex == nil {
		return
	}
	for _, tokenNode := range lex.FindAllKind(NodeLexRuleTokenName) {
		name := IdentifierValue(tokenNode)
		if _, exists := env.Tokens[name]; exists {
			if onDuplicate != nil {
				onDuplicate(SymbolKindToken, name, tokenNode)
			}
		} else {
			env.Tokens[name] = tokenNode
		}
	}
}

func buildEnvPatterns(root *Node, env *SemanticEnv, onDuplicate OnDuplicateFunc) {
	for _, def := range root.FindAllKind(NodePatternDefinition) {
		name, nameNode := ExtractPatternDefName(def)
		if name == "" {
			continue
		}
		if _, exists := env.Patterns[name]; exists {
			if onDuplicate != nil {
				onDuplicate(SymbolKindPattern, name, nameNode)
			}
		} else {
			env.Patterns[name] = def
			if def.FindFirstKind(NodeLocalVariable) != nil {
				env.LocalPatterns[name] = true
			}
		}
	}
}

func buildEnvRules(root *Node, env *SemanticEnv, onDuplicate OnDuplicateFunc) {
	parse := root.FindFirstKind(NodeParseSection)
	if parse == nil {
		return
	}
	for _, rule := range parse.FindAllKind(NodeParseRule) {
		nameNode := rule.FindFirstKind(NodeParseRuleName)
		name := ParseRuleName(nameNode)
		if name == "" {
			continue
		}
		if _, exists := env.Rules[name]; exists {
			if onDuplicate != nil {
				onDuplicate(SymbolKindRule, name, nameNode)
			}
		} else {
			env.Rules[name] = rule
		}
	}
}

func buildEnvPairs(root *Node, env *SemanticEnv, onDuplicate OnDuplicateFunc) {
	parse := root.FindFirstKind(NodeParseSection)
	if parse == nil {
		return
	}
	for _, pairNode := range parse.FindAllKind(NodeParsePair) {
		name, openTok, closeTok, ok := ExtractPairDeclaration(pairNode)
		if !ok {
			continue
		}
		if _, exists := env.Pairs[name]; exists {
			if onDuplicate != nil {
				onDuplicate(SymbolKindPair, name, pairNode)
			}
			continue
		}
		env.Pairs[name] = &PairDecl{
			Node:       pairNode,
			OpenToken:  openTok,
			CloseToken: closeTok,
		}
	}
}

func buildEnvTemplates(root *Node, env *SemanticEnv, onDuplicate OnDuplicateFunc) {
	parse := root.FindFirstKind(NodeParseSection)
	if parse == nil {
		return
	}
	for _, tplNode := range parse.FindAllKind(NodeParseTemplate) {
		decl, ok := ExtractTemplateDeclaration(tplNode)
		if !ok || decl.Name == "" {
			continue
		}
		if _, exists := env.Templates[decl.Name]; exists {
			if onDuplicate != nil {
				idNode := tplNode.FindFirstKind(NodeParseTemplateIdentifier)
				onDuplicate(SymbolKindTemplate, decl.Name, idNode)
			}
			continue
		}
		env.Templates[decl.Name] = &decl
	}
}

func buildEnvPratt(root *Node, env *SemanticEnv, onDuplicate OnDuplicateFunc) {
	pratt := root.FindFirstKind(NodePrattSection)
	if pratt == nil {
		return
	}
	for _, expr := range pratt.FindAllKind(NodePrattExprDef) {
		nameNode := expr.FindFirstKind(NodePrattExprName)
		name := IdentifierValue(nameNode)
		if name == "" {
			continue
		}
		if _, exists := env.Pratt[name]; exists {
			if onDuplicate != nil {
				onDuplicate(SymbolKindPratt, name, nameNode)
			}
		} else {
			env.Pratt[name] = expr
			if expr.FindFirstKind(NodeLocalVariable) != nil {
				env.LocalPratt[name] = true
			}
		}
	}
}

/* PatternRefTargetName returns the raw lexeme text of a pattern reference node (the referenced name). */
func PatternRefTargetName(patternRef *Node) string {
	return string(patternRef.Tokens()[0].Raw)
}

func ResolveImportedModule(env *SemanticEnv, alias string) (*ImportedModuleSymbols, bool) {
	if env == nil || alias == "" {
		return nil, false
	}
	module, ok := env.Imports[alias]
	return module, ok
}
