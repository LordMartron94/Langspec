package semantics

import (
	"fmt"
	"langspec/validation"
	"lexarch"
	"sort"
	"strconv"
	"strings"
	"syntaxa"
	"syntaxa/lowering"

	. "langspec/dsl/spec"
)

type GrammarValidationState[TNodeKind ~uint32] struct {
	Package         *GrammarPackage[TNodeKind]
	SourceMap       map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*Node
	Symbols         *CompiledSymbolTable
	ImportedModules map[string]*ImportedModuleSymbols
	LibraryMode     bool
}

/* ValidationCtx is the validation stage context type for LangSpec LST validation. */
type ValidationCtx[TNodeKind ~uint32] = validation.LSTValidationStageContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *GrammarValidationState[TNodeKind]]

/*
ValidationCode is a stable machine-readable diagnostic code for LangSpec validation (e.g. V_PAT001).
*/
type ValidationCode string

const (
	VALIDATION_DUPLICATE_TOKEN ValidationCode = "V_R001"

	VALIDATION_DUPLICATE_PATTERN_NAME    ValidationCode = "V_PAT001"
	VALIDATION_UNRESOLVED_PATTERN_REF    ValidationCode = "V_PAT002"
	VALIDATION_EMPTY_PATTERN_EXPRESSION  ValidationCode = "V_PAT003"
	VALIDATION_LOCAL_REF_OUTSIDE_SECTION ValidationCode = "V_PAT004"
	VALIDATION_CYCLIC_PATTERN_REF        ValidationCode = "V_PAT005"

	VALIDATION_UNREACHABLE_PATTERN         ValidationCode = "V_PAT006"
	VALIDATION_AMBIGUOUS_TOKEN_MATCH       ValidationCode = "V_LEX002"
	VALIDATION_IDENTICAL_TOKEN_PATTERN     ValidationCode = "V_LEX003"
	VALIDATION_TOKEN_SHADOWED              ValidationCode = "V_LEX004"
	VALIDATION_TOKEN_UNREFERENCED_IN_PARSE ValidationCode = "V_LEX005"
	VALIDATION_UNRESOLVED_TOKEN_REF        ValidationCode = "V_LEX006"

	VALIDATION_LEX_INITIAL_STATE_MISSING      ValidationCode = "V_LEX007"
	VALIDATION_LEX_UNDEFINED_STATE_REF        ValidationCode = "V_LEX008"
	VALIDATION_LEX_UNREFERENCED_STATE         ValidationCode = "V_LEX009"
	VALIDATION_LEX_PUSH_SET_EMPTY_ARGS        ValidationCode = "V_LEX010"
	VALIDATION_LEX_TOKEN_MULTI_STATE_CONFLICT ValidationCode = "V_LEX011"
	VALIDATION_LEX_RULE_MISSING_PATTERN       ValidationCode = "V_LEX012"
	VALIDATION_LEX_DUPLICATE_STATE_NAME       ValidationCode = "V_LEX013"

	VALIDATION_NEGATION_INVALID_CONTENT ValidationCode = "V_PAT007"

	// Repetition bounds ({n,m}) apply to both pattern expressions and parse NodeRepetition; codes use V_REP (not V_PAT).
	VALIDATION_REPETITION_BOUNDS_EMPTY   ValidationCode = "V_REP001"
	VALIDATION_REPETITION_MIN_GT_MAX     ValidationCode = "V_REP002"
	VALIDATION_REPETITION_NEGATIVE_BOUND ValidationCode = "V_REP003"

	VALIDATION_DUPLICATE_PARSE_RULE_NAME ValidationCode = "V_PAR001"
	VALIDATION_UNRESOLVED_PARSE_RULE_REF ValidationCode = "V_PAR002"
	VALIDATION_UNREFERENCED_PARSE_RULE   ValidationCode = "V_PAR003"
	VALIDATION_PROGRAM_RULE_REQUIRED     ValidationCode = "V_PAR004"

	VALIDATION_PARSE_LEFT_RECURSION                  ValidationCode = "V_PAR005"
	VALIDATION_PARSE_UNBOUNDED_OPTIONAL_REPETITION   ValidationCode = "V_PAR006"
	VALIDATION_TOKEN_REFERENCED_AS_EXPRESSION        ValidationCode = "V_PAR007"
	VALIDATION_EXPRESSION_REFERENCED_AS_TOKEN_OUTPUT ValidationCode = "V_PAR008"
	VALIDATION_FIRST_SET_CONFLICT                    ValidationCode = "V_PAR009"

	VALIDATION_DUPLICATE_PAIR_NAME            ValidationCode = "V_PAR010"
	VALIDATION_UNRESOLVED_PAIR_REF            ValidationCode = "V_PAR012"
	VALIDATION_PAIR_REF_MALFORMED             ValidationCode = "V_PAR013"
	VALIDATION_SEPARATOR_REPEAT_OPTIONAL_TAIL ValidationCode = "V_PAR014"
	VALIDATION_ORDERED_CHOICE_OVERLAP         ValidationCode = "V_PAR015"
	// Ordered-choice prefix trap: earlier arm begins with the same rule as a later arm that is only that rule; PEG does not backtrack after a successful sub-parse.
	VALIDATION_ORDERED_CHOICE_PREFIX_TRAP ValidationCode = "V_PAR016"

	VALIDATION_DUPLICATE_PRATT_EXPR             ValidationCode = "V_PRA001"
	VALIDATION_PRATT_UNRESOLVED_TOKEN           ValidationCode = "V_PRA002"
	VALIDATION_PRATT_UNRESOLVED_PATTERN         ValidationCode = "V_PRA003"
	VALIDATION_PRATT_UNRESOLVED_RULE            ValidationCode = "V_PRA004"
	VALIDATION_PRATT_LOCAL_LEAK                 ValidationCode = "V_PRA005"
	VALIDATION_PRATT_UNRESOLVED_OPERATOR_TARGET ValidationCode = "V_PRA006"

	VALIDATION_DUPLICATE_TEMPLATE_NAME    ValidationCode = "V_TPL001"
	VALIDATION_TEMPLATE_BARE_REFERENCE    ValidationCode = "V_TPL002"
	VALIDATION_TEMPLATE_PARAM_OUTSIDE     ValidationCode = "V_TPL003"
	VALIDATION_TEMPLATE_UNKNOWN_PARAM     ValidationCode = "V_TPL004"
	VALIDATION_TEMPLATE_UNRESOLVED_CALLEE ValidationCode = "V_TPL005"
	VALIDATION_TEMPLATE_CALL_ARITY        ValidationCode = "V_TPL006"
	VALIDATION_TEMPLATE_CALL_ARG_TYPE     ValidationCode = "V_TPL007"
	VALIDATION_TEMPLATE_CYCLE             ValidationCode = "V_TPL008"
	VALIDATION_TEMPLATE_CALLEE_NOT_IDENT  ValidationCode = "V_TPL009"

	VALIDATION_SYMBOL_NAME_COLLISION ValidationCode = "V_SYM001"

	VALIDATION_IMPORT_DUPLICATE_ALIAS           ValidationCode = "V_IMP001"
	VALIDATION_IMPORT_MISSING_ALIAS             ValidationCode = "V_IMP002"
	VALIDATION_IMPORT_UNRESOLVED_MODULE         ValidationCode = "V_IMP003"
	VALIDATION_IMPORT_UNSUPPORTED_EXTERNAL      ValidationCode = "V_IMP004"
	VALIDATION_IMPORT_UNRESOLVED_EXPORT         ValidationCode = "V_IMP005"
	VALIDATION_IMPORT_INVALID_USING_CALL        ValidationCode = "V_IMP006"
	VALIDATION_IMPORT_MISSING_TOKEN_OBLIGATION  ValidationCode = "V_IMP007"
	VALIDATION_IMPORT_MISSING_STATE_OBLIGATION  ValidationCode = "V_IMP008"
	VALIDATION_IMPORT_EMBED_ALIAS_REQUIRED      ValidationCode = "V_IMP009"
	VALIDATION_IMPORT_EMBED_ENTRY_TOKEN_MISSING ValidationCode = "V_IMP010"
	VALIDATION_IMPORT_EMBED_EXIT_TOKEN_SHADOWED ValidationCode = "V_IMP011"
)

/* String returns the code string (implements fmt.Stringer). */
func (v ValidationCode) String() string {
	return string(v)
}

func attributeAs[TAttribute any](node *Node, attributeName string) (TAttribute, bool) {
	return syntaxa.AttributeAs[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, TAttribute](
		node, attributeName,
	)
}

/*
ValidationStages returns the ordered LST validation stages for the LangSpec DSL compiler.
*/
func ValidationStages[TNodeKind ~uint32]() []*validation.LSTValidationStage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *GrammarValidationState[TNodeKind]] {
	return []*validation.LSTValidationStage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *GrammarValidationState[TNodeKind]]{
		{
			Name:        "Symbol Binding & Environment",
			Description: "Builds semantic environment, checks duplicates, scoping, and reference resolution.",
			Order:       0,
			Processor:   processSymbolBinding[TNodeKind],
		},
		{
			Name:        "Structure & Reachability",
			Description: "Analyzes cross-section reachability and detects cyclic dependencies.",
			Order:       1,
			Processor:   processReachability[TNodeKind],
		},
		{
			Name:        "Lexer Semantics",
			Description: "Validates EOF configurations, shadowing, and ambiguous token matches.",
			Order:       2,
			Processor:   processLexSemantics[TNodeKind],
		},
		{
			Name:        "Pattern Semantics",
			Description: "Validates pattern negation and brace repetition bounds in pattern and parse sections.",
			Order:       3,
			Processor:   processPatternSemantics[TNodeKind],
		},
		{
			Name:        "Grammar Safety & Ambiguity",
			Description: "Analyzes lowered grammar for left-recursion, unbounded nullable repetitions, FIRST-set conflicts, and PEG prefix traps between ordered-choice arms.",
			Order:       4,
			Processor:   processGrammarSafety[TNodeKind],
		},
	}
}

// ------------------------------------------------------------- SYMBOL BINDING (STAGE 0)

func processSymbolBinding[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	var imports map[string]*ImportedModuleSymbols
	if ctx.RunState != nil {
		imports = ctx.RunState.ImportedModules
	}
	env := BuildSemanticEnvWithImports(ctx.RootNode, imports, func(kind SemanticSymbolKind, name string, node *Node) {
		var code ValidationCode
		var msg string
		switch kind {
		case SymbolKindToken:
			code = VALIDATION_DUPLICATE_TOKEN
			msg = fmt.Sprintf("token '%s' already declared", name)
		case SymbolKindPattern:
			code = VALIDATION_DUPLICATE_PATTERN_NAME
			msg = fmt.Sprintf("pattern '%s' already declared", name)
		case SymbolKindRule:
			code = VALIDATION_DUPLICATE_PARSE_RULE_NAME
			msg = fmt.Sprintf("parse rule '%s' already declared", name)
		case SymbolKindPair:
			code = VALIDATION_DUPLICATE_PAIR_NAME
			msg = fmt.Sprintf("pair '%s' already declared", name)
		case SymbolKindTemplate:
			code = VALIDATION_DUPLICATE_TEMPLATE_NAME
			msg = fmt.Sprintf("template '%s' already declared", name)
		case SymbolKindPratt:
			code = VALIDATION_DUPLICATE_PRATT_EXPR
			msg = fmt.Sprintf("pratt expression '%s' already declared", name)
		default:
			code = VALIDATION_DUPLICATE_TOKEN
			msg = fmt.Sprintf("symbol '%s' already declared", name)
		}
		ctx.ReportError(code.String(), msg, node)
	})

	validateSymbolNameCollisions(ctx, env)
	validateImportDefinitions(ctx)
	validateEmbedAliasDefinitions(ctx, env)
	validateTemplateParameterScoping(ctx, env)
	nodeKinds := collectDeclaredParseOutputNodeKinds(ctx.RootNode)
	validateUsingReferences(ctx, env, nodeKinds)
	validateImportLexicalObligations(ctx, env)
	validateEmbedEntryTokenReachability(ctx, env)
	validateEmbedExitTokenShadowing(ctx, env)
	validateTemplateCallSites(ctx, env, nodeKinds)
	validateTokenReferences(ctx, env)
	validatePatternReferences(ctx, env)
	validateRuleReferences(ctx, env)
}

