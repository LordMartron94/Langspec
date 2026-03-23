package toolchain

import (
	"cmp"
	"errors"
	"fmt"
	"foundation/bytes"
	"foundation/hash"
	"foundation/system"
	"langspec/dsl"
	"langspec/editor"
	"langspec/editor/sublime"
	"lexarch"
	"syntaxa"
)

/* PRAGMA keys for the Sublime toolchain (tool.sublime { ... } in .lspec). */
const (
	SublimeToolName             = "sublime"
	SublimeOutputPathKey        = "output-path"
	SublimeConfigurationPathKey = "configuration-path"
	SublimeEnableKey            = "enable"
)

var hasher = hash.XXH3HasherCreateWithSeed(6789)

/*
SublimeContext carries the scope and optional meta-scope used when building Sublime Text syntax
rules from the editor IR.
*/
type SublimeContext struct {
	Scope     string
	MetaScope string
}

/*
SublimeRunnerConfig holds the lexer, grammar, IR config, file extensions, base scope, output path,
and scope suffix for a single Sublime generator run. Used after the toolchain has built the editor IR.
*/
type SublimeRunnerConfig[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable] struct {
	LexerRuleset   *lexarch.LexingRuleset[TObservation, TToken, TTokenRole]
	GrammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]
	IRConfig       *editor.EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, SublimeContext]
	FileExtensions []string
	BaseScope      string
	OutputPaths    []string
	ScopeSuffix    string
}

// ----------------------------------------------------------------- ENTRY POINTS

/*
RunSublimeToolchain is the JSON-driven entry point for the Sublime toolchain. It
reads the Sublime PRAGMA (enable, output-path, configuration-path), loads the
manifest from the file at configuration-path via LoadSublimeConfigFromJSON,
then runs the toolchain.

Use cases:
- Generating Sublime syntax when the manifest is defined in a JSON file (e.g. shared config in repo).
- LangSpec compiling itself or other specs that do not use bootstrap.WithSublimeToolchain.
- One-off or script-driven generation where no in-memory manifest is supplied.

Prerequisites:
- compileResult must contain a tool.sublime PRAGMA with enable = true, output-path set, and configuration-path set.
- The file at configuration-path must exist and be valid JSON matching SublimeConfiguration.

Edge cases:
- Returns nil without error if no tool.sublime PRAGMA is present or enable is not "true".
- overrideProducer may be nil; a no-op producer is used internally so overrides are optional.
- Returns an error if configuration-path is missing, file read fails, or JSON is invalid.
*/
func RunSublimeToolchain(
	compileResult *dsl.LangSpecCompileResult,
	overrideProducer func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, SublimeContext],
) error {
	for _, pragma := range compileResult.CompiledToolPragmas {
		if pragma.ToolName != SublimeToolName {
			continue
		}

		configPath := pragma.Settings[SublimeConfigurationPathKey].(string)
		enabled := pragma.Settings[SublimeEnableKey].(string)

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

		outputPaths, err := ExtractOutputPathsFromSublimePragma(pragma)
		if err != nil {
			return err
		}

		return executeSublimeToolchain(
			compileResult,
			sublimeCfg.Manifest,
			overrideProducer,
			outputPaths,
			sublimeCfg.FileExtensions,
			sublimeCfg.ScopeExtension,
		)
	}
	return nil
}

func ExtractOutputPathsFromSublimePragma(pragma dsl.ToolPragma) ([]string, error) {
	outputPathValue := pragma.Settings[SublimeOutputPathKey]
	var rawPaths []string

	if path, cnvOk := outputPathValue.(string); cnvOk {
		rawPaths = append(rawPaths, path)
	} else if pathArray, cnvOk := outputPathValue.([]string); cnvOk {
		rawPaths = pathArray
	} else {
		return nil, fmt.Errorf("engine-error: output paths neither single value nor array")
	}

	var resolvedPaths []string
	for _, rp := range rawPaths {
		resolved, err := system.PathResolveWorkspace(rp)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path %s: %w", rp, err)
		}
		resolvedPaths = append(resolvedPaths, resolved)
	}

	return resolvedPaths, nil
}

/*
RunSublimeToolchainFromMemory is the in-memory entry point for the Sublime
toolchain. It runs the pipeline using the provided SemanticManifest,
outputPath, fileExtensions, and scopeExtension without reading any
configuration file.

Use cases:
- Bootstrapped languages where the host supplies manifest and metadata in Go via WithSublimeToolchain.
- Typed manifests (e.g. from a compiler) that are not serialized to JSON.
- Avoiding file I/O when the manifest is already in memory.

Prerequisites:
- compileResult must be a valid LangSpecCompileResult from a successful compile.
- outputPath, fileExtensions, and scopeExtension must be set as desired for the generated syntax file.

Edge cases:
- overrideProducer may be nil; a no-op producer is used internally.
- Does not read or validate PRAGMA; the caller (e.g. bootstrap) is responsible for enable and output-path.
*/
func RunSublimeToolchainFromMemory(
	compileResult *dsl.LangSpecCompileResult,
	manifest SemanticManifest[string, string],
	overrideProducer func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, SublimeContext],
	outputPaths []string,
	fileExtensions []string,
	scopeExtension string,
) error {
	return executeSublimeToolchain(
		compileResult,
		manifest,
		overrideProducer,
		outputPaths,
		fileExtensions,
		scopeExtension,
	)
}

/*
executeSublimeToolchain is the shared implementation for both manifest sources.
It builds the editor IR (context producer from manifest, optional override producer),
then runs the Sublime generator. Called by RunSublimeToolchain and RunSublimeToolchainFromMemory.
*/
func executeSublimeToolchain(
	compileResult *dsl.LangSpecCompileResult,
	manifest SemanticManifest[string, string],
	overrideProducer func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, SublimeContext],
	outputPaths []string,
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
		OutputPaths:    outputPaths,
		ScopeSuffix:    scopeExtension,
	}

	return RunSublimeGenerator(runnerCfg)
}

/*
RunSublimeGenerator builds the editor IR from the given runner config and writes
the .sublime-syntax file to cfg.OutputPath.

Use cases:
- Called internally by executeSublimeToolchain after the IR config is built.
- Direct use when the caller has already constructed SublimeRunnerConfig (e.g. dsl/editor with a typed manifest).

Prerequisites:
- cfg must be fully populated: LexerRuleset, GrammarPackage, IRConfig, FileExtensions, BaseScope, OutputPath, ScopeSuffix.

Edge cases:
- Returns an error if editor IR creation or file write fails.
*/
func RunSublimeGenerator[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable](
	cfg *SublimeRunnerConfig[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
) error {
	editorIR, err := editor.EditorIRCreate(
		cfg.LexerRuleset,
		cfg.GrammarPackage,
		cfg.IRConfig,
	)
	if err != nil {
		return fmt.Errorf("failed to create editor IR: %w", err)
	}

	var errs []error

	for _, outputPath := range cfg.OutputPaths {
		genErr := sublime.GenerateSyntaxFile(
			editorIR,
			cfg.FileExtensions,
			cfg.BaseScope,
			outputPath,
			sublime.ExtractionConfig[SublimeContext]{
				ExtractScope: func(ctx SublimeContext) string {
					return applyScopeSuffix(ctx.Scope, cfg.ScopeSuffix)
				},
				ExtractMetaScope: func(ctx SublimeContext) string {
					return applyScopeSuffix(ctx.MetaScope, cfg.ScopeSuffix)
				},
			},
		)
		if genErr != nil {
			errs = append(errs, fmt.Errorf("output path %s: %w", outputPath, genErr))
		}
	}

	return errors.Join(errs...)
}
