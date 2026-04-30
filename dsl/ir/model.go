package ir

import (
	"langspec"
	"langspec/dsl/semantics"
	dslspec "langspec/dsl/spec"
	"lexarch"
	"syntaxa"
)

type CompileOptions struct {
	Library          bool
	WarningsAsErrors bool
}

type SemanticModel[TNodeKind ~uint32] struct {
	RootNode *dslspec.Node

	SourceFile string

	LanguageName           string
	LanguageVersion        string
	TargetLangspecVersion  string
	CompileOptions         CompileOptions
	EOFTokenName           string
	ToolPragmas            map[string]map[string]any
	SemanticEnv            *semantics.SemanticEnv
	CompiledSymbolTable    *semantics.CompiledSymbolTable
	ImportedModulesByAlias map[string]*semantics.ImportedModuleSymbols

	SourceMap map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*dslspec.Node
	Lowered   *LoweredArtifacts[TNodeKind]
}

type LoweredArtifacts[TNodeKind ~uint32] struct {
	LexerSpec      *langspec.LexerSpec[rune, uint32, uint32, string]
	ParserSpec     *langspec.ParserSpec[rune, uint32, uint32, string, TNodeKind]
	GrammarPackage semantics.GrammarPackage[TNodeKind]
	EOFToken       uint32
}