func validateImportDefinitions[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	importSection := ctx.RootNode.FindFirstKind(NodeImportSection)
	if importSection == nil {
		return
	}
	seen := make(map[string]bool)
	for _, imp := range importSection.FindAllKind(NodeImportDefinition) {
		pathNode := imp.FindFirstKind(NodeImportPath)
		aliasNode := imp.FindFirstKind(NodeImportAlias)
		if pathNode == nil || aliasNode == nil {
			ctx.ReportError(VALIDATION_IMPORT_MISSING_ALIAS.String(), "import definition must include path and alias", imp)
			continue
		}
		alias := IdentifierValue(aliasNode)
		if alias == "" {
			ctx.ReportError(VALIDATION_IMPORT_MISSING_ALIAS.String(), "import alias cannot be empty", aliasNode)
			continue
		}
		if seen[alias] {
			ctx.ReportError(VALIDATION_IMPORT_DUPLICATE_ALIAS.String(), fmt.Sprintf("duplicate import alias '%s'", alias), aliasNode)
			continue
		}
		seen[alias] = true
	}
}

func validateUsingReferences[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, nodeKinds map[string]struct{}) {
	importAliases := collectImportAliases(ctx.RootNode)
	for _, usingRef := range ctx.RootNode.FindAllKind(NodeLexRulePatternUsing) {
		moduleNode := usingRef.FindFirstKind(NodeModuleReference)
		symbolNode := usingRef.FindFirstKind(NodePatternExternalPatternReference)
		if moduleNode == nil || symbolNode == nil {
			ctx.ReportError(VALIDATION_IMPORT_UNRESOLVED_MODULE.String(), "malformed using reference; expected using Module.Symbol", usingRef)
			continue
		}
		moduleName := IdentifierValue(moduleNode)
		symbolName := IdentifierValue(symbolNode)
		if moduleName == "" || symbolName == "" {
			ctx.ReportError(VALIDATION_IMPORT_UNRESOLVED_MODULE.String(), "malformed using reference; module and symbol are required", usingRef)
			continue
		}
		if !importAliases[moduleName] {
			ctx.ReportError(VALIDATION_IMPORT_UNRESOLVED_MODULE.String(), fmt.Sprintf("using references unknown import alias '%s'", moduleName), moduleNode)
			continue
		}
		module, ok := env.Imports[moduleName]
		if !ok || module == nil {
			ctx.ReportError(VALIDATION_IMPORT_UNRESOLVED_MODULE.String(), fmt.Sprintf("import alias '%s' could not be resolved", moduleName), moduleNode)
			continue
		}

		callArgs := usingRef.FindFirstKind(NodeParseTemplateCallArgs)
		isLexSite := nodeHasAncestorKind(usingRef, NodeLexRule)
		isNestPairSite := usingRef.Parent() != nil && usingRef.Parent().Kind() == NodeParseOpNest
		isParseExpressionSite := !isLexSite && !isNestPairSite

		if isLexSite {
			if callArgs != nil {
				ctx.ReportError(VALIDATION_IMPORT_INVALID_USING_CALL.String(), "lexer using-pattern does not support call arguments", usingRef)
				continue
			}
			if module.ExportedPatterns[symbolName] == nil {
				if module.ExportedRules[symbolName] != nil || module.ExportedTemplates[symbolName] != nil || module.ExportedPairs[symbolName] != nil {
					ctx.ReportError(VALIDATION_IMPORT_INVALID_USING_CALL.String(), fmt.Sprintf("lexer using requires an exported pattern, got '%s.%s'", moduleName, symbolName), usingRef)
				} else if module.ExportedPratt[symbolName] != nil || module.Tokens[symbolName] != nil {
					ctx.ReportError(VALIDATION_IMPORT_UNSUPPORTED_EXTERNAL.String(), fmt.Sprintf("external symbol '%s.%s' is not supported here", moduleName, symbolName), usingRef)
				} else {
					ctx.ReportError(VALIDATION_IMPORT_UNRESOLVED_EXPORT.String(), fmt.Sprintf("unresolved exported symbol '%s.%s'", moduleName, symbolName), usingRef)
				}
			}
			continue
		}

		if isNestPairSite {
			if callArgs != nil {
				ctx.ReportError(VALIDATION_IMPORT_INVALID_USING_CALL.String(), "nest pair using does not support call arguments", usingRef)
				continue
			}
			if module.ExportedPairs[symbolName] == nil {
				if module.ExportedRules[symbolName] != nil || module.ExportedTemplates[symbolName] != nil || module.ExportedPatterns[symbolName] != nil {
					ctx.ReportError(VALIDATION_IMPORT_INVALID_USING_CALL.String(), fmt.Sprintf("nest using requires an exported pair, got '%s.%s'", moduleName, symbolName), usingRef)
				} else if module.ExportedPratt[symbolName] != nil || module.Tokens[symbolName] != nil {
					ctx.ReportError(VALIDATION_IMPORT_UNSUPPORTED_EXTERNAL.String(), fmt.Sprintf("external symbol '%s.%s' is not supported here", moduleName, symbolName), usingRef)
				} else {
					ctx.ReportError(VALIDATION_IMPORT_UNRESOLVED_EXPORT.String(), fmt.Sprintf("unresolved exported symbol '%s.%s'", moduleName, symbolName), usingRef)
				}
			}
			continue
		}

		if isParseExpressionSite {
			if callArgs != nil {
				decl := module.ExportedTemplates[symbolName]
				if decl == nil {
					if module.ExportedRules[symbolName] != nil || module.ExportedPairs[symbolName] != nil || module.ExportedPatterns[symbolName] != nil {
						ctx.ReportError(VALIDATION_IMPORT_INVALID_USING_CALL.String(), fmt.Sprintf("only templates can be invoked with call arguments, got '%s.%s'", moduleName, symbolName), usingRef)
					} else if module.ExportedPratt[symbolName] != nil || module.Tokens[symbolName] != nil {
						ctx.ReportError(VALIDATION_IMPORT_UNSUPPORTED_EXTERNAL.String(), fmt.Sprintf("external symbol '%s.%s' is not supported", moduleName, symbolName), usingRef)
					} else {
						ctx.ReportError(VALIDATION_IMPORT_UNRESOLVED_EXPORT.String(), fmt.Sprintf("unresolved exported symbol '%s.%s'", moduleName, symbolName), usingRef)
					}
					continue
				}
				args := TemplateCallArgumentNodes(callArgs)
				if len(args) != len(decl.Params) {
					ctx.ReportError(VALIDATION_TEMPLATE_CALL_ARITY.String(),
						fmt.Sprintf("template '%s.%s' expects %d argument(s), got %d", moduleName, symbolName, len(decl.Params), len(args)),
						callArgs)
					continue
				}
				for i, p := range decl.Params {
					if p.Type == TemplateParamPrattExpr {
						ctx.ReportError(VALIDATION_IMPORT_UNSUPPORTED_EXTERNAL.String(), fmt.Sprintf("external template '%s.%s' cannot use PrattExpr parameter currently", moduleName, symbolName), usingRef)
						break
					}
					validateTemplateCallArg(ctx, env, nodeKinds, p.Type, args[i], nil)
				}
				continue
			}

			if module.ExportedTemplates[symbolName] != nil {
				ctx.ReportError(VALIDATION_IMPORT_INVALID_USING_CALL.String(), fmt.Sprintf("template '%s.%s' must be invoked as using %s.%s(...)", moduleName, symbolName, moduleName, symbolName), usingRef)
				continue
			}
			if module.ExportedRules[symbolName] != nil {
				continue
			}
			if module.ExportedPairs[symbolName] != nil || module.ExportedPatterns[symbolName] != nil {
				ctx.ReportError(VALIDATION_IMPORT_INVALID_USING_CALL.String(), fmt.Sprintf("parse using requires an exported rule or template call, got '%s.%s'", moduleName, symbolName), usingRef)
				continue
			}
			if module.ExportedPratt[symbolName] != nil || module.Tokens[symbolName] != nil {
				ctx.ReportError(VALIDATION_IMPORT_UNSUPPORTED_EXTERNAL.String(), fmt.Sprintf("external symbol '%s.%s' is not supported", moduleName, symbolName), usingRef)
				continue
			}
			ctx.ReportError(VALIDATION_IMPORT_UNRESOLVED_EXPORT.String(), fmt.Sprintf("unresolved exported symbol '%s.%s'", moduleName, symbolName), usingRef)
			continue
		}

		ctx.ReportError(VALIDATION_IMPORT_INVALID_USING_CALL.String(), "using reference appears in unsupported location", usingRef)
	}
}

func nodeHasAncestorKind(node *Node, kind LangSpecParserNodeKind) bool {
	for cur := node; cur != nil; cur = cur.Parent() {
		if cur.Kind() == kind {
			return true
		}
	}
	return false
}

func collectImportAliases(root *Node) map[string]bool {
	out := make(map[string]bool)
	importSection := root.FindFirstKind(NodeImportSection)
	if importSection == nil {
		return out
	}
	for _, imp := range importSection.FindAllKind(NodeImportDefinition) {
		aliasNode := imp.FindFirstKind(NodeImportAlias)
		if aliasNode == nil {
			continue
		}
		alias := IdentifierValue(aliasNode)
		if alias == "" {
			continue
		}
		out[alias] = true
	}
	return out
}

func collectEmbedImportAliases(root *Node) map[string]bool {
	out := make(map[string]bool)
	importSection := root.FindFirstKind(NodeImportSection)
	if importSection == nil {
		return out
	}
	for _, imp := range importSection.FindAllKind(NodeImportDefinition) {
		if imp.FindFirstKind(NodeImportEmbed) == nil {
			continue
		}
		aliasNode := imp.FindFirstKind(NodeImportAlias)
		if aliasNode == nil {
			continue
		}
		alias := IdentifierValue(aliasNode)
		if alias != "" {
			out[alias] = true
		}
	}
	return out
}

func validateEmbedAliasDefinitions[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv) {
	if ctx == nil || env == nil {
		return
	}
	embedAliases := collectEmbedImportAliases(ctx.RootNode)
	for _, embedNode := range ctx.RootNode.FindAllKind(NodeParseEmbedStatement) {
		moduleNode := embedNode.FindFirstKind(NodeModuleReference)
		if moduleNode == nil {
			continue
		}
		alias := IdentifierValue(moduleNode)
		if alias == "" {
			continue
		}
		if !embedAliases[alias] {
			ctx.ReportError(
				VALIDATION_IMPORT_EMBED_ALIAS_REQUIRED.String(),
				fmt.Sprintf("embed statement alias '%s' must come from `embed \"...\" as %s` import", alias, alias),
				moduleNode,
			)
			continue
		}
		module, ok := ResolveImportedModule(env, alias)
		if !ok || module == nil || !module.IsEmbed {
			ctx.ReportError(
				VALIDATION_IMPORT_EMBED_ALIAS_REQUIRED.String(),
				fmt.Sprintf("alias '%s' is not resolved as embed module", alias),
				moduleNode,
			)
		}
	}
}

func validateEmbedEntryTokenReachability[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv) {
	if ctx == nil || env == nil {
		return
	}
	lexRules := collectLexRules(ctx.RootNode)
	for _, embedNode := range ctx.RootNode.FindAllKind(NodeParseEmbedStatement) {
		openNode := embedNode.FindFirstKind(NodeParseNestOpenToken)
		if openNode == nil {
			continue
		}
		openToken := IdentifierValue(openNode)
		if openToken == "" {
			continue
		}
		found := false
		for _, rule := range lexRules {
			if rule.tokenName == openToken {
				found = true
				break
			}
		}
		if !found {
			ctx.ReportError(
				VALIDATION_IMPORT_EMBED_ENTRY_TOKEN_MISSING.String(),
				fmt.Sprintf("embed entry token '%s' is not emitted by any host lexer rule", openToken),
				openNode,
			)
		}
	}
}

