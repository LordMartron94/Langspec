package toolchain

import (
	"cmp"
	"encoding/json"
	"fmt"
	"foundation/system"
	"langspec/dsl/semantics"
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

// SublimeConfiguration wraps the manifest alongside editor-specific file
// metadata (file extensions, scope extension). This is the structure decoded
// from the JSON file at configuration-path when using the JSON-driven
// toolchain (RunSublimeToolchain).
type SublimeConfiguration[TToken, TNodeKind comparable] struct {
	FileExtensions []string                            `json:"file_extensions,omitempty"`
	ScopeExtension string                              `json:"scope_extension,omitempty"`
	Manifest       SemanticManifest[TToken, TNodeKind] `json:"scope_manifest"`
}

// ----------------------------------------------------------------- JSON LOADER

/*
LoadSublimeConfigFromJSON reads a JSON file from disk and parses it into a
strongly-typed SublimeConfiguration. Used by RunSublimeToolchain when the
manifest is supplied via PRAGMA configuration-path.

Use cases:
- Loading a shared Sublime config file (e.g. in repo) for the JSON-driven toolchain.
- Decoding a manifest and file metadata without writing Go struct literals.

Prerequisites:
- configPath must point to a readable file containing valid JSON matching SublimeConfiguration (scope_manifest, file_extensions, scope_extension).

Edge cases:
- Returns an error if the file cannot be read or JSON is invalid or does not match the expected structure.
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

/*
SemanticManifestRemapFromStrings maps string-keyed manifest entries (e.g. from JSON) to uint32
keys using the same IDs as the LangSpec compiler’s CompiledSymbolTable.
*/
func SemanticManifestRemapFromStrings(
	sym *semantics.CompiledSymbolTable,
	in SemanticManifest[string, string],
) SemanticManifest[uint32, uint32] {
	out := SemanticManifest[uint32, uint32]{
		InvalidScope: in.InvalidScope,
	}
	if len(in.BaseTokenScopes) > 0 {
		out.BaseTokenScopes = make(map[uint32]string, len(in.BaseTokenScopes))
		for name, scope := range in.BaseTokenScopes {
			out.BaseTokenScopes[sym.TokenID(name)] = scope
		}
	}
	if len(in.NodeBindings) > 0 {
		out.NodeBindings = make(map[uint32]NodeBinding[uint32], len(in.NodeBindings))
		for nodeName, b := range in.NodeBindings {
			nb := NodeBinding[uint32]{
				Scopes:    b.Scopes,
				MetaScope: b.MetaScope,
			}
			if len(b.TokenScopes) > 0 {
				nb.TokenScopes = make(map[uint32][]string, len(b.TokenScopes))
				for tokName, scopes := range b.TokenScopes {
					nb.TokenScopes[sym.TokenID(tokName)] = scopes
				}
			}
			out.NodeBindings[sym.NodeKindID(nodeName)] = nb
		}
	}
	return out
}

// ----------------------------------------------------------------- CONTEXT PRODUCER

/*
BuildContextProducerFromManifest creates a context producer that maps editor
context (token, node kind, invalid/nest state) to SublimeContext using the
given manifest. Used by both JSON-driven and in-memory toolchain paths to build
EditorIRConfiguration.

Use cases:
- Building the context producer when using a SemanticManifest (from JSON or in-memory).
- Avoiding custom GetBaseScope/GetNodeScope callbacks when manifest data is sufficient.

Prerequisites:
- manifest must have BaseTokenScopes and/or NodeBindings populated as needed for the language.

Edge cases:
- Invalid context yields SublimeContext with manifest.InvalidScope.
- Nest wrapper context yields meta-scope from the manifest NodeBinding for the opening
production when that binding has MetaScope; otherwise meta derived from the nest label (.body).
- Unmatched token/node yields zero SublimeContext.
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
			if ctx.NodeKind != nil {
				if ms, ok := manifestNestMetaOverride(manifest, *ctx.NodeKind); ok {
					return SublimeContext{MetaScope: ms}
				}
			}
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
			ctx.Scope = joinSublimeScopes(ts)
			return ctx, true
		}
	}

	if len(binding.Scopes) > 0 {
		ctx.Scope = joinSublimeScopes(binding.Scopes)
		return ctx, true
	}

	return ctx, ctx.MetaScope != ""
}

func manifestNestMetaOverride[TToken, TNodeKind comparable](
	manifest SemanticManifest[TToken, TNodeKind],
	openingNodeKind TNodeKind,
) (string, bool) {
	binding, exists := manifest.NodeBindings[openingNodeKind]
	if !exists {
		return "", false
	}
	ms := strings.TrimSpace(binding.MetaScope)
	if ms == "" {
		return "", false
	}
	return ms, true
}

// --- UNIVERSAL STRING UTILITIES ---

// joinSublimeScopes joins non-empty scopes with spaces (Sublime stacked scopes in one scope: field).
func joinSublimeScopes(scopes []string) string {
	var b strings.Builder
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
	return b.String()
}

// applyScopeSuffix appends suffix to each space-separated scope segment.
func applyScopeSuffix(scope, suffix string) string {
	if scope == "" {
		return ""
	}
	if suffix == "" {
		return scope
	}
	parts := strings.Fields(scope)
	if len(parts) == 0 {
		return ""
	}
	for i := range parts {
		parts[i] = parts[i] + suffix
	}
	return strings.Join(parts, " ")
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
