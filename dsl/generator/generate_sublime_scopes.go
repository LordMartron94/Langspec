package generator

import (
	"encoding/json"
	"fmt"
	"foundation/system"
	"langspec/editor/sublime"
)

// ----------------------------------------------------------------- SUBLIME CONFIGURATION

func writeSublimeConfiguration[TToken, TTokenRole, TNodeKind comparable](
	config *GeneratorConfig[TToken, TTokenRole, TNodeKind],
) error {
	if !config.enableSublime || config.sublimeConfigPath == "" {
		return nil
	}

	cfgData := buildSublimeConfigurationData(config)
	return flushConfigurationToFile(config.sublimeConfigPath, cfgData)
}

func buildSublimeConfigurationData[TToken, TTokenRole, TNodeKind comparable](
	config *GeneratorConfig[TToken, TTokenRole, TNodeKind],
) sublime.SublimeConfiguration {
	manifest := sublime.SublimeScopeManifest{
		BaseTokenScopes: make(map[string]string),
		NodeBindings:    make(map[string]sublime.Binding),
		InvalidScope:    config.invalidScope,
	}

	for k, v := range config.baseScopes {
		manifest.BaseTokenScopes[config.tokenFormatter(k)] = v
	}

	for kind, b := range config.nodeBindings {
		binding := sublime.Binding{
			Scopes:      b.Scopes,
			TokenScopes: make(map[string][]string),
		}

		for tok, scopes := range b.TokenScopes {
			binding.TokenScopes[config.tokenFormatter(tok)] = scopes
		}

		manifest.NodeBindings[config.nodeKindFormatter(kind)] = binding
	}

	return sublime.SublimeConfiguration{
		FileExtensions: config.sublimeFileExtensions,
		ScopeExtension: config.sublimeScopeExtension,
		Manifest:       manifest,
	}
}

func flushConfigurationToFile(path string, cfgData sublime.SublimeConfiguration) error {
	data, err := json.MarshalIndent(cfgData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sublime configuration: %w", err)
	}

	if err := system.FileWriteBytes(path, data); err != nil {
		return fmt.Errorf("failed to write sublime configuration file: %w", err)
	}

	return nil
}