func validateEmbedExitTokenShadowing[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv) {
	if ctx == nil || env == nil {
		return
	}
	lexRules := collectLexRules(ctx.RootNode)
	patternByToken := make(map[string]string, len(lexRules))
	for _, rule := range lexRules {
		if rule.patternKey == "" {
			continue
		}
		if _, exists := patternByToken[rule.tokenName]; !exists {
			patternByToken[rule.tokenName] = rule.patternKey
		}
	}
	for _, embedNode := range ctx.RootNode.FindAllKind(NodeParseEmbedStatement) {
		moduleNode := embedNode.FindFirstKind(NodeModuleReference)
		closeNode := embedNode.FindFirstKind(NodeParseNestCloseToken)
		if moduleNode == nil || closeNode == nil {
			continue
		}
		moduleAlias := IdentifierValue(moduleNode)
		closeToken := IdentifierValue(closeNode)
		if moduleAlias == "" || closeToken == "" {
			continue
		}
		module, ok := ResolveImportedModule(env, moduleAlias)
		if !ok || module == nil {
			continue
		}
		closePattern := patternByToken[closeToken]
		if closePattern == "" || module.Root == nil {
			continue
		}
		for _, embeddedRule := range collectLexRules(module.Root) {
			if embeddedRule.patternKey == closePattern && embeddedRule.priority >= 0 {
				ctx.ReportError(
					VALIDATION_IMPORT_EMBED_EXIT_TOKEN_SHADOWED.String(),
					fmt.Sprintf("embed exit token '%s' can be shadowed in '%s' root lexer by token '%s'", closeToken, moduleAlias, embeddedRule.tokenName),
					closeNode,
				)
				break
			}
		}
	}
}

func validateImportLexicalObligations[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv) {
	if ctx == nil || env == nil {
		return
	}
	definedStates := collectDefinedLexerStates(ctx.RootNode)
	for _, usingRef := range ctx.RootNode.FindAllKind(NodeLexRulePatternUsing) {
		if nodeHasAncestorKind(usingRef, NodeLexRule) {
			continue
		}
		moduleName, symbolName := usingRuleReferenceParts(usingRef)
		if moduleName == "" || symbolName == "" {
			continue
		}
		module, ok := ResolveImportedModule(env, moduleName)
		if !ok || module == nil {
			continue
		}
		obligation := module.ExportedLexicalObligations[symbolName]
		if obligation == nil {
			continue
		}
		for tokenName := range obligation.RequiredTokens {
			if env.Tokens[tokenName] != nil {
				continue
			}
			ctx.ReportError(
				VALIDATION_IMPORT_MISSING_TOKEN_OBLIGATION.String(),
				fmt.Sprintf("imported symbol '%s.%s' requires host token '%s' in LEX", moduleName, symbolName, tokenName),
				usingRef,
			)
		}
		for stateName := range obligation.RequiredStates {
			if definedStates[stateName] {
				continue
			}
			ctx.ReportError(
				VALIDATION_IMPORT_MISSING_STATE_OBLIGATION.String(),
				fmt.Sprintf("imported symbol '%s.%s' requires host lexer state '%s'", moduleName, symbolName, stateName),
				usingRef,
			)
		}
	}
}

func collectDefinedLexerStates(root *Node) map[string]bool {
	out := make(map[string]bool)
	lex := root.FindFirstKind(NodeLexSection)
	if lex == nil {
		return out
	}
	for _, stateList := range lex.FindAllKind(NodeStateList) {
		list := stateList.FindFirstKind(NodeStateDefinitionList)
		if list == nil {
			continue
		}
		for _, stateDef := range list.FindAllKind(NodeStateDefinition) {
			stateName := NodeSingleTokenContent(stateDef)
			if stateName == "" {
				continue
			}
			out[stateName] = true
		}
	}
	return out
}

// validateSymbolNameCollisions ensures no name is declared in more than one symbol table
// (tokens, patterns, parse rules, pairs, pratt). Order for "first" declaration follows SemanticSymbolKind iota.
func validateSymbolNameCollisions[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv) {
	kindName := func(k SemanticSymbolKind) string {
		switch k {
		case SymbolKindToken:
			return "token"
		case SymbolKindPattern:
			return "pattern"
		case SymbolKindRule:
			return "parse rule"
		case SymbolKindPair:
			return "pair"
		case SymbolKindTemplate:
			return "template"
		case SymbolKindPratt:
			return "pratt expression"
		default:
			return "symbol"
		}
	}

	allNames := make(map[string]struct{})
	for name := range env.Tokens {
		allNames[name] = struct{}{}
	}
	for name := range env.Patterns {
		allNames[name] = struct{}{}
	}
	for name := range env.Rules {
		allNames[name] = struct{}{}
	}
	for name := range env.Pairs {
		allNames[name] = struct{}{}
	}
	for name := range env.Templates {
		allNames[name] = struct{}{}
	}
	for name := range env.Pratt {
		allNames[name] = struct{}{}
	}

	for name := range allNames {
		var firstKind SemanticSymbolKind = 255
		if _, ok := env.Tokens[name]; ok {
			firstKind = SymbolKindToken
		}
		if _, ok := env.Patterns[name]; ok {
			if firstKind > SymbolKindPattern {
				firstKind = SymbolKindPattern
			}
		}
		if _, ok := env.Rules[name]; ok {
			if firstKind > SymbolKindRule {
				firstKind = SymbolKindRule
			}
		}
		if _, ok := env.Pairs[name]; ok {
			if firstKind > SymbolKindPair {
				firstKind = SymbolKindPair
			}
		}
		if _, ok := env.Templates[name]; ok {
			if firstKind > SymbolKindTemplate {
				firstKind = SymbolKindTemplate
			}
		}
		if _, ok := env.Pratt[name]; ok {
			if firstKind > SymbolKindPratt {
				firstKind = SymbolKindPratt
			}
		}

		reportOn := func(kind SemanticSymbolKind, node *Node) {
			if node == nil || kind == firstKind {
				return
			}
			msg := fmt.Sprintf("symbol '%s' already declared as %s", name, kindName(firstKind))
			ctx.ReportError(VALIDATION_SYMBOL_NAME_COLLISION.String(), msg, node)
		}
		reportOn(SymbolKindToken, env.Tokens[name])
		reportOn(SymbolKindPattern, env.Patterns[name])
		reportOn(SymbolKindRule, env.Rules[name])
		if decl := env.Pairs[name]; decl != nil {
			reportOn(SymbolKindPair, decl.Node)
		}
		if decl := env.Templates[name]; decl != nil {
			reportOn(SymbolKindTemplate, decl.Node)
		}
		reportOn(SymbolKindPratt, env.Pratt[name])
	}
}

func validateTokenReferences[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv) {
	used := make(map[string]bool)
	ignoredRoles := getIgnoredRoles(ctx)

	markExplicitTokenReferences(ctx, env, used)
	markNestTokenReferences(ctx, env, used)
	markPrattOperatorTargetReferences(ctx, env, used)
	markImportedLexicalObligationTokenReferences(ctx, env, used)
	reportUnusedTokens(ctx, env, used, ignoredRoles)
}

func markImportedLexicalObligationTokenReferences[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, used map[string]bool) {
	if ctx == nil || env == nil {
		return
	}
	for _, usingRef := range ctx.RootNode.FindAllKind(NodeLexRulePatternUsing) {
		if usingRef == nil || nodeHasAncestorKind(usingRef, NodeLexRule) {
			continue
		}
		moduleName, symbolName := usingRuleReferenceParts(usingRef)
		if moduleName == "" || symbolName == "" {
			continue
		}
		module, ok := ResolveImportedModule(env, moduleName)
		if !ok || module == nil {
			continue
		}
		obligation := module.ExportedLexicalObligations[symbolName]
		if obligation == nil {
			continue
		}
		for tokenName := range obligation.RequiredTokens {
			if _, exists := env.Tokens[tokenName]; exists {
				used[tokenName] = true
			}
		}
	}
}

func markExplicitTokenReferences[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, used map[string]bool) {
	parseSec := ctx.RootNode.FindFirstKind(NodeParseSection)
	if parseSec == nil {
		return
	}
	markTokensUnder := func(root *Node) {
		if root == nil {
			return
		}
		root.WalkPre(func(node *Node) (bool, bool) {
			if node != nil && node.Kind() == NodeLexRulePatternUsing {
				moduleName, symbolName := usingRuleReferenceParts(node)
				if moduleName != "" && symbolName != "" {
					module, ok := ResolveImportedModule(env, moduleName)
					if ok && module != nil {
						decl := module.ExportedTemplates[symbolName]
						callArgs := node.FindFirstKind(NodeParseTemplateCallArgs)
						if decl != nil && callArgs != nil {
							for _, tokenName := range staticTokenRefsFromTemplateDeclArgs(callArgs, decl, env) {
								used[tokenName] = true
							}
						}
					}
				}
			}
			if node != nil && node.Kind() == NodeParseSegment && SegmentHasExplicitTemplateInvocation(node) {
				for _, name := range StaticTokenRefsFromTemplateCallSegment(node, env) {
					used[name] = true
				}
			}
			if node.Kind() == NodeParseTokenReference || node.Kind() == NodeParseNestOpenToken || node.Kind() == NodeParseNestCloseToken {
				name := IdentifierValue(node)
				if name == "" {
					return false, false
				}

				kind := resolveParseRefSymbolKind(env, name)
				if kind == parseRefSymbolToken {
					used[name] = true
				} else {
					msg := fmt.Sprintf("cannot map expression '%s' here; a token is required", name)
					ctx.ReportError(VALIDATION_EXPRESSION_REFERENCED_AS_TOKEN_OUTPUT.String(), msg, node)
				}
			}

			return false, false
		})
	}
	markTokensUnder(parseSec)
	for _, tpl := range parseSec.FindAllKind(NodeParseTemplate) {
		if body := tpl.FindFirstKind(NodeParseTemplateBody); body != nil {
			markTokensUnder(body)
		}
	}
}

func markNestTokenReferences[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, used map[string]bool) {
	for _, nestOp := range ctx.RootNode.FindAllKind(NodeParseOpNest) {
		openNode := nestOp.FindFirstKind(NodeParseNestOpenToken)
		closeNode := nestOp.FindFirstKind(NodeParseNestCloseToken)
		if openNode != nil && closeNode != nil {
			validateAndMarkToken(ctx, env, used, openNode)
			validateAndMarkToken(ctx, env, used, closeNode)
			continue
		}
		pairRef := nestOp.FindFirstKind(NodeParseNestPairRef)
		if pairRef == nil {
			continue
		}
		pName := PairNameFromNestPairRefNode(pairRef)
		if pName == "" {
			ctx.ReportError(VALIDATION_PAIR_REF_MALFORMED.String(), "malformed pair reference (expected @Name)", pairRef)
			continue
		}
		decl, ok := env.Pairs[pName]
		if !ok {
			ctx.ReportError(VALIDATION_UNRESOLVED_PAIR_REF.String(), fmt.Sprintf("unresolved pair '%s'", pName), pairRef)
			continue
		}
		markTokenNameUsed(ctx, env, used, decl.OpenToken, pairRef)
		markTokenNameUsed(ctx, env, used, decl.CloseToken, pairRef)
	}
}

func markTokenNameUsed[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, used map[string]bool, tokenName string, refNode *Node) {
	if tokenName == "" {
		return
	}
	if _, exists := env.Tokens[tokenName]; !exists {
		reportUnresolvedToken(ctx, refNode, tokenName)
		return
	}
	used[tokenName] = true
}

func markPrattOperatorTargetReferences[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, used map[string]bool) {
	prattSection := ctx.RootNode.FindFirstKind(NodePrattSection)
	if prattSection == nil {
		return
	}
	for _, def := range prattSection.FindAllKind(NodePrattOperatorDef) {
		ref := def.FindFirstKind(NodeParseSymbolReference)
		if ref == nil {
			continue
		}
		name := RefName(ref)
		if name == "" {
			continue
		}

		kind := resolveParseRefSymbolKind(env, name)
		switch kind {
		case parseRefSymbolToken:
			used[name] = true
		case parseRefSymbolRule, parseRefSymbolPratt:
			// No-op
		default:
			ctx.ReportError(
				VALIDATION_PRATT_UNRESOLVED_OPERATOR_TARGET.String(),
				fmt.Sprintf("unresolved pratt operator target '%s'", name),
				ref,
			)
		}
	}
}

func validateAndMarkToken[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, used map[string]bool, tokenNode *Node) {
	if tokenNode == nil {
		return
	}

	name := IdentifierValue(tokenNode)
	if _, exists := env.Tokens[name]; !exists {
		reportUnresolvedToken(ctx, tokenNode, name)
	} else {
		used[name] = true
	}
}

func reportUnresolvedToken[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], ref *Node, name string) {
	if shouldSkipLibraryUnresolvedChecks(ctx) {
		return
	}
	ctx.ReportError(codeForUnresolvedTokenRef(ref).String(), fmt.Sprintf("unresolved token reference '%s'", name), ref)
}

func isLibraryMode[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) bool {
	return ctx != nil && ctx.RunState != nil && ctx.RunState.LibraryMode
}

