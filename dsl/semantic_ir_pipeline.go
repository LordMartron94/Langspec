package dsl

import (
	"autarch/pattern"
	"foundation/domain"
	"langspec"
	"langspec/dsl/ir"
	"langspec/dsl/semantics"
	dslspec "langspec/dsl/spec"
	"lexarch"
	"syntaxa"
	"syntaxa/lowering"
)

func toolPragmasToRaw(pragmas map[string]ToolPragma) map[string]map[string]any {
	if len(pragmas) == 0 {
		return nil
	}
	out := make(map[string]map[string]any, len(pragmas))
	for name, pragma := range pragmas {
		settings := make(map[string]any, len(pragma.Settings))
		for k, v := range pragma.Settings {
			settings[k] = v
		}
		out[name] = settings
	}
	return out
}

func toolPragmasFromRaw(raw map[string]map[string]any) map[string]ToolPragma {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]ToolPragma, len(raw))
	for name, settingsRaw := range raw {
		settings := make(map[string]any, len(settingsRaw))
		for k, v := range settingsRaw {
			settings[k] = v
		}
		out[name] = ToolPragma{
			ToolName: name,
			Settings: settings,
		}
	}
	return out
}

func semanticIRBuildFromLST[TNodeKind ~uint32](comp *LangSpecCompiler, rootNode *Node, sourceFile string, importGraph *resolvedImportGraph) *ir.SemanticModel[TNodeKind] {
	dslName, dslVersion, langspecTargetVersion := getInfoFromHeader(rootNode.FindFirstKind(dslspec.NodeHeader))
	if importGraph == nil {
		var importErr error
		importGraph, importErr = resolveImportGraph(comp, sourceFile, rootNode)
		if importErr != nil {
			panic(importErr)
		}
	}

	env := semantics.BuildSemanticEnvWithImports(rootNode, importGraph.byAlias, nil)
	eofName := getEOFToken(rootNode)
	tokStrs, roleStrs, nodeStrs := semantics.CollectCompiledSymbolStrings(rootNode, env, eofName)
	sym := semantics.CompiledSymbolTableBuild(tokStrs, roleStrs, nodeStrs)

	compileOpts := extractCompileOptions(rootNode)
	return &ir.SemanticModel[TNodeKind]{
		RootNode:               rootNode,
		SourceFile:             sourceFile,
		LanguageName:           dslName,
		LanguageVersion:        dslVersion,
		TargetLangspecVersion:  langspecTargetVersion,
		CompileOptions:         ir.CompileOptions{Library: compileOpts.Library, WarningsAsErrors: compileOpts.WarningsAsErrors},
		EOFTokenName:           eofName,
		ToolPragmas:            toolPragmasToRaw(extractPragmas(rootNode)),
		SemanticEnv:            env,
		CompiledSymbolTable:    sym,
		ImportedModulesByAlias: importGraph.byAlias,
	}
}

func semanticIRLowerToCompiled[TNodeKind ~uint32](comp *LangSpecCompiler, model *ir.SemanticModel[TNodeKind]) *CompiledLangSpec[TNodeKind] {
	domainRunes := domain.DiscreteDomainRuneCreate()
	eofToken := model.CompiledSymbolTable.TokenID(model.EOFTokenName)
	lexerSpec := langspec.LexerSpecCreate[rune, uint32, uint32, string](
		eofToken,
		"INITIAL",
		func(t uint32) string { return model.CompiledSymbolTable.TokenName(t) },
		true,
	)
	lexerSpec.WithCompilationMode(lexarch.PATTERN_COMPILE_GLUSHKOV)
	switch comp.config.lexerPositionTracking {
	case LangSpecLexerPositionTrackingGeneric:
	case LangSpecLexerPositionTrackingRuneFast:
		lexerSpec.WithRunePositionTrackingFast(comp.config.lexerRuneTabWidth)
	default:
		lexerSpec.WithRunePositionTrackingFast(4)
	}

	lspecCompiler := compiler{
		factory: pattern.RegulaASTFactoryCreate(domainRunes),
	}
	patternCtx := &patternCompileCtx{
		c:        &lspecCompiler,
		rootNode: model.RootNode,
		env:      model.SemanticEnv,
		sym:      model.CompiledSymbolTable,
	}
	lspecCompiler.compilePatterns(patternCtx)
	lspecCompiler.compileLexerSpec(patternCtx, lexerSpec, "")
	lspecCompiler.compileEmbeddedLexerSpecs(model.SemanticEnv, lexerSpec, model.CompiledSymbolTable)
	injectEmbedLexerHandoffs(model.RootNode, model.SemanticEnv, lexerSpec, model.CompiledSymbolTable)

	grammarPackage, ruleRegistry, rootNodeKind, skipRoles, sourceMap := getParserSpecInfo[TNodeKind](
		model.RootNode,
		model.SemanticEnv,
		model.LanguageName,
		model.LanguageVersion,
		model.CompiledSymbolTable,
	)
	grammarPkg := new(syntaxa.GrammarPackage[TNodeKind])
	*grammarPkg = grammarPackage

	analysis := lowering.GetAnalysis(grammarPkg)
	getAnalysis := func() *syntaxa.GrammarAnalysis { return analysis }
	parserSpec := langspec.ParserSpecCreate[rune, uint32, uint32, string, TNodeKind](
		grammarPkg,
		ruleRegistry,
		rootNodeKind,
		TNodeKind(model.CompiledSymbolTable.NodeKindID("ERROR_NODE")),
		false,
		getAnalysis,
	)
	parserSpec.WithSkipRoles(skipRoles...)

	model.SourceMap = sourceMap
	model.Lowered = &ir.LoweredArtifacts[TNodeKind]{
		LexerSpec:      lexerSpec,
		ParserSpec:     parserSpec,
		GrammarPackage: *grammarPkg,
		EOFToken:       eofToken,
	}

	return &CompiledLangSpec[TNodeKind]{
		dslName:               model.LanguageName,
		dslVersion:            model.LanguageVersion,
		targetLangspecVersion: model.TargetLangspecVersion,
		lexerSpec:             lexerSpec,
		parserSpec:            parserSpec,
		grammarPackage:        *grammarPkg,
		toolPragmas:           toolPragmasFromRaw(model.ToolPragmas),
		symbols:               model.CompiledSymbolTable,
		eofToken:              eofToken,
		sourceMap:             sourceMap,
	}
}

func SemanticIRBuildFromCompileResult[TNodeKind ~uint32](result *LangSpecCompileResult[TNodeKind]) *ir.SemanticModel[TNodeKind] {
	if result == nil {
		return nil
	}
	model := &ir.SemanticModel[TNodeKind]{
		RootNode:              result.RootNode,
		LanguageName:          result.LanguageName,
		LanguageVersion:       result.LanguageVersion,
		TargetLangspecVersion: result.TargetLangspecVersion,
		ToolPragmas:           toolPragmasToRaw(result.CompiledToolPragmas),
		CompiledSymbolTable:   result.CompiledSymbols,
		SourceMap:             result.SourceMap,
	}
	if result.CompiledSymbols != nil {
		model.EOFTokenName = result.CompiledSymbols.TokenName(result.EOFToken)
	}
	model.Lowered = &ir.LoweredArtifacts[TNodeKind]{
		LexerSpec:      result.CompiledLexerSpec,
		ParserSpec:     result.CompiledParserSpec,
		GrammarPackage: result.CompiledGrammarPackage,
		EOFToken:       result.EOFToken,
	}
	return model
}
