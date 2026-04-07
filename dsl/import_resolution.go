package dsl

import (
	"fmt"
	"path/filepath"

	"langspec"
	"langspec/dsl/semantics"
	dslspec "langspec/dsl/spec"
	"langspec/validation"
)

type resolvedImportGraph struct {
	byAlias      map[string]*semantics.ImportedModuleSymbols
	diagnostics  []ImportedModuleDiagnostic
	diagnosticsN map[string]bool
}

type importDefinition struct {
	Alias   string
	Path    string
	IsEmbed bool
}

func resolveImportGraph(compiler *LangSpecCompiler, sourceFile string, root *Node) (*resolvedImportGraph, error) {
	graph := &resolvedImportGraph{
		byAlias:      make(map[string]*semantics.ImportedModuleSymbols),
		diagnosticsN: make(map[string]bool),
	}
	visitedByPath := make(map[string]bool)
	if err := resolveImportGraphRecursive(compiler, sourceFile, root, graph, visitedByPath); err != nil {
		return graph, err
	}
	return graph, nil
}

func resolveImportGraphRecursive(
	compiler *LangSpecCompiler,
	sourceFile string,
	root *Node,
	graph *resolvedImportGraph,
	visitedByPath map[string]bool,
) error {
	if root == nil {
		return nil
	}
	absSource, err := filepath.Abs(sourceFile)
	if err != nil {
		return err
	}
	if visitedByPath[absSource] {
		return nil
	}
	visitedByPath[absSource] = true

	imports := collectImportDefinitions(root)
	for _, importDef := range imports {
		alias := importDef.Alias
		rawPath := importDef.Path
		if alias == "" || rawPath == "" {
			continue
		}
		if _, exists := graph.byAlias[alias]; exists {
			return fmt.Errorf("duplicate import alias '%s' while resolving '%s'", alias, sourceFile)
		}

		absImport := rawPath
		if !filepath.IsAbs(absImport) {
			absImport = filepath.Join(filepath.Dir(absSource), rawPath)
		}
		absImport = filepath.Clean(absImport)

		importRoot, importErr := parseAndValidateImportModule(compiler, alias, absImport, graph)
		if importErr != nil {
			return importErr
		}
		graph.byAlias[alias] = semantics.ImportedModuleSymbolsBuild(alias, absImport, importRoot, importDef.IsEmbed)

		if err := resolveImportGraphRecursive(compiler, absImport, importRoot, graph, visitedByPath); err != nil {
			return err
		}
	}
	return nil
}

func collectImportDefinitions(root *Node) []importDefinition {
	var out []importDefinition
	importSection := root.FindFirstKind(dslspec.NodeImportSection)
	if importSection == nil {
		return out
	}
	seenAliases := make(map[string]bool)
	for _, imp := range importSection.FindAllKind(dslspec.NodeImportDefinition) {
		pathNode := imp.FindFirstKind(dslspec.NodeImportPath)
		aliasNode := imp.FindFirstKind(dslspec.NodeImportAlias)
		if pathNode == nil || aliasNode == nil {
			continue
		}
		pathValue, ok := AttributeAs[string](pathNode, dslspec.ATTRIBUTE_LITERAL_STRING_VALUE)
		if !ok {
			continue
		}
		alias := dslspec.IdentifierValue(aliasNode)
		if alias == "" || pathValue == "" {
			continue
		}
		if seenAliases[alias] {
			continue
		}
		seenAliases[alias] = true
		out = append(out, importDefinition{
			Alias:   alias,
			Path:    pathValue,
			IsEmbed: imp.FindFirstKind(dslspec.NodeImportEmbed) != nil,
		})
	}
	return out
}

func parseAndValidateImportModule(
	compiler *LangSpecCompiler,
	alias string,
	sourceFile string,
	graph *resolvedImportGraph,
) (*Node, error) {
	session := langspec.LangParserSessionCreate[rune](sourceFile, nil)
	contentRune, root, syntaxErrors, err := langspec.LangParserParseFile(compiler.parser, session, nil)
	if err != nil {
		if syntaxErrors != nil {
			for _, e := range syntaxErrors.Errors {
				graph.appendDiagnostic(ImportedModuleDiagnostic{
					Alias:       alias,
					Path:        sourceFile,
					Code:        "SYNTAX",
					Message:     e.Message,
					StartLine:   e.StartLine,
					StartColumn: e.StartColumn,
				})
			}
		}
		graph.appendDiagnostic(ImportedModuleDiagnostic{
			Alias:   alias,
			Path:    sourceFile,
			Code:    "IMPORT_PARSE",
			Message: err.Error(),
		})
		return nil, err
	}
	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		for _, e := range syntaxErrors.Errors {
			graph.appendDiagnostic(ImportedModuleDiagnostic{
				Alias:       alias,
				Path:        sourceFile,
				Code:        "SYNTAX",
				Message:     e.Message,
				StartLine:   e.StartLine,
				StartColumn: e.StartColumn,
			})
		}
		return nil, fmt.Errorf("import '%s' (%s) has syntax errors", alias, sourceFile)
	}

	opts := extractCompileOptions(root)
	preValidationState := &semantics.GrammarValidationState{
		ImportedModules: graph.byAlias,
		LibraryMode:     opts.Library,
	}
	validationEntries, validationErr := validation.LSTValidatorRun(
		compiler.validatorConfig,
		root,
		func(stage *validation.LSTValidationStage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState]) bool {
			return stage.Order < 4
		},
		preValidationState,
	)
	if validationErr != nil {
		graph.appendDiagnostic(ImportedModuleDiagnostic{
			Alias:   alias,
			Path:    sourceFile,
			Code:    "IMPORT_VALIDATE",
			Message: validationErr.Error(),
		})
		return nil, validationErr
	}
	if validationEntries != nil {
		for _, stage := range validationEntries.Results {
			for _, entry := range stage.Entries {
				graph.appendDiagnostic(ImportedModuleDiagnostic{
					Alias:   alias,
					Path:    sourceFile,
					Code:    entry.Code,
					Message: entry.Message,
				})
			}
		}
	}
	if hasCriticalValidationErrors(validationEntries) {
		_ = contentRune
		return nil, fmt.Errorf("import '%s' (%s) failed validation", alias, sourceFile)
	}
	return root, nil
}

func (g *resolvedImportGraph) appendDiagnostic(diag ImportedModuleDiagnostic) {
	if g == nil {
		return
	}
	key := fmt.Sprintf("%s|%s|%s|%s|%d|%d", diag.Alias, diag.Path, diag.Code, diag.Message, diag.StartLine, diag.StartColumn)
	if g.diagnosticsN[key] {
		return
	}
	g.diagnosticsN[key] = true
	g.diagnostics = append(g.diagnostics, diag)
}