func shouldSkipLibraryUnresolvedChecks[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) bool {
	return isLibraryMode(ctx)
}

func reportUnusedTokens[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, used map[string]bool, ignoredRoles map[string]bool) {
	if isLibraryMode(ctx) {
		return
	}
	for name, node := range env.Tokens {
		if used[name] || ignoredRoles[getTokenRole(node)] {
			continue
		}
		ctx.ReportWarning(VALIDATION_TOKEN_UNREFERENCED_IN_PARSE.String(), fmt.Sprintf("token '%s' is never referenced in parse or pratt", name), node)
	}
}

func getIgnoredRoles[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) map[string]bool {
	ignored := make(map[string]bool)
	parse := ctx.RootNode.FindFirstKind(NodeParseSection)
	if parse == nil {
		return ignored
	}

	ignoreSec := parse.FindFirstKind(NodeParseIgnoreSection)
	if ignoreSec == nil {
		return ignored
	}

	for _, roleNode := range ignoreSec.FindAllKind(NodeParseIgnoreRole) {
		ignored[IdentifierValue(roleNode)] = true
	}
	return ignored
}

func getTokenRole(tokenNameNode *Node) string {
	parent := tokenNameNode.Parent()
	if parent == nil || parent.Kind() != NodeLexRule {
		return ""
	}

	roleNode := parent.FindFirstKind(NodeLexRuleRole)
	if roleNode == nil {
		return ""
	}

	return IdentifierValue(roleNode)
}

func validatePatternReferences[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv) {
	for _, ref := range ctx.RootNode.FindAllKind(NodePatternRef) {
		name := PatternRefTargetName(ref)
		if name == "" {
			continue
		}

		if _, exists := env.Patterns[name]; !exists {
			if shouldSkipLibraryUnresolvedChecks(ctx) {
				continue
			}
			ctx.ReportError(codeForUnresolvedPatternRef(ref).String(), fmt.Sprintf("unresolved pattern reference '%s'", name), ref)
		} else {
			if env.LocalPatterns[name] && enclosingPatternDef(ref) == nil {
				ctx.ReportError(VALIDATION_LOCAL_REF_OUTSIDE_SECTION.String(), fmt.Sprintf("local pattern '%s' referenced outside its pattern scope", name), ref)
			}
		}
	}
}

func validateRuleReferences[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv) {
	if isLibraryMode(ctx) {
		if programNode, ok := env.Rules[ProgramRuleName]; ok {
			nameNode := programNode.FindFirstKind(NodeParseRuleName)
			if nameNode == nil {
				nameNode = programNode
			}
			ctx.ReportError(VALIDATION_PROGRAM_RULE_REQUIRED.String(), fmt.Sprintf("library specs must not define entry rule '%s'", ProgramRuleName), nameNode)
		}
	} else {
		if _, ok := env.Rules[ProgramRuleName]; !ok {
			ctx.ReportError(VALIDATION_PROGRAM_RULE_REQUIRED.String(), fmt.Sprintf("parse section must define entry rule '%s'", ProgramRuleName), ctx.RootNode)
		}
	}

	for _, ref := range ctx.RootNode.FindAllKind(NodeParseExpressionReference) {
		validateParseExpressionReference(ctx, env, ref)
	}
	for _, ref := range ctx.RootNode.FindAllKind(NodeParseSymbolReference) {
		if symbolReferenceIsBareParseSegmentGrammarRef(ref) {
			validateBareParseSegmentSymbolReference(ctx, env, ref)
		}
	}
}

func validateParseExpressionReference[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, ref *Node) {
	name := RefName(ref)
	if name == "" {
		return
	}
	kind := resolveParseRefSymbolKind(env, name)
	switch kind {
	case parseRefSymbolRule, parseRefSymbolPratt:
		if kind == parseRefSymbolPratt && env.LocalPratt[name] && enclosingPrattDef(ref) == nil {
			ctx.ReportError(VALIDATION_PRATT_LOCAL_LEAK.String(), fmt.Sprintf("local pratt expression '%s' referenced outside pratt section", name), ref)
		}
	case parseRefSymbolTemplate:
		ctx.ReportError(VALIDATION_TEMPLATE_BARE_REFERENCE.String(),
			fmt.Sprintf("template '%s' must be invoked as call %s(...)", name, name), ref)
	case parseRefSymbolToken:
		ctx.ReportError(VALIDATION_TOKEN_REFERENCED_AS_EXPRESSION.String(), messageTokenNotAllowedBareInParse(name), ref)
	default:
		if shouldSkipLibraryUnresolvedChecks(ctx) {
			return
		}
		ctx.ReportError(codeForUnresolvedParseRef(ref).String(), fmt.Sprintf("unresolved parse or pratt rule reference '%s'", name), ref)
	}
}

/*
validateBareParseSegmentSymbolReference checks NodeParseSymbolReference in a parse segment
without output mapping (= token / = group). A declared token is not valid there: tokens must
use `virtual <tok>` or `<node name> = <token reference>`. Rule/pratt names are valid bare references.
*/
func validateBareParseSegmentSymbolReference[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, ref *Node) {
	name := RefName(ref)
	if name == "" {
		return
	}
	kind := resolveParseRefSymbolKind(env, name)
	switch kind {
	case parseRefSymbolRule, parseRefSymbolPratt:
		if kind == parseRefSymbolPratt && env.LocalPratt[name] && enclosingPrattDef(ref) == nil {
			ctx.ReportError(VALIDATION_PRATT_LOCAL_LEAK.String(), fmt.Sprintf("local pratt expression '%s' referenced outside pratt section", name), ref)
		}
	case parseRefSymbolTemplate:
		ctx.ReportError(VALIDATION_TEMPLATE_BARE_REFERENCE.String(),
			fmt.Sprintf("template '%s' must be invoked as call %s(...)", name, name), ref)
	case parseRefSymbolToken:
		ctx.ReportError(VALIDATION_TOKEN_REFERENCED_AS_EXPRESSION.String(), messageTokenNotAllowedBareInParse(name), ref)
	default:
		if shouldSkipLibraryUnresolvedChecks(ctx) {
			return
		}
		ctx.ReportError(codeForUnresolvedParseRef(ref).String(), fmt.Sprintf("unresolved parse or pratt rule reference '%s'", name), ref)
	}
}

/*
symbolReferenceIsBareParseSegmentGrammarRef is true when ref is the leading identifier of
identifierMapping with no "= …" tail (bare Foo vs Foo = Tok / Foo = ( … )). Without the tail,
Foo is a rule/pratt reference, or (if it names a lexer token) an invalid bare token use.
*/
func symbolReferenceIsBareParseSegmentGrammarRef(ref *Node) bool {
	if ref == nil || ref.Kind() != NodeParseSymbolReference {
		return false
	}
	return ParseSegmentLeadingSymbolRefIsBareGrammarSite(ref.Parent())
}

// ------------------------------------------------------------- REACHABILITY (STAGE 1)

func processReachability[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	var imports map[string]*ImportedModuleSymbols
	if ctx.RunState != nil {
		imports = ctx.RunState.ImportedModules
	}
	env := BuildSemanticEnvWithImports(ctx.RootNode, imports, nil)

	patternDeps := buildPatternDependencyMap(ctx.RootNode, env)
	checkPatternCycles(ctx, env, patternDeps)
	if !isLibraryMode(ctx) {
		checkPatternReachability(ctx, env, patternDeps)
	}

	if !isLibraryMode(ctx) {
		parseDeps := buildUnifiedParseDependencyMap(ctx.RootNode, env)
		checkParseReachability(ctx, env, parseDeps)
	}
}

func checkPatternCycles[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, deps map[string][]string) {
	for name, node := range env.Patterns {
		if cycle := findCycleInPatternDeps(name, deps); cycle != nil {
			_, nameNode := ExtractPatternDefName(node)
			ctx.ReportError(VALIDATION_CYCLIC_PATTERN_REF.String(), fmt.Sprintf("cyclic reference detected: %s", formatCycle(cycle)), nameNode)
		}
	}
}

func checkPatternReachability[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, deps map[string][]string) {
	reachable := computeReachablePatterns(ctx.RootNode, deps)
	for name, node := range env.Patterns {
		if env.LocalPatterns[name] {
			continue
		}
		if isLibraryMode(ctx) && patternDefinitionIsExported(node) {
			// Library modules often export utility patterns intended for importers only.
			// Do not report local-unreachable warnings for exported pattern surface.
			continue
		}
		if !reachable[name] {
			_, nameNode := ExtractPatternDefName(node)
			ctx.ReportWarning(VALIDATION_UNREACHABLE_PATTERN.String(), fmt.Sprintf("pattern '%s' is never referenced (unreachable)", name), nameNode)
		}
	}
}

func patternDefinitionIsExported(def *Node) bool {
	return def != nil && def.FindFirstKind(NodeExported) != nil
}

func checkParseReachability[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], env *SemanticEnv, deps map[string][]string) {
	reachable := computeReachableParseRules(deps, ProgramRuleName)
	if reachable == nil {
		return
	}

	for name, node := range env.Rules {
		if !reachable[name] {
			nameNode := node.FindFirstKind(NodeParseRuleName)
			ctx.ReportWarning(VALIDATION_UNREFERENCED_PARSE_RULE.String(), fmt.Sprintf("parse rule '%s' is never referenced", name), nameNode)
		}
	}
}

// ------------------------------------------------------------- LEX SEMANTICS (STAGE 2)

func processLexerStateSemantics[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	lex := ctx.RootNode.FindFirstKind(NodeLexSection)
	if lex == nil {
		return
	}

	defined := make(map[string]struct{})
	firstDefNode := make(map[string]*Node)

	for _, sl := range lex.FindAllKind(NodeStateList) {
		list := sl.FindFirstKind(NodeStateDefinitionList)
		if list == nil {
			continue
		}
		for _, def := range list.FindAllKind(NodeStateDefinition) {
			n := NodeSingleTokenContent(def)
			if _, dup := defined[n]; dup {
				ctx.ReportError(VALIDATION_LEX_DUPLICATE_STATE_NAME.String(), fmt.Sprintf("duplicate lexer state '%s'", n), def)
				continue
			}
			defined[n] = struct{}{}
			firstDefNode[n] = def
		}
	}

	if _, hasInitial := defined["INITIAL"]; !hasInitial && !isLibraryMode(ctx) {
		ctx.ReportError(VALIDATION_LEX_INITIAL_STATE_MISSING.String(), "lexer must declare state INITIAL", lex)
	}

	referenced := make(map[string]bool)
	for _, sl := range lex.FindAllKind(NodeStateList) {
		body := sl.FindFirstKind(NodeStateDefinitionBody)
		if body == nil {
			continue
		}
		for _, rule := range body.FindAllKind(NodeLexRule) {
			validateLexRuleHasPattern(ctx, rule)
			validateLexerMutations(ctx, rule, defined)
			markReferencedLexerStates(rule, referenced)
		}
	}

	for name := range defined {
		if name == "INITIAL" {
			continue
		}
		if isLibraryMode(ctx) {
			continue
		}
		if !referenced[name] {
			ctx.ReportWarning(VALIDATION_LEX_UNREFERENCED_STATE.String(), fmt.Sprintf("lexer state '%s' is never a target of push or set", name), firstDefNode[name])
		}
	}

	checkLexTokenConsistencyAcrossStates(ctx)
}

func validateLexRuleHasPattern[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], ruleNode *Node) {
	if ruleNode == nil {
		return
	}
	key, _ := extractLexPattern(ruleNode)
	if key != "" {
		return
	}
	tokenNameNode := ruleNode.FindFirstKind(NodeLexRuleTokenName)
	if tokenNameNode == nil {
		tokenNameNode = ruleNode
	}
	ctx.ReportError(
		VALIDATION_LEX_RULE_MISSING_PATTERN.String(),
		"lexer rule must have a regex pattern, a pattern reference, or `using Module.exportedPattern`",
		tokenNameNode,
	)
}

