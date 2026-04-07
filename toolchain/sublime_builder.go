package toolchain

import (
	"cmp"
	"errors"
	"fmt"
	"foundation/hash"
	"langspec"
	"langspec/dsl"
	"langspec/editor"
	"langspec/editor/sublime"
	"lexarch"
	"os"
	"strings"
	"syntaxa"
)

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
	LexerRuleset   *editor.LexingRuleSet[TObservation, TToken, TTokenRole]
	GrammarPackage *syntaxa.GrammarPackage[TNodeKind]
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
	overrideProducer func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, SublimeContext],
) error {
	if pragma, ok := compileResult.CompiledToolPragmas[SublimeToolName]; ok {
		enabled, ok := pragma.Settings[SublimeEnableKey].(string)
		if !ok {
			return fmt.Errorf("sublime toolchain: missing or invalid '%s'", SublimeEnableKey)
		}

		if enabled == "false" {
			return nil
		}
		if enabled != "true" {
			return fmt.Errorf("unexpected enabled setting value: '%s'", enabled)
		}

		configPathVal, ok := pragma.Settings[SublimeConfigurationPathKey].(string)
		if !ok || strings.TrimSpace(configPathVal) == "" {
			return fmt.Errorf("sublime toolchain: '%s' is required when %s is true", SublimeConfigurationPathKey, SublimeEnableKey)
		}
		configPath := configPathVal

		sublimeCfg, err := LoadSublimeConfigFromJSON(configPath)
		if err != nil {
			return err
		}

		outputPaths, err := ExtractOutputPathsFromPragma(pragma, SublimeOutputPathKey)
		if err != nil {
			return err
		}

		return executeSublimeToolchain(
			compileResult,
			sublimeCfg.Manifest,
			sublimeOverrideFactoryFromProducer(overrideProducer),
			outputPaths,
			sublimeCfg.FileExtensions,
			sublimeCfg.ScopeExtension,
		)
	}
	return nil
}

/*
sublimeOverrideFactoryFromProducer adapts the legacy RunSublimeToolchain override producer
to SublimeInMemoryOverrideFactory. The returned inner producer ignores the lexing ruleset
and coverage ctxProducer (callers that need accurate unused-manifest coverage should pass
a real factory to RunSublimeToolchainFromMemory instead).
*/
func sublimeOverrideFactoryFromProducer(
	overrideProducer func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, SublimeContext],
) SublimeInMemoryOverrideFactory {
	return func(
		lexing *editor.LexingRuleSet[rune, uint32, uint32],
		ctxProducer func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) SublimeContext,
	) func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, SublimeContext] {
		_ = lexing
		_ = ctxProducer
		if overrideProducer == nil {
			return func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, SublimeContext] {
				return nil
			}
		}
		return overrideProducer
	}
}

// SublimeInMemoryOverrideFactory builds Sublime override rules from the flattened lexer
// ruleset and the same manifest context producer wired into editor IR. Using this factory
// (instead of a pre-built override producer) keeps override-driven scope lookups on the
// coverage-enabled producer used for unused-manifest warnings.
type SublimeInMemoryOverrideFactory func(
	lexing *editor.LexingRuleSet[rune, uint32, uint32],
	ctxProducer func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) SublimeContext,
) func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, SublimeContext]

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
- overrideFactory may be nil; a no-op inner producer is used.
- Does not read or validate PRAGMA; the caller (e.g. bootstrap) is responsible for enable and output-path.
*/
func RunSublimeToolchainFromMemory(
	compileResult *dsl.LangSpecCompileResult,
	manifest SemanticManifest[string, string],
	overrideFactory SublimeInMemoryOverrideFactory,
	outputPaths []string,
	fileExtensions []string,
	scopeExtension string,
) error {
	return executeSublimeToolchain(
		compileResult,
		manifest,
		overrideFactory,
		outputPaths,
		fileExtensions,
		scopeExtension,
	)
}

