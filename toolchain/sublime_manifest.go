package toolchain

import (
	"cmp"
	"encoding/json"
	"fmt"
	"foundation/system"
	"langspec/editor"
	"strings"
)

// ----------------------------------------------------------------- MANIFEST STRUCTS

// NodeBinding defines how a specific AST node maps to Sublime Text scopes.
type NodeBinding[TToken comparable] struct {
	Scopes      []string            `json:"scopes,omitempty"`
	MetaScope   string              `json:"meta_scope,omitempty"`
	TokenScopes map[TToken][]string `json:"token_scopes,omitempty"`
}

// SemanticManifest is a generic container for language syntax highlighting rules.
type SemanticManifest[TToken, TNodeKind comparable] struct {
	InvalidScope    string                            `json:"invalid_scope,omitempty"`
	BaseTokenScopes map[TToken]string                 `json:"base_token_scopes,omitempty"`
	NodeBindings    map[TNodeKind]NodeBinding[TToken] `json:"node_bindings,omitempty"`
}

// SublimeConfiguration wraps the manifest alongside editor-specific file metadata.
type SublimeConfiguration[TToken, TNodeKind comparable] struct {
	FileExtensions []string                            `json:"file_extensions,omitempty"`
	ScopeExtension string                              `json:"scope_extension,omitempty"`
	Manifest       SemanticManifest[TToken, TNodeKind] `json:"scope_manifest"`
}

// ----------------------------------------------------------------- JSON LOADER

/*
LoadSublimeConfigFromJSON reads a JSON file from disk and parses it into a strongly-typed Go configuration.
This allows users to share manifests natively without writing Go code.
*/
func LoadSublimeConfigFromJSON(configPath string) (*SublimeConfiguration[string, string], error) {
	data, err := system.FileReadAllBytes(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var cfg SublimeConfiguration[string, string]
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration JSON: %w", err)
	}

	return &cfg, nil
}

// ----------------------------------------------------------------- CONTEXT PRODUCER

/*
BuildContextProducerFromManifest creates a standard context producer directly from a static manifest.
This replaces the need for clients to write custom GetBaseScope/GetNodeScope callbacks.
*/
func BuildContextProducerFromManifest[
	TObservation cmp.Ordered,
	TToken comparable,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
](
	manifest SemanticManifest[TToken, TNodeKind],
) func(ctx *editor.EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) SublimeContext {
	return func(ctx *editor.EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) SublimeContext {
		if ctx.IsInvalidContext {
			return SublimeContext{Scope: manifest.InvalidScope}
		}

		if ctx.IsNest {
			return SublimeContext{MetaScope: nestLabelToMetaScope(string(ctx.NestLabel))}
		}

		return resolveManifestContext(ctx, manifest)
	}
}

func resolveManifestContext[
	TObservation cmp.Ordered,
	TToken comparable,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
](
	ctx *editor.EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	manifest SemanticManifest[TToken, TNodeKind],
) SublimeContext {
	if ctx.NodeKind != nil {
		if resolvedCtx, ok := resolveNodeBinding(*ctx.NodeKind, ctx.Token, manifest); ok {
			return resolvedCtx
		}
	}

	if ctx.Token != nil {
		if scope, ok := manifest.BaseTokenScopes[*ctx.Token]; ok && scope != "" {
			return SublimeContext{Scope: scope}
		}
	}

	return SublimeContext{}
}

func resolveNodeBinding[TToken, TNodeKind comparable](
	nodeKind TNodeKind,
	token *TToken,
	manifest SemanticManifest[TToken, TNodeKind],
) (SublimeContext, bool) {
	binding, exists := manifest.NodeBindings[nodeKind]
	if !exists {
		return SublimeContext{}, false
	}

	ctx := SublimeContext{MetaScope: binding.MetaScope}

	if token != nil && len(binding.TokenScopes) > 0 {
		if ts, match := binding.TokenScopes[*token]; match && len(ts) > 0 {
			ctx.Scope = ts[0]
			return ctx, true
		}
	}

	if len(binding.Scopes) > 0 {
		ctx.Scope = binding.Scopes[0]
		return ctx, true
	}

	return ctx, ctx.MetaScope != ""
}

// --- UNIVERSAL STRING UTILITIES ---

func applyScopeSuffix(scope, suffix string) string {
	if scope == "" {
		return ""
	}
	return scope + suffix
}

func nestLabelToMetaScope(label string) string {
	if label == "" {
		return ""
	}
	lower := strings.ToLower(label)
	lower = strings.TrimSuffix(lower, "_nest")
	lower = strings.TrimSuffix(lower, " nest")
	lower = strings.ReplaceAll(lower, " ", "-")
	lower = strings.ReplaceAll(lower, "_", "-")
	return "meta." + lower + ".body"
}