func validateLexerMutations[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], rule *Node, defined map[string]struct{}) {
	mutRoot := rule.FindFirstKind(NodeLexRuleStateMutation)
	if mutRoot == nil {
		return
	}
	if mutRoot.FindFirstKind(NodeStateMutationPush) != nil || mutRoot.FindFirstKind(NodeStateMutationSet) != nil {
		refs := mutRoot.FindAllKind(NodeStateReference)
		if len(refs) == 0 {
			ctx.ReportError(VALIDATION_LEX_PUSH_SET_EMPTY_ARGS.String(), "push/set requires at least one state name", mutRoot)
			return
		}
		for _, ref := range refs {
			name := NodeSingleTokenContent(ref)
			if _, ok := defined[name]; !ok {
				ctx.ReportError(VALIDATION_LEX_UNDEFINED_STATE_REF.String(), fmt.Sprintf("undefined lexer state '%s'", name), ref)
			}
		}
	}
}

func markReferencedLexerStates(rule *Node, referenced map[string]bool) {
	mutRoot := rule.FindFirstKind(NodeLexRuleStateMutation)
	if mutRoot == nil {
		return
	}
	for _, ref := range mutRoot.FindAllKind(NodeStateReference) {
		referenced[NodeSingleTokenContent(ref)] = true
	}
}

func checkLexTokenConsistencyAcrossStates[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	byToken := make(map[string]string)
	for _, info := range collectLexRules(ctx.RootNode) {
		sig := info.patternKey + "|" + info.stackSig
		if prev, ok := byToken[info.tokenName]; ok {
			if prev != sig {
				ctx.ReportError(VALIDATION_LEX_TOKEN_MULTI_STATE_CONFLICT.String(),
					fmt.Sprintf("token '%s' must use the same pattern and [push/pop/set] in every lexer state", info.tokenName),
					info.tokenNameNode)
			}
			continue
		}
		byToken[info.tokenName] = sig
	}
}

func lexerStateDefinitionNames(stateList *Node) []string {
	list := stateList.FindFirstKind(NodeStateDefinitionList)
	if list == nil {
		return nil
	}
	var out []string
	for _, def := range list.FindAllKind(NodeStateDefinition) {
		out = append(out, NodeSingleTokenContent(def))
	}
	return out
}

func processLexSemantics[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	processLexerStateSemantics(ctx)
	lexRules := collectLexRules(ctx.RootNode)
	patternKeyToRules := groupLexRulesByPattern(lexRules)

	for key, rules := range patternKeyToRules {
		if len(rules) < 2 {
			continue
		}
		reportIdenticalAndAmbiguousPatterns(ctx, key, rules)
		reportShadowedTokens(ctx, rules)
	}
}

func reportIdenticalAndAmbiguousPatterns[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], key string, rules []lexRuleInfo) {
	for _, r := range rules {
		msgIden := fmt.Sprintf("token '%s' produces the same pattern as other token(s) (pattern key: %s)", r.tokenName, key)
		ctx.ReportWarning(VALIDATION_IDENTICAL_TOKEN_PATTERN.String(), msgIden, r.tokenNameNode)

		msgAmb := fmt.Sprintf("token '%s' can match the same input as other token(s) (ambiguous)", r.tokenName)
		ctx.ReportWarning(VALIDATION_AMBIGUOUS_TOKEN_MATCH.String(), msgAmb, r.patternNode)
	}
}

func reportShadowedTokens[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], rules []lexRuleInfo) {
	maxPri := rules[0].priority
	for _, r := range rules[1:] {
		if r.priority > maxPri {
			maxPri = r.priority
		}
	}
	for _, r := range rules {
		if r.priority < maxPri {
			msg := fmt.Sprintf("token '%s' is shadowed by higher-priority token(s) with the same pattern", r.tokenName)
			ctx.ReportWarning(VALIDATION_TOKEN_SHADOWED.String(), msg, r.tokenNameNode)
		}
	}
}

// ------------------------------------------------------------- PATTERN SEMANTICS (STAGE 3)

func processPatternSemantics[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	validateNegationNodes(ctx)
	validateRepetitionBounds(ctx)
}

func validateNegationNodes[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	for _, negNode := range ctx.RootNode.FindAllKind(NodePatternNegation) {
		children := negNode.Children()
		if len(children) == 0 {
			continue
		}
		validateNegationSubtree(children[0], func(offending *Node) {
			ctx.ReportError(VALIDATION_NEGATION_INVALID_CONTENT.String(), "negation (!) may only contain character, range, group, class, or alternation", offending)
		})
	}
}

func validateRepetitionBounds[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	for _, boundsNode := range ctx.RootNode.FindAllKind(NodeRepetitionBounds) {
		minNode := boundsNode.FindFirstKind(NodeRepetitionMin)
		maxNode := boundsNode.FindFirstKind(NodeRepetitionMax)

		minVal, minOK := parseIntFromNode(minNode)
		maxVal, maxOK := parseIntFromNode(maxNode)

		if minNode == nil && maxNode == nil {
			ctx.ReportError(VALIDATION_REPETITION_BOUNDS_EMPTY.String(),
				"repetition bounds must contain at least one integer (examples: `{3}`, `{1,4}`)", boundsNode)
			continue
		}

		if minOK && minVal < 0 {
			ctx.ReportError(VALIDATION_REPETITION_NEGATIVE_BOUND.String(),
				fmt.Sprintf("repetition minimum must be non-negative (got %d)", minVal), boundsNode)
			continue
		}
		if maxOK && maxVal < 0 {
			ctx.ReportError(VALIDATION_REPETITION_NEGATIVE_BOUND.String(),
				fmt.Sprintf("repetition maximum must be non-negative (got %d)", maxVal), boundsNode)
			continue
		}

		if minOK && maxOK && minVal > maxVal && maxVal != -1 {
			ctx.ReportError(VALIDATION_REPETITION_MIN_GT_MAX.String(),
				fmt.Sprintf("repetition range is invalid: minimum (%d) is greater than maximum (%d)", minVal, maxVal), boundsNode)
		}
	}
}

// ------------------------------------------------------------- GRAMMAR SEMANTICS (POST-COMPILATION) (STAGE 4)

func processGrammarSafety[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind]) {
	if ctx.RunState == nil || ctx.RunState.Package == nil {
		ctx.ReportFatal("V_INTERNAL", "engine error: grammar package missing in validation run state", ctx.RootNode)
		return
	}
	pkg := ctx.RunState.Package
	sourceMap := ctx.RunState.SourceMap

	analysis := lowering.GetAnalysis(pkg)
	if analysis == nil {
		ctx.ReportFatal("V_INTERNAL", "engine error: failed to generate grammar analysis", ctx.RootNode)
		return
	}

	checkGrammarLeftRecursion(ctx, pkg, analysis)
	checkGrammarUnboundedOptional(ctx, pkg, analysis)
	checkGrammarSeparatorRepeatOptionalTail[TNodeKind](ctx, pkg, analysis, sourceMap)
	checkGrammarChoiceConflicts[TNodeKind](ctx, pkg, analysis, sourceMap)
	checkGrammarChoicePrefixTraps[TNodeKind](ctx, pkg, analysis, sourceMap)
}

func checkGrammarLeftRecursion[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], pkg *GrammarPackage[TNodeKind], analysis *syntaxa.GrammarAnalysis) {
	parseSection := ctx.RootNode.FindFirstKind(NodeParseSection)

	for ruleName, rootGrammar := range pkg.Grammars {
		if hasLeftRecursion[TNodeKind](rootGrammar, pkg, analysis, make(map[syntaxa.GrammarLabel]bool), ruleName) {
			ruleNode := findParseRuleByName(parseSection, string(ruleName))
			nodeToReport := ctx.RootNode
			if ruleNode != nil {
				if body := ruleNode.FindFirstKind(NodeParseRuleBody); body != nil {
					nodeToReport = body
				}
			}
			msg := fmt.Sprintf("parse rule '%s' is left-recursive; top-down parser will infinite loop", ruleName)
			ctx.ReportFatal(VALIDATION_PARSE_LEFT_RECURSION.String(), msg, nodeToReport)
		}
	}
}

func hasLeftRecursion[TNodeKind ~uint32](g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind], pkg *GrammarPackage[TNodeKind], analysis *syntaxa.GrammarAnalysis, visited map[syntaxa.GrammarLabel]bool, target syntaxa.GrammarLabel) bool {
	if g == nil {
		return false
	}
	switch g.Kind {
	case syntaxa.GReference:
		if g.ReferenceTarget == target {
			return true
		}
		if visited[g.ReferenceTarget] {
			return false // Prevent infinite loop in mutually recursive non-target rules
		}
		visited[g.ReferenceTarget] = true
		targetRule := pkg.Grammars[g.ReferenceTarget]
		res := hasLeftRecursion[TNodeKind](targetRule, pkg, analysis, visited, target)
		visited[g.ReferenceTarget] = false
		return res
	case syntaxa.GToken:
		return false
	case syntaxa.GNest:
		return false // Open token consumes input immediately
	case syntaxa.GConcat:
		for _, child := range g.Children {
			if hasLeftRecursion[TNodeKind](child, pkg, analysis, visited, target) {
				return true
			}
			// If child is not nullable, it consumes input; we can't left-recurse past it.
			if child.NodePath != nil {
				if !analysis.Nullable[syntaxa.NodeKeyFromPath(*child.NodePath)] {
					break
				}
			}
		}
		return false
	case syntaxa.GChoice:
		for _, child := range g.Children {
			if hasLeftRecursion[TNodeKind](child, pkg, analysis, visited, target) {
				return true
			}
		}
		return false
	case syntaxa.GRepeat, syntaxa.GOptional:
		if len(g.Children) > 0 {
			return hasLeftRecursion[TNodeKind](g.Children[0], pkg, analysis, visited, target)
		}
		return false
	}
	return false
}

func checkGrammarUnboundedOptional[TNodeKind ~uint32](ctx *ValidationCtx[TNodeKind], pkg *GrammarPackage[TNodeKind], analysis *syntaxa.GrammarAnalysis) {
	parseSection := ctx.RootNode.FindFirstKind(NodeParseSection)
	visited := make(map[syntaxa.GrammarKey]bool)

	var walk func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind])
	walk = func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) {
		if g == nil || g.NodePath == nil || visited[g.GrammarKey] {
			return
		}
		visited[g.GrammarKey] = true

		if g.Kind == syntaxa.GRepeat && g.Max == nil {
			if len(g.Children) > 0 && g.Children[0].NodePath != nil {
				childKey := syntaxa.NodeKeyFromPath(*g.Children[0].NodePath)
				if analysis.Nullable[childKey] {
					ruleName := pkg.PathToGrammarLabel[syntaxa.NodeKeyFromPath(*g.NodePath)]
					ruleNode := findParseRuleByName(parseSection, string(ruleName))
					nodeToReport := ctx.RootNode
					if ruleNode != nil {
						nodeToReport = ruleNode
					}
					msg := fmt.Sprintf("unbounded repetition of an optional or nullable expression causes an infinite parser loop in rule '%s'", ruleName)
					ctx.ReportError(VALIDATION_PARSE_UNBOUNDED_OPTIONAL_REPETITION.String(), msg, nodeToReport)
				}
			}
		}
		for _, child := range g.Children {
			walk(child)
		}
	}

	for _, root := range pkg.Grammars {
		walk(root)
	}
}

func checkGrammarSeparatorRepeatOptionalTail[TNodeKind ~uint32](
	ctx *ValidationCtx[TNodeKind],
	pkg *GrammarPackage[TNodeKind],
	analysis *syntaxa.GrammarAnalysis,
	sourceMap map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*Node,
) {
	visited := make(map[syntaxa.GrammarKey]bool)

	var walk func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind])
	walk = func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) {
		if g == nil || g.NodePath == nil || visited[g.GrammarKey] {
			return
		}
		visited[g.GrammarKey] = true

		if g.Kind == syntaxa.GConcat {
			for i := 0; i < len(g.Children)-1; i++ {
				repeatNode := g.Children[i]
				tailNode := g.Children[i+1]
				if !isSeparatorLeadingStarRepeat[TNodeKind](repeatNode) || !isOptionalOrNullableNode[TNodeKind](tailNode, analysis) {
					continue
				}

				anchor := resolveFirstConflictAnchor[TNodeKind](ctx, sourceMap, g, repeatNode)
				if anchor == nil {
					anchor = ctx.RootNode
				}
				ruleName := pkg.PathToGrammarLabel[syntaxa.NodeKeyFromPath(*g.NodePath)]

				msg := fmt.Sprintf(
					"parser-commit sensitive pattern in rule '%s': separator-leading unbounded repetition followed by optional/nullable tail mis-handles trailing separators in a strict LL(1) sequence; prefer 'element (separator element?)*' style",
					ruleName,
				)
				ctx.ReportWarning(VALIDATION_SEPARATOR_REPEAT_OPTIONAL_TAIL.String(), msg, anchor)
			}
		}

		for _, child := range g.Children {
			walk(child)
		}
	}

	for _, root := range pkg.Grammars {
		walk(root)
	}
}