/*
executeSublimeToolchain is the shared implementation for both manifest sources.
It builds the editor IR (manifest context producer with coverage, then overrideFactory(lexing, ctxProducer)),
then runs the Sublime generator. Called by RunSublimeToolchain and RunSublimeToolchainFromMemory.
*/
func executeSublimeToolchain(
	compileResult *dsl.LangSpecCompileResult,
	manifest SemanticManifest[string, string],
	overrideFactory SublimeInMemoryOverrideFactory,
	outputPaths []string,
	fileExtensions []string,
	scopeExtension string,
) error {
	if compileResult.CompiledSymbols == nil {
		return fmt.Errorf("sublime toolchain: compiled symbols missing on compile result")
	}

	if overrideFactory == nil {
		overrideFactory = func(
			*editor.LexingRuleSet[rune, uint32, uint32],
			func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) SublimeContext,
		) func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, SublimeContext] {
			return func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, SublimeContext] {
				return nil
			}
		}
	}

	editorRuleset := LexerSpecToEditorLexingRuleSet(compileResult.CompiledLexerSpec)

	manifestUint := SemanticManifestRemapFromStrings(compileResult.CompiledSymbols, manifest)
	ctxProducer, scopeCov := BuildContextProducerFromManifestWithCoverage[rune, uint32, uint32, string](manifestUint)

	overrideProducer := overrideFactory(editorRuleset, ctxProducer)
	if overrideProducer == nil {
		overrideProducer = func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, SublimeContext] {
			return nil
		}
	}

	irConfig := editor.EditorIRConfigurationCreate(
		hasher,
		func(token lexarch.TokenKind) uint64 {
			return uint64(token)
		},
		func(nodeKind uint32) uint64 {
			return uint64(nodeKind)
		},
		ctxProducer,
		overrideProducer,
		func(left, right SublimeContext) bool {
			return left == right
		},
	)

	runnerCfg := &SublimeRunnerConfig[rune, uint32, uint32, string, uint32]{
		LexerRuleset:   editorRuleset,
		GrammarPackage: &compileResult.CompiledGrammarPackage,
		IRConfig:       irConfig,
		FileExtensions: fileExtensions,
		BaseScope:      fmt.Sprintf("source%s", scopeExtension),
		OutputPaths:    outputPaths,
		ScopeSuffix:    scopeExtension,
	}

	err := RunSublimeGenerator(runnerCfg)
	for _, w := range UnusedManifestScopeWarningsFromStringManifest(compileResult.CompiledSymbols, manifest, scopeCov, UnusedManifestScopeOpts{
		WarnUnusedBaseTokenScopes: true,
	}) {
		fmt.Fprintln(os.Stderr, "langspec sublime:", w)
	}
	return err
}

/* LexerSpecToEditorLexingRuleSet flattens all lexer states into one editor ruleset (LexerState + stack metadata preserved). */
func LexerSpecToEditorLexingRuleSet(
	spec *langspec.LexerSpec[rune, uint32, uint32, string],
) *editor.LexingRuleSet[rune, uint32, uint32] {
	if spec == nil {
		return editor.LexingRuleSetCreate[rune, uint32, uint32]()
	}
	var out []editor.LexingRule[rune, uint32, uint32]
	for _, st := range spec.SortedStateKeys() {
		rs := spec.Ruleset(st)
		for _, ro := range langspec.LexerRulesetGetRules(rs) {
			out = append(out, editor.LexingRule[rune, uint32, uint32]{
				Token:          ro.Token,
				Role:           ro.Role,
				Pattern:        ro.Pattern,
				Priority:       ro.Priority,
				LexerState:     st,
				StackKind:      ro.StackKind,
				StackTargets:   append([]string(nil), ro.StackStates...),
				StackPopAmount: ro.StackPopAmount,
			})
		}
	}
	return editor.LexingRuleSetCreate(out...)
}

func convertRulesetToEditor(
	ruleset langspec.LexerRuleset[uint32, uint32],
) *editor.LexingRuleSet[rune, uint32, uint32] {
	sourceRules := langspec.LexerRulesetGetRules(ruleset)
	out := make([]editor.LexingRule[rune, uint32, uint32], 0, len(sourceRules))
	for _, rule := range sourceRules {
		out = append(out, editor.LexingRule[rune, uint32, uint32]{
			Token:          rule.Token,
			Role:           rule.Role,
			Pattern:        rule.Pattern,
			Priority:       rule.Priority,
			LexerState:     "INITIAL",
			StackKind:      rule.StackKind,
			StackTargets:   append([]string(nil), rule.StackStates...),
			StackPopAmount: rule.StackPopAmount,
		})
	}
	return editor.LexingRuleSetCreate(out...)
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
func RunSublimeGenerator[TObservation cmp.Ordered, TToken ~uint32, TTokenRole, TLexerState, TNodeKind comparable](
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
			os.Stderr,
		)
		if genErr != nil {
			errs = append(errs, fmt.Errorf("output path %s: %w", outputPath, genErr))
		}
	}

	return errors.Join(errs...)
}
