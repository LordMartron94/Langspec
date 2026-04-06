package dsl

import (
	"fmt"
	"path/filepath"

	"langspec"
	"langspec/dsl/semantics"
	dslspec "langspec/dsl/spec"
)

type resolvedImportGraph struct {
	byAlias map[string]*semantics.ImportedModuleSymbols
}

func resolveImportGraph(compiler *LangSpecCompiler, sourceFile string, root *Node) (*resolvedImportGraph, error) {
	graph := &resolvedImportGraph{
		byAlias: make(map[string]*semantics.ImportedModuleSymbols),
	}
	visitedByPath := make(map[string]bool)
	if err := resolveImportGraphRecursive(compiler, sourceFile, root, graph, visitedByPath); err != nil {
		return nil, err
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
	for alias, rawPath := range imports {
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

		importRoot, parseErr := parseLangSpecRootOnly(compiler, absImport)
		if parseErr != nil {
			return fmt.Errorf("failed to parse imported module '%s' (%s): %w", alias, absImport, parseErr)
		}
		graph.byAlias[alias] = semantics.ImportedModuleSymbolsBuild(alias, absImport, importRoot)

		if err := resolveImportGraphRecursive(compiler, absImport, importRoot, graph, visitedByPath); err != nil {
			return err
		}
	}
	return nil
}

func collectImportDefinitions(root *Node) map[string]string {
	out := make(map[string]string)
	importSection := root.FindFirstKind(dslspec.NodeImportSection)
	if importSection == nil {
		return out
	}
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
		out[alias] = pathValue
	}
	return out
}

func parseLangSpecRootOnly(compiler *LangSpecCompiler, sourceFile string) (*Node, error) {
	session := langspec.LangParserSessionCreate[rune](sourceFile, nil)
	_, root, syntaxErrors, err := langspec.LangParserParseFile(compiler.parser, session, nil)
	if err != nil {
		if syntaxErrors != nil && syntaxErrors.HasErrors() {
			first := syntaxErrors.Errors[0]
			return nil, fmt.Errorf("%w; first syntax error: %s at %d:%d", err, first.Message, first.StartLine, first.StartColumn)
		}
		return nil, err
	}
	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		first := syntaxErrors.Errors[0]
		return nil, fmt.Errorf("imported file has syntax errors: %s at %d:%d", first.Message, first.StartLine, first.StartColumn)
	}
	return root, nil
}