func isSeparatorLeadingStarRepeat[TNodeKind ~uint32](g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) bool {
	if g == nil || g.Kind != syntaxa.GRepeat || g.Max != nil || len(g.Children) != 1 {
		return false
	}
	body := g.Children[0]
	if body == nil || body.Kind != syntaxa.GConcat || len(body.Children) < 2 {
		return false
	}
	first := body.Children[0]
	return first != nil && first.Kind == syntaxa.GToken
}

func isOptionalOrNullableNode[TNodeKind ~uint32](g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind], analysis *syntaxa.GrammarAnalysis) bool {
	if g == nil {
		return false
	}
	if g.Kind == syntaxa.GOptional {
		return true
	}
	if analysis == nil || g.NodePath == nil {
		return false
	}
	return analysis.Nullable[syntaxa.NodeKeyFromPath(*g.NodePath)]
}

const choiceOverlapMaxPrefixDepth = 4
const choiceOverlapMaxPrefixEnumerations = 8192

type choiceOverlapKind uint8

const (
	choiceOverlapResolved choiceOverlapKind = iota
	choiceOverlapOrderedWarning
	choiceOverlapLegacyError
)

/*
classifyChoiceOverlapForToken simulates Syntaxa choice dispatch over bounded peek prefixes
starting with conflictToken (peek(0)). Returns choiceOverlapResolved when every simulated
prefix leaves at most one candidate arm; choiceOverlapOrderedWarning when some prefix
leaves multiple candidates (PEG / ordered choice applies); choiceOverlapLegacyError when
simulation cannot run (missing token vocabulary) or analysis is unusable.
*/
func classifyChoiceOverlapForToken[TNodeKind ~uint32](
	pkg *GrammarPackage[TNodeKind],
	analysis *syntaxa.GrammarAnalysis,
	choiceNode *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
	conflictToken lexarch.TokenKind,
) choiceOverlapKind {
	if analysis == nil || len(pkg.TokensUsed) == 0 {
		return choiceOverlapLegacyError
	}

	maxOff := maxGuardPeekOffsetForChoice[TNodeKind](analysis, choiceNode)
	K := maxOff + 1
	if K < 1 {
		K = 1
	}
	if K > choiceOverlapMaxPrefixDepth {
		K = choiceOverlapMaxPrefixDepth
	}

	extended := extendedTokenAlphabet(pkg.TokensUsed)
	for {
		if K <= 1 {
			break
		}
		prod := 1
		for j := 0; j < K-1; j++ {
			prod *= len(extended)
		}
		if prod <= choiceOverlapMaxPrefixEnumerations {
			break
		}
		K--
	}

	arms := choiceNode.Children
	prefix := make([]lexarch.TokenKind, K)
	prefix[0] = conflictToken

	multi := false
	unmodelled := false

	var visit func(pos int)
	visit = func(pos int) {
		if unmodelled {
			return
		}
		if pos == K {
			peekFn := func(n int) syntaxa.Lexeme {
				if n >= 0 && n < len(prefix) {
					return syntaxa.Lexeme{Token: prefix[n]}
				}
				return syntaxa.Lexeme{Token: lexarch.TokenKindEOF}
			}
			cands, mode := syntaxa.ChoiceCandidateIndicesFromAnalysis(analysis, arms, peekFn)
			if mode == syntaxa.ChoiceDispatchFallback {
				unmodelled = true
				return
			}
			if len(cands) > 1 {
				multi = true
			}
			return
		}
		for _, tk := range extended {
			prefix[pos] = tk
			visit(pos + 1)
			if unmodelled {
				return
			}
		}
	}
	if K == 1 {
		peekFn := func(n int) syntaxa.Lexeme {
			if n == 0 {
				return syntaxa.Lexeme{Token: conflictToken}
			}
			return syntaxa.Lexeme{Token: lexarch.TokenKindEOF}
		}
		cands, mode := syntaxa.ChoiceCandidateIndicesFromAnalysis(analysis, arms, peekFn)
		if mode == syntaxa.ChoiceDispatchFallback {
			return choiceOverlapLegacyError
		}
		if len(cands) > 1 {
			return choiceOverlapOrderedWarning
		}
		return choiceOverlapResolved
	}
	visit(1)
	if unmodelled {
		return choiceOverlapLegacyError
	}
	if multi {
		return choiceOverlapOrderedWarning
	}
	return choiceOverlapResolved
}

func maxGuardPeekOffsetForChoice[TNodeKind ~uint32](
	analysis *syntaxa.GrammarAnalysis,
	choiceNode *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
) int {
	maxOff := 0
	for _, c := range choiceNode.Children {
		if c == nil || c.NodePath == nil {
			continue
		}
		key := syntaxa.NodeKeyFromPath(*c.NodePath)
		g := guardForChoiceArm[TNodeKind](analysis, key, c)
		for _, l := range g {
			if l.Offset > maxOff {
				maxOff = l.Offset
			}
		}
	}
	return maxOff
}

func extendedTokenAlphabet(tokens []lexarch.TokenKind) []lexarch.TokenKind {
	seen := make(map[lexarch.TokenKind]struct{}, len(tokens)+1)
	for _, t := range tokens {
		seen[t] = struct{}{}
	}
	seen[lexarch.TokenKindEOF] = struct{}{}
	out := make([]lexarch.TokenKind, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func checkGrammarChoiceConflicts[TNodeKind ~uint32](
	ctx *ValidationCtx[TNodeKind],
	pkg *GrammarPackage[TNodeKind],
	analysis *syntaxa.GrammarAnalysis,
	sourceMap map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*Node,
) {
	visited := make(map[syntaxa.GrammarKey]bool)

	var walk func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind])
	walk = func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) {
		if g == nil || g.NodePath == nil || visited[g.GrammarKey] {
			return
		}
		visited[g.GrammarKey] = true

		if g.Kind == syntaxa.GChoice {
			analyzeChoiceNode[TNodeKind](ctx, pkg, analysis, sourceMap, g)
		}

		for _, child := range g.Children {
			walk(child)
		}
	}

	for _, root := range pkg.Grammars {
		walk(root)
	}
}

func analyzeChoiceNode[TNodeKind ~uint32](
	ctx *ValidationCtx[TNodeKind],
	pkg *GrammarPackage[TNodeKind],
	analysis *syntaxa.GrammarAnalysis,
	sourceMap map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*Node,
	choiceNode *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
) {
	seenTokens := make(map[lexarch.TokenKind]int)
	reportedPrev := make(map[lexarch.TokenKind]bool)
	tokenOverlapClass := make(map[lexarch.TokenKind]choiceOverlapKind)
	orderedWarnReported := make(map[lexarch.TokenKind]bool)
	sym := ctx.RunState.Symbols

	ruleName := pkg.PathToGrammarLabel[syntaxa.NodeKeyFromPath(*choiceNode.NodePath)]

	for i, child := range choiceNode.Children {
		if child == nil || child.NodePath == nil {
			continue
		}

		childKey := syntaxa.NodeKeyFromPath(*child.NodePath)

		for token := range firstSetForChoiceArm(analysis, childKey) {
			prevBranch, exists := seenTokens[token]
			if !exists {
				seenTokens[token] = i
				continue
			}

			prevChild := choiceNode.Children[prevBranch]
			if prevChild == nil || prevChild.NodePath == nil {
				continue
			}
			prevKey := syntaxa.NodeKeyFromPath(*prevChild.NodePath)
			g1 := guardForChoiceArm[TNodeKind](analysis, prevKey, prevChild)
			g2 := guardForChoiceArm[TNodeKind](analysis, childKey, child)
			if syntaxa.GuardsMutuallyExclusive(g1, g2) {
				continue
			}

			if _, ok := tokenOverlapClass[token]; !ok {
				tokenOverlapClass[token] = classifyChoiceOverlapForToken(pkg, analysis, choiceNode, token)
			}
			switch tokenOverlapClass[token] {
			case choiceOverlapResolved:
				continue
			case choiceOverlapOrderedWarning:
				if !orderedWarnReported[token] {
					msg := fmt.Sprintf(
						"Rule '%s': token '%s' matches more than one ordered choice arm for some peek prefixes; alternatives are tried in source order (earlier alternative wins on success). Add disjoint `predict` or reorder if a later arm should take precedence.",
						ruleName, formatCompiledToken(sym, token),
					)
					anchor := resolveFirstConflictAnchor[TNodeKind](ctx, sourceMap, choiceNode, child)
					ctx.ReportWarning(VALIDATION_ORDERED_CHOICE_OVERLAP.String(), msg, anchor)
					orderedWarnReported[token] = true
				}
				continue
			case choiceOverlapLegacyError:
				// fall through to V_PAR009
			}

			tokenLabel := formatCompiledToken(sym, token)
			note := buildFirstSetConflictNote(sym, token, prevBranch, i, g1, g2)

			if !reportedPrev[token] {
				prevNode := resolveFirstConflictAnchor[TNodeKind](ctx, sourceMap, choiceNode, prevChild)
				msg := fmt.Sprintf(
					"FIRST-set conflict in rule '%s'. Token '%s' is ambiguous; it is also expected by a later branch (%d).%s",
					ruleName, tokenLabel, i, note,
				)
				ctx.ReportError(VALIDATION_FIRST_SET_CONFLICT.String(), msg, prevNode)
				reportedPrev[token] = true
			}

			currNode := resolveFirstConflictAnchor[TNodeKind](ctx, sourceMap, choiceNode, child)
			msg := fmt.Sprintf(
				"FIRST-set conflict in rule '%s'. Token '%s' is ambiguous; it is already expected by an earlier branch (%d).%s",
				ruleName, tokenLabel, prevBranch, note,
			)
			ctx.ReportError(VALIDATION_FIRST_SET_CONFLICT.String(), msg, currNode)
		}
	}
}

func checkGrammarChoicePrefixTraps[TNodeKind ~uint32](
	ctx *ValidationCtx[TNodeKind],
	pkg *GrammarPackage[TNodeKind],
	analysis *syntaxa.GrammarAnalysis,
	sourceMap map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*Node,
) {
	visited := make(map[syntaxa.GrammarKey]bool)
	var walk func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind])
	walk = func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) {
		if g == nil || g.NodePath == nil || visited[g.GrammarKey] {
			return
		}
		visited[g.GrammarKey] = true
		if g.Kind == syntaxa.GChoice {
			analyzeChoicePrefixTrap[TNodeKind](ctx, pkg, analysis, sourceMap, g)
		}
		for _, child := range g.Children {
			walk(child)
		}
	}
	for _, root := range pkg.Grammars {
		walk(root)
	}
}

/*
analyzeChoicePrefixTrap warns when an earlier arm is a concat whose first symbol is the same
rule R as a later arm that is only R. PEG/ordered choice commits after a successful sub-parse
of R inside the longer arm; it does not backtrack to try the shorter arm.

Mitigations: disjoint predict(...) on the longer arm (look past the shared prefix), or order
the shorter alternative first.
*/
func analyzeChoicePrefixTrap[TNodeKind ~uint32](
	ctx *ValidationCtx[TNodeKind],
	pkg *GrammarPackage[TNodeKind],
	analysis *syntaxa.GrammarAnalysis,
	sourceMap map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*Node,
	choiceNode *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
) {
	children := choiceNode.Children
	ruleName := pkg.PathToGrammarLabel[syntaxa.NodeKeyFromPath(*choiceNode.NodePath)]
	for i := 0; i < len(children); i++ {
		for j := i + 1; j < len(children); j++ {
			longArm := children[i]
			shortArm := children[j]
			if longArm == nil || shortArm == nil || longArm.NodePath == nil || shortArm.NodePath == nil {
				continue
			}
			// The shorter arm must be a direct rule reference (e.g. `STRING_LITERAL`). Do not unwrap
			// into the callee body here — that body is often a GChoice (e.g. single vs double quote).
			if shortArm.Kind != syntaxa.GReference || shortArm.ReferenceTarget == "" {
				continue
			}

			longBody := grammarUnwrapRef[TNodeKind](longArm, pkg)
			if longBody == nil {
				continue
			}
			if longBody.Kind != syntaxa.GConcat || len(longBody.Children) < 2 {
				continue
			}
			first := longBody.Children[0]
			if first == nil || first.Kind != syntaxa.GReference || first.ReferenceTarget == "" {
				continue
			}
			if first.ReferenceTarget != shortArm.ReferenceTarget {
				continue
			}
			// Require non-empty continuation after the shared leading rule: either three or more
			// concat operands (e.g. lit .. lit), or a analyzed non-nullable suffix (two operands).
			if len(longBody.Children) < 3 && !concatTailNonNullable[TNodeKind](longBody, analysis) {
				continue
			}

			ruleLabel := string(first.ReferenceTarget)
			msg := fmt.Sprintf(
				"Rule '%s': arm %d starts with '%s' but requires more input after it; arm %d is only '%s'. Ordered choice tries arm %d first — after a successful '%s' sub-parse, failure is a syntax error (no backtrack to arm %d). Add disjoint `predict (...)` lookaheads on arm %d (tokens after the shared prefix), or move arm %d before arm %d.",
				ruleName, i+1, ruleLabel, j+1, ruleLabel, i+1, ruleLabel, j+1, i+1, j+1, i+1,
			)
			anchor := resolveFirstConflictAnchor[TNodeKind](ctx, sourceMap, choiceNode, longArm)
			ctx.ReportWarning(VALIDATION_ORDERED_CHOICE_PREFIX_TRAP.String(), msg, anchor)
		}
	}
}

