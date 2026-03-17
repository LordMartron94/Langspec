package toolchain

import (
	"encoding/json"
	"fmt"
	"foundation/bytes"
	"foundation/hash"
	"foundation/system"
	"langspec/dsl"
	"langspec/editor"
	"langspec/editor/sublime"
)

const (
	SublimeToolName             = "sublime"
	SublimeOutputPathKey        = "output-path"
	SublimeConfigurationPathKey = "configuration-path"
	SublimeEnableKey            = "enable"
)

var hasher = hash.XXH3HasherCreateWithSeed(6789)

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

		// 1. Load the new envelope configuration
		sublimeCfg, err := LoadConfigurationFromJSON(configPath)
		if err != nil {
			return err
		}

		if overrideProducer == nil {
			overrideProducer = func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, SublimeContext] {
				return nil
			}
		}

		// 2. Extract the Manifest for the Context Producer
		manifest := sublimeCfg.Manifest

		ctxCfg := ContextProducerConfig[string, string]{
			InvalidScope: manifest.InvalidScope,
			GetBaseScope: func(token string) string {
				return manifest.BaseTokenScopes[token]
			},
			GetNodeScope: func(nodeKind string, token *string) string {
				binding, ok := manifest.NodeBindings[nodeKind]
				if !ok {
					return ""
				}
				if token != nil && len(binding.TokenScopes) > 0 {
					if ts, ok := binding.TokenScopes[*token]; ok && len(ts) > 0 {
						return ts[0]
					}
				}
				if len(binding.Scopes) > 0 {
					return binding.Scopes[0]
				}
				return ""
			},
		}

		irConfig := editor.EditorIRConfigurationCreate(
			hasher,
			func(token string) uint64 {
				return hash.XXH3HasherHash64(hasher, bytes.StringSliceToBytes([]string{token}, 0x00))
			},
			BuildContextProducer[rune, string, string, string](ctxCfg),
			overrideProducer,
			func(left, right SublimeContext) bool {
				return left == right
			},
		)

		ruleset := compileResult.CompiledLexerSpec.Ruleset("default")

		// 3. Populate Runner Config dynamically from the JSON
		runnerCfg := &SublimeRunnerConfig[rune, string, string, string, string]{
			LexerRuleset:   &ruleset,
			GrammarPackage: &compileResult.CompiledGrammarPackage,
			IRConfig:       irConfig,
			FileExtensions: sublimeCfg.FileExtensions,
			BaseScope:      fmt.Sprintf("source%s", sublimeCfg.ScopeExtension),
			OutputPath:     outputPath,
			ScopeSuffix:    sublimeCfg.ScopeExtension,
		}

		return RunSublimeGenerator(runnerCfg)
	}
	return nil
}

func LoadConfigurationFromJSON(configPath string) (*sublime.SublimeConfiguration, error) {
	data, err := system.FileReadAllBytes(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration: %w", err)
	}

	var cfg sublime.SublimeConfiguration
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration JSON: %w", err)
	}
	return &cfg, nil
}
