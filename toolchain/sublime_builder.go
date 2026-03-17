package toolchain

import (
	"cmp"
	"fmt"
	"foundation/bytes"
	"foundation/hash"
	"langspec/dsl"
	"langspec/editor"
	"langspec/editor/sublime"
	"lexarch"
	"syntaxa"
)

const (
	SublimeToolName             = "sublime"
	SublimeOutputPathKey        = "output-path"
	SublimeConfigurationPathKey = "configuration-path"
	SublimeEnableKey            = "enable"
)

var hasher = hash.XXH3HasherCreateWithSeed(6789)

type SublimeContext struct {
	Scope     string
	MetaScope string
}

type SublimeRunnerConfig[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable] struct {
	LexerRuleset   *lexarch.LexingRuleset[TObservation, TToken, TTokenRole]
	GrammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]
	IRConfig       *editor.EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, SublimeContext]
	FileExtensions []string
	BaseScope      string
	OutputPath     string
	ScopeSuffix    string
}

// ----------------------------------------------------------------- ENTRY POINTS

/*
RunSublimeToolchain is the legacy JSON-driven entry point (used by LangSpec compiling itself).
It reads the configuration path from the pragma and loads the manifest from disk.
*/
func RunSublimeToolchain(
	compileResult *dsl.LangSpecCompileResult,
	overrideProducer func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, SublimeContext],
) error {
	for _, pragma := range compileResult.CompiledToolPragmas {
		if pragma.ToolName != SublimeToolName {
			continue
		}

		configPath := pragma.Settings[SublimeConfigurationPathKey]
		outputPath := pragma.Settings[SublimeOutputPathKey]
		enabled := pragma.Settings[SublimeEnableKey]

		if enabled == "false" {
			return nil
		}
		if enabled != "true" {
			return fmt.Errorf("unexpected enabled setting value: '%s'", enabled)
		}

		sublimeCfg, err := LoadSublimeConfigFromJSON(configPath)
		if err != nil {
			return err
		}

		return executeSublimeToolchain(
			compileResult,
			sublimeCfg.Manifest,
			overrideProducer,
			outputPath,
			sublimeCfg.FileExtensions,
			sublimeCfg.ScopeExtension,
		)
	}
	return nil
}

/*
RunSublimeToolchainFromMemory is the pure Go entry point (used by bootstrapped languages).
It bypasses JSON loading and uses the provided SemanticManifest directly.
*/
func RunSublimeToolchainFromMemory(
	compileResult *dsl.LangSpecCompileResult,
	manifest SemanticManifest[string, string],
	overrideProducer func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, SublimeContext],
	outputPath string,
	fileExtensions []string,
	scopeExtension string,
) error {
	return executeSublimeToolchain(
		compileResult,
		manifest,
		overrideProducer,
		outputPath,
		fileExtensions,
		scopeExtension,
	)
}

// ----------------------------------------------------------------- CORE EXECUTION

func executeSublimeToolchain(
	compileResult *dsl.LangSpecCompileResult,
	manifest SemanticManifest[string, string],
	overrideProducer func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, SublimeContext],
	outputPath string,
	fileExtensions []string,
	scopeExtension string,
) error {
	if overrideProducer == nil {
		overrideProducer = func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, SublimeContext] {
			return nil
		}
	}

	ctxProducer := BuildContextProducerFromManifest[rune, string, string, string, string](manifest)

	irConfig := editor.EditorIRConfigurationCreate(
		hasher,
		func(token string) uint64 {
			return hash.XXH3HasherHash64(hasher, bytes.StringSliceToBytes([]string{token}, 0x00))
		},
		ctxProducer,
		overrideProducer,
		func(left, right SublimeContext) bool {
			return left == right
		},
	)

	ruleset := compileResult.CompiledLexerSpec.Ruleset("default")

	runnerCfg := &SublimeRunnerConfig[rune, string, string, string, string]{
		LexerRuleset:   &ruleset,
		GrammarPackage: &compileResult.CompiledGrammarPackage,
		IRConfig:       irConfig,
		FileExtensions: fileExtensions,
		BaseScope:      fmt.Sprintf("source%s", scopeExtension),
		OutputPath:     outputPath,
		ScopeSuffix:    scopeExtension,
	}

	return RunSublimeGenerator(runnerCfg)
}

func RunSublimeGenerator[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable](
	cfg *SublimeRunnerConfig[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
) error {
	editorIR, err := editor.EditorIRCreate(
		cfg.LexerRuleset,
		cfg.GrammarPackage,
		cfg.IRConfig,
	)
	if err != nil {
		return err
	}

	return sublime.GenerateSyntaxFile(
		editorIR,
		cfg.FileExtensions,
		cfg.BaseScope,
		cfg.OutputPath,
		sublime.ExtractionConfig[SublimeContext]{
			ExtractScope: func(ctx SublimeContext) string {
				return applyScopeSuffix(ctx.Scope, cfg.ScopeSuffix)
			},
			ExtractMetaScope: func(ctx SublimeContext) string {
				return applyScopeSuffix(ctx.MetaScope, cfg.ScopeSuffix)
			},
		},
	)
}