func grammarUnwrapRef[TNodeKind ~uint32](g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind], pkg *GrammarPackage[TNodeKind]) *syntaxa.Grammar[lexarch.TokenKind, TNodeKind] {
	if g == nil {
		return nil
	}
	if g.Kind == syntaxa.GReference && g.ResolvedReference != nil {
		return g.ResolvedReference
	}
	if g.Kind == syntaxa.GReference && g.ReferenceTarget != "" {
		if body, ok := pkg.Grammars[g.ReferenceTarget]; ok {
			return body
		}
	}
	return g
}

func concatTailNonNullable[TNodeKind ~uint32](concat *syntaxa.Grammar[lexarch.TokenKind, TNodeKind], analysis *syntaxa.GrammarAnalysis) bool {
	if concat == nil || concat.Kind != syntaxa.GConcat || len(concat.Children) < 2 || analysis == nil {
		return false
	}
	for _, ch := range concat.Children[1:] {
		if ch == nil {
			continue
		}
		if ch.NodePath == nil {
			// Virtual tokens / synthetic fragments often have no analysis entry; treat as
			// required continuation (otherwise tails like `virtual TokRange` are skipped).
			return true
		}
		k := syntaxa.NodeKeyFromPath(*ch.NodePath)
		if !analysis.Nullable[k] {
			return true
		}
	}
	return false
}

func firstSetForChoiceArm(analysis *syntaxa.GrammarAnalysis, childKey syntaxa.NodeKey) syntaxa.TokenSet {
	if analysis == nil {
		return nil
	}
	if analysis.ArmPredict != nil {
		if arm, ok := analysis.ArmPredict[childKey]; ok && len(arm.First) > 0 {
			return arm.First
		}
	}
	return analysis.First[childKey]
}

func guardForChoiceArm[TNodeKind ~uint32](
	analysis *syntaxa.GrammarAnalysis,
	childKey syntaxa.NodeKey,
	child *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
) []syntaxa.Lookahead[lexarch.TokenKind] {
	if analysis != nil && analysis.ArmPredict != nil {
		if arm, ok := analysis.ArmPredict[childKey]; ok && len(arm.Guard) > 0 {
			return arm.Guard
		}
	}
	if child != nil {
		if len(child.Lookaheads) > 0 {
			return child.Lookaheads
		}
		if child.Kind == syntaxa.GReference && child.ResolvedReference != nil {
			if la := syntaxa.FirstLookaheadsInSubtreeBFS(child.ResolvedReference); len(la) > 0 {
				return la
			}
		}
	}
	return nil
}

func formatCompiledToken(sym *CompiledSymbolTable, id lexarch.TokenKind) string {
	if sym != nil {
		return sym.TokenName(uint32(id))
	}
	return strconv.FormatUint(uint64(id), 10)
}

func formatPredictGuardList(sym *CompiledSymbolTable, g []syntaxa.Lookahead[lexarch.TokenKind]) string {
	if len(g) == 0 {
		return ""
	}
	parts := make([]string, len(g))
	for i := range g {
		parts[i] = fmt.Sprintf("%d:%s", g[i].Offset, formatCompiledToken(sym, g[i].Expected))
	}
	return strings.Join(parts, ", ")
}

func sortedPeekOffsetKeys(ma, mb map[int]lexarch.TokenKind) []int {
	seen := make(map[int]struct{})
	for o := range ma {
		seen[o] = struct{}{}
	}
	for o := range mb {
		seen[o] = struct{}{}
	}
	keys := make([]int, 0, len(seen))
	for o := range seen {
		keys = append(keys, o)
	}
	sort.Ints(keys)
	return keys
}

/*
buildFirstSetConflictNote explains why predict/lookahead did not clear the overlap.
Static checking only treats alternatives as disjoint when guards contradict at some shared peek offset.
*/
func buildFirstSetConflictNote(
	sym *CompiledSymbolTable,
	conflictToken lexarch.TokenKind,
	altA, altB int,
	gA, gB []syntaxa.Lookahead[lexarch.TokenKind],
) string {
	ga := formatPredictGuardList(sym, gA)
	gb := formatPredictGuardList(sym, gB)
	overlapLabel := formatCompiledToken(sym, conflictToken)

	var b strings.Builder
	b.WriteString(" Static check: `predict` clears this only when two branches require different tokens at the same peek offset.")
	b.WriteString(fmt.Sprintf(
		" Token '%s' belongs to both FIRST sets; that overlap is tied to peek(0) for the first consumed token.",
		overlapLabel,
	))

	ma, badA := syntaxa.GuardPeekConstraints(gA)
	mb, badB := syntaxa.GuardPeekConstraints(gB)
	if badA || badB {
		b.WriteString(" At least one alternative lists two different tokens for the same peek offset (self-contradictory `predict`); exclusivity treats that guard as unusable.")
	}

	keys := sortedPeekOffsetKeys(ma, mb)
	if len(keys) > 0 {
		parts := make([]string, 0, len(keys))
		for _, o := range keys {
			ta, aOk := ma[o]
			tb, bOk := mb[o]
			switch {
			case aOk && bOk && ta == tb:
				parts = append(parts, fmt.Sprintf(
					"peek(%d) fixes %s on both alternatives (no contradiction).",
					o, formatCompiledToken(sym, ta),
				))
			case aOk && bOk && ta != tb:
				parts = append(parts, fmt.Sprintf(
					"peek(%d): alternative %d requires %s, alternative %d requires %s.",
					o, altA, formatCompiledToken(sym, ta), altB, formatCompiledToken(sym, tb),
				))
			case aOk && !bOk:
				parts = append(parts, fmt.Sprintf(
					"peek(%d): only alternative %d requires %s; alternative %d does not fix this offset.",
					o, altA, formatCompiledToken(sym, ta), altB,
				))
			case !aOk && bOk:
				parts = append(parts, fmt.Sprintf(
					"peek(%d): only alternative %d requires %s; alternative %d does not fix this offset.",
					o, altB, formatCompiledToken(sym, tb), altA,
				))
			}
		}
		b.WriteString(" ")
		b.WriteString(strings.Join(parts, " "))
	}

	switch {
	case ga == "" && gb == "":
		b.WriteString(fmt.Sprintf(
			" Here alternative %d and %d have no `predict (...)` on this overlap; add disjoint lookaheads or refactor so leading tokens differ.",
			altA, altB,
		))
	case ga != "" && gb == "":
		b.WriteString(fmt.Sprintf(
			" Alternative %d has `predict (%s)` but alternative %d has none, so any input starting with this token still counts as overlapping the guarded branch unless you add a contradicting `predict` on %d, reorder, or refactor.",
			altA, ga, altB, altB,
		))
	case ga == "" && gb != "":
		b.WriteString(fmt.Sprintf(
			" Alternative %d has `predict (%s)` but alternative %d has none (same reasoning as above, roles swapped).",
			altB, gb, altA,
		))
	default:
		if len(keys) == 0 {
			b.WriteString(fmt.Sprintf(
				" Alternatives %d and %d use `predict (%s)` vs `predict (%s)` but they do not contradict at any single offset (same offset must force different tokens).",
				altA, altB, ga, gb,
			))
		}
	}
	return b.String()
}

/*
resolveFirstConflictAnchor picks an LST node for FIRST-set conflict diagnostics.

The compiler records sourceMap[GChoice] = NodeParseAlternation in compileAlternation (inner
choice before TransparentSequence) and maps each compiled arm subtree to its LST node via
compileParseExpression.

Resolution order: prefer the conflicting alternative’s arm grammar node (pinpoints the rule
expression that overlaps), then the whole alternation if the arm has no mapped/usable span,
then root. This avoids reporting the entire `|` twice when both branches conflict.
*/
func resolveFirstConflictAnchor[TNodeKind ~uint32](
	ctx *ValidationCtx[TNodeKind],
	sourceMap map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*Node,
	choiceRoot *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
	arm *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
) *Node {
	if arm != nil {
		if n := sourceMap[arm]; n != nil && syntaxa.LSTNodeHasMergedByteSpan(n) {
			return n
		}
	}
	if choiceRoot != nil {
		if n := sourceMap[choiceRoot]; n != nil && syntaxa.LSTNodeHasMergedByteSpan(n) {
			return n
		}
	}
	if arm != nil {
		if n := sourceMap[arm]; n != nil {
			return n
		}
	}
	if choiceRoot != nil {
		if n := sourceMap[choiceRoot]; n != nil {
			return n
		}
	}
	return ctx.RootNode
}

// ------------------------------------------------------------- HELPERS

type parseRefSymbolKind uint8

const (
	parseRefSymbolNone parseRefSymbolKind = iota
	parseRefSymbolToken
	parseRefSymbolRule
	parseRefSymbolTemplate
	parseRefSymbolPratt
)

func resolveParseRefSymbolKind(env *SemanticEnv, name string) parseRefSymbolKind {
	if name == "" {
		return parseRefSymbolNone
	}
	if _, ok := env.Tokens[name]; ok {
		return parseRefSymbolToken
	}
	if _, ok := env.Rules[name]; ok {
		return parseRefSymbolRule
	}
	if _, ok := env.Templates[name]; ok {
		return parseRefSymbolTemplate
	}
	if _, ok := env.Pratt[name]; ok {
		return parseRefSymbolPratt
	}
	return parseRefSymbolNone
}

func messageTokenNotAllowedBareInParse(name string) string {
	return fmt.Sprintf(
		"token '%s' cannot appear bare in a parse expression; use `virtual %s` or an output mapping (`<node name> : %s`)",
		name, name, name,
	)
}

func codeForUnresolvedParseRef(ref *Node) ValidationCode {
	if enclosingPrattDef(ref) != nil {
		return VALIDATION_PRATT_UNRESOLVED_RULE
	}
	return VALIDATION_UNRESOLVED_PARSE_RULE_REF
}

func codeForUnresolvedPatternRef(ref *Node) ValidationCode {
	if enclosingPrattDef(ref) != nil {
		return VALIDATION_PRATT_UNRESOLVED_PATTERN
	}
	return VALIDATION_UNRESOLVED_PATTERN_REF
}

func codeForUnresolvedTokenRef(ref *Node) ValidationCode {
	if enclosingPrattDef(ref) != nil {
		return VALIDATION_PRATT_UNRESOLVED_TOKEN
	}
	return VALIDATION_UNRESOLVED_TOKEN_REF
}

func enclosingPatternDef(node *Node) *Node {
	for n := node; n != nil; n = n.Parent() {
		if n.Kind() == NodePatternDefinition {
			return n
		}
	}
	return nil
}

func enclosingPrattDef(node *Node) *Node {
	for n := node; n != nil; n = n.Parent() {
		if n.Kind() == NodePrattExprDef {
			return n
		}
	}
	return nil
}

func buildPatternDependencyMap(root *Node, env *SemanticEnv) map[string][]string {
	out := make(map[string][]string)
	for _, def := range root.FindAllKind(NodePatternDefinition) {
		name, _ := ExtractPatternDefName(def)
		if name == "" {
			continue
		}

		var refs []string
		for _, varRef := range def.FindAllKind(NodePatternRef) {
			refName := PatternRefTargetName(varRef)
			if refName != "" && env.Patterns[refName] != nil {
				refs = append(refs, refName)
			}
		}
		out[name] = refs
	}
	return out
}

func buildUnifiedParseDependencyMap(root *Node, env *SemanticEnv) map[string][]string {
	out := make(map[string][]string)

	parseSection := root.FindFirstKind(NodeParseSection)
	if parseSection != nil {
		for _, rule := range parseSection.FindAllKind(NodeParseRule) {
			name := ParseRuleName(rule.FindFirstKind(NodeParseRuleName))
			if name != "" {
				out[name] = extractAllRuleRefs(rule, env)
			}
		}
	}

	prattSection := root.FindFirstKind(NodePrattSection)
	if prattSection != nil {
		for _, prattDef := range prattSection.FindAllKind(NodePrattExprDef) {
			name := IdentifierValue(prattDef.FindFirstKind(NodePrattExprName))
			if name != "" {
				out[name] = extractAllRuleRefs(prattDef, env)
			}
		}
	}

	return out
}

/*
ruleRefsFromLexRulePatternUsing collects parse-rule / Pratt edges from a `using Module.symbol(...)` site:
imported exported parse rules are walked; imported exported templates contribute Rule/PrattExpr args only.
*/
func ruleRefsFromLexRulePatternUsing(usingNode *Node, env *SemanticEnv) []string {
	if usingNode == nil || env == nil || usingNode.Kind() != NodeLexRulePatternUsing {
		return nil
	}
	moduleName, symbolName := usingRuleReferenceParts(usingNode)
	if moduleName == "" || symbolName == "" {
		return nil
	}
	module, ok := ResolveImportedModule(env, moduleName)
	if !ok || module == nil {
		return nil
	}
	if importedRule := module.ExportedRules[symbolName]; importedRule != nil {
		return extractImportedRuleRefs(importedRule, env)
	}
	if decl := module.ExportedTemplates[symbolName]; decl != nil {
		callArgs := usingNode.FindFirstKind(NodeParseTemplateCallArgs)
		return staticRuleAndPrattRefsFromTemplateDeclArgs(callArgs, decl, env)
	}
	return nil
}

func extractAllRuleRefs(container *Node, env *SemanticEnv) []string {
	var refs []string

	container.WalkPre(func(node *Node) (bool, bool) {
		if node != nil && node.Kind() == NodeLexRulePatternUsing {
			refs = append(refs, ruleRefsFromLexRulePatternUsing(node, env)...)
		}

		if node != nil && node.Kind() == NodeParseSegment {
			if inner, _, ok := ExpandedTemplateBodyRootForCallSegment(node, env); ok {
				refs = append(refs, StaticRuleAndPrattRefsFromTemplateCallSegment(node, env)...)
				refs = append(refs, staticRuleAndPrattRefsFromTemplateBody(inner, env, make(map[string]bool))...)
				return false, false
			}
			if SegmentHasExplicitTemplateInvocation(node) {
				return false, false
			}
		}

		name := RefName(node)
		if name == "" {
			return false, false
		}

		if _, isRule := env.Rules[name]; isRule {
			refs = append(refs, name)
		} else if _, isPratt := env.Pratt[name]; isPratt {
			refs = append(refs, name)
		}

		return false, false
	})

	return refs
}

func usingRuleReferenceParts(usingRef *Node) (string, string) {
	if usingRef == nil {
		return "", ""
	}
	moduleRef := usingRef.FindFirstKind(NodeModuleReference)
	symbolRef := usingRef.FindFirstKind(NodePatternExternalPatternReference)
	if moduleRef == nil || symbolRef == nil {
		return "", ""
	}
	return IdentifierValue(moduleRef), IdentifierValue(symbolRef)
}

func extractImportedRuleRefs(ruleNode *Node, env *SemanticEnv) []string {
	if ruleNode == nil {
		return nil
	}
	body := ruleNode.FindFirstKind(NodeParseRuleBody)
	if body == nil {
		return nil
	}
	var refs []string
	body.WalkPre(func(node *Node) (bool, bool) {
		if node != nil && node.Kind() == NodeLexRulePatternUsing {
			refs = append(refs, ruleRefsFromLexRulePatternUsing(node, env)...)
		}
		if node != nil && node.Kind() == NodeParseSegment {
			if inner, _, ok := ExpandedTemplateBodyRootForCallSegment(node, env); ok {
				refs = append(refs, StaticRuleAndPrattRefsFromTemplateCallSegment(node, env)...)
				refs = append(refs, staticRuleAndPrattRefsFromTemplateBody(inner, env, make(map[string]bool))...)
				return false, false
			}
			if SegmentHasExplicitTemplateInvocation(node) {
				return false, false
			}
		}

		if node == nil {
			return false, false
		}
		if node.Kind() != NodeParseSymbolReference && node.Kind() != NodeParseExpressionReference {
			return false, false
		}
		refName := RefName(node)
		if refName == "" {
			return false, false
		}
		if env.Rules[refName] != nil || env.Pratt[refName] != nil {
			refs = append(refs, refName)
		}
		return false, false
	})
	return refs
}

func computeReachablePatterns(root *Node, deps map[string][]string) map[string]bool {
	entryPoints := make(map[string]bool)
	if lexSection := root.FindFirstKind(NodeLexSection); lexSection != nil {
		for _, ruleNode := range lexSection.FindAllKind(NodeLexRule) {
			if varRef := ruleNode.FindFirstKind(NodePatternRef); varRef != nil {
				if name := PatternRefTargetName(varRef); name != "" {
					entryPoints[name] = true
				}
			}
		}
	}

	reachable := make(map[string]bool)
	var bfs func(name string)
	bfs = func(name string) {
		if reachable[name] {
			return
		}
		reachable[name] = true
		for _, ref := range deps[name] {
			bfs(ref)
		}
	}

	for name := range entryPoints {
		bfs(name)
	}
	return reachable
}

func computeReachableParseRules(deps map[string][]string, entryRuleName string) map[string]bool {
	if _, ok := deps[entryRuleName]; !ok {
		return nil
	}
	reachable := make(map[string]bool)
	var bfs func(name string)
	bfs = func(name string) {
		if reachable[name] {
			return
		}
		reachable[name] = true
		for _, ref := range deps[name] {
			bfs(ref)
		}
	}
	bfs(entryRuleName)
	return reachable
}

func findCycleInPatternDeps(start string, deps map[string][]string) []string {
	path := make(map[string]bool)
	var stack []string
	var cycle []string

	var dfs func(name string) bool
	dfs = func(name string) bool {
		if path[name] {
			for i := range stack {
				if stack[i] == name {
					cycle = append([]string{}, stack[i:]...)
					cycle = append(cycle, name)
					return true
				}
			}
			return true
		}

		path[name] = true
		stack = append(stack, name)
		for _, ref := range deps[name] {
			if dfs(ref) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		path[name] = false
		return false
	}

	if dfs(start) && cycle != nil {
		return cycle
	}
	return nil
}

func formatCycle(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	return strings.Join(cycle, " -> ")
}

type lexRuleInfo struct {
	tokenName       string
	patternKey      string
	priority        int
	tokenNameNode   *Node
	patternNode     *Node
	lexerStateGroup string
	stackSig        string
}

func collectLexRules(root *Node) []lexRuleInfo {
	var out []lexRuleInfo
	lexSection := root.FindFirstKind(NodeLexSection)
	if lexSection == nil {
		return out
	}

	for _, stateList := range lexSection.FindAllKind(NodeStateList) {
		groupKey := strings.Join(lexerStateDefinitionNames(stateList), ",")
		body := stateList.FindFirstKind(NodeStateDefinitionBody)
		if body == nil {
			continue
		}

		for _, ruleNode := range body.FindAllKind(NodeLexRule) {
			tokenNameNode := ruleNode.FindFirstKind(NodeLexRuleTokenName)
			if tokenNameNode == nil {
				continue
			}
			tokenName := strings.TrimSpace(IdentifierValue(tokenNameNode))
			if tokenName == "" {
				continue
			}

			priority := parsePriority(ruleNode.FindFirstKind(NodeLexRulePriority))
			patternKey, patternNode := extractLexPattern(ruleNode)

			if patternKey != "" && patternNode != nil {
				out = append(out, lexRuleInfo{
					tokenName:       tokenName,
					patternKey:      patternKey,
					priority:        priority,
					tokenNameNode:   tokenNameNode,
					patternNode:     patternNode,
					lexerStateGroup: groupKey,
					stackSig:        lexerRuleStackSignature(ruleNode),
				})
			}
		}
	}
	return out
}

func lexerRuleStackSignature(ruleNode *Node) string {
	mutRoot := ruleNode.FindFirstKind(NodeLexRuleStateMutation)
	if mutRoot == nil {
		return "none"
	}
	if mutRoot.FindFirstKind(NodeStateMutationPush) != nil {
		return "push:" + strings.Join(lexerStateRefNames(mutRoot), ",")
	}
	if mutRoot.FindFirstKind(NodeStateMutationSet) != nil {
		return "set:" + strings.Join(lexerStateRefNames(mutRoot), ",")
	}
	if mutRoot.FindFirstKind(NodeStateMutationPop) != nil {
		amt := 1
		if an := mutRoot.FindFirstKind(NodeStateMutationPopAmount); an != nil {
			if v, ok := parseIntFromNode(an); ok {
				amt = v
			}
		}
		return fmt.Sprintf("pop:%d", amt)
	}
	return "none"
}

func lexerStateRefNames(mutRoot *Node) []string {
	var s []string
	for _, ref := range mutRoot.FindAllKind(NodeStateReference) {
		s = append(s, NodeSingleTokenContent(ref))
	}
	return s
}

func groupLexRulesByPattern(rules []lexRuleInfo) map[string][]lexRuleInfo {
	grouped := make(map[string][]lexRuleInfo)
	for _, r := range rules {
		key := r.lexerStateGroup + "|" + r.patternKey
		grouped[key] = append(grouped[key], r)
	}
	return grouped
}

func parsePriority(priNode *Node) int {
	if priNode != nil {
		if raw := priNode.GetContent(""); raw != "" {
			if n, err := parseIntFromContent(raw); err == nil {
				return n
			}
		}
	}
	return 0
}

func extractLexPattern(ruleNode *Node) (string, *Node) {
	if varRef := ruleNode.FindFirstKind(NodePatternRef); varRef != nil {
		if name := PatternRefTargetName(varRef); name != "" {
			return "ref:" + name, varRef
		}
	}
	if usingRef := ruleNode.FindFirstKind(NodeLexRulePatternUsing); usingRef != nil {
		moduleNode := usingRef.FindFirstKind(NodeModuleReference)
		symbolNode := usingRef.FindFirstKind(NodePatternExternalPatternReference)
		if moduleNode != nil && symbolNode != nil {
			moduleName := IdentifierValue(moduleNode)
			symbolName := IdentifierValue(symbolNode)
			if moduleName != "" && symbolName != "" {
				return "using:" + moduleName + "." + symbolName, usingRef
			}
		}
	}
	if regexNode := ruleNode.FindFirstKind(NodeLexRulePattern); regexNode != nil {
		return "regex:" + regexNode.GetContent(""), regexNode
	}
	return "", nil
}

func parseIntFromContent(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func parseIntFromNode(node *Node) (int, bool) {
	if node == nil || len(node.Tokens()) != 1 {
		return 0, false
	}
	raw := strings.TrimSpace(string(node.Tokens()[0].Raw))
	n, err := parseIntFromContent(raw)
	return n, err == nil
}

func negationAllowedKind(kind LangSpecParserNodeKind) bool {
	switch kind {
	case NodeCharLiteral, NodePatternRange, NodePatternClass,
		NodePatternGroup, NodePatternAlternation, NodePatternSegment:
		return true
	default:
		return false
	}
}

func validateNegationSubtree(node *Node, report func(offending *Node)) {
	kind := node.Kind()
	if !negationAllowedKind(kind) {
		report(node)
	}
	switch kind {
	case NodeCharLiteral, NodePatternRange, NodePatternClass:
		return
	default:
		for _, ch := range node.Children() {
			validateNegationSubtree(ch, report)
		}
	}
}

func findParseRuleByName(parseSection *Node, ruleName string) *Node {
	for _, rule := range parseSection.FindAllKind(NodeParseRule) {
		nameNode := rule.FindFirstKind(NodeParseRuleName)
		if ParseRuleName(nameNode) == ruleName {
			return rule
		}
	}
	return nil
}
