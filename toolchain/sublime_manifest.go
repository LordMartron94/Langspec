package toolchain

import (
	"cmp"
	"encoding/json"
	"fmt"
	"foundation/system"
	"langspec/dsl/semantics"
	"langspec/editor"
	"strconv"
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
			out.BaseTokenScopes[resolveManifestTokenID(sym, name)] = scope
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
					nb.TokenScopes[resolveManifestTokenID(sym, tokName)] = scopes
				}
			}
			out.NodeBindings[resolveManifestNodeID(sym, nodeName)] = nb
		}
	}
	return out
}

func resolveManifestTokenID(sym *semantics.CompiledSymbolTable, key string) uint32 {
	if id, err := strconv.ParseUint(strings.TrimSpace(key), 10, 32); err == nil {
		return uint32(id)
	}
	return sym.TokenID(key)
}

func resolveManifestNodeID(sym *semantics.CompiledSymbolTable, key string) uint32 {
	if id, err := strconv.ParseUint(strings.TrimSpace(key), 10, 32); err == nil {
		return uint32(id)
	}
	return sym.NodeKindID(key)
}

// ----------------------------------------------------------------- CONTEXT PRODUCER

// NodeBindingUsage records how often manifest node bindings were exercised while
// building editor IR (context producer calls during Sublime generation).
type NodeBindingUsage struct {
	ResolveHit      bool
	NestMetaHit     bool
	ScopesPath      bool
	TokenScopesPath bool
}

// ManifestScopeCoverage accumulates manifest lookups during context production.
// Used to emit unused-manifest warnings after RunSublimeGenerator.
type ManifestScopeCoverage[TToken, TNodeKind comparable] struct {
	NodeBinding map[TNodeKind]*NodeBindingUsage
	BaseToken   map[TToken]struct{}
	InvalidHit  bool
}

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
	return buildContextProducerFromManifest[TObservation, TToken, TTokenRole, TLexerState, TNodeKind](manifest, nil)
}

/*
BuildContextProducerFromManifestWithCoverage is like BuildContextProducerFromManifest but
records which manifest entries were used. After Sublime generation, pass the coverage
and manifest to UnusedManifestScopeWarnings (or UnusedManifestScopeWarningsFromStringManifest
for JSON manifests) to print diagnostics for unused scopes.
*/
func BuildContextProducerFromManifestWithCoverage[
	TObservation cmp.Ordered,
	TToken comparable,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
](
	manifest SemanticManifest[TToken, TNodeKind],
) (
	func(ctx *editor.EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) SublimeContext,
	*ManifestScopeCoverage[TToken, TNodeKind],
) {
	cov := &ManifestScopeCoverage[TToken, TNodeKind]{}
	return buildContextProducerFromManifest[TObservation, TToken, TTokenRole, TLexerState, TNodeKind](manifest, cov), cov
}

func buildContextProducerFromManifest[
	TObservation cmp.Ordered,
	TToken comparable,
	TTokenRole comparable,
	TLexerState comparable,
	TNodeKind comparable,
](
	manifest SemanticManifest[TToken, TNodeKind],
	cov *ManifestScopeCoverage[TToken, TNodeKind],
) func(ctx *editor.EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) SublimeContext {
	return func(ctx *editor.EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) SublimeContext {
		if ctx.IsInvalidContext {
			if cov != nil && strings.TrimSpace(manifest.InvalidScope) != "" {
				cov.InvalidHit = true
			}
			return SublimeContext{Scope: manifest.InvalidScope}
		}

		if ctx.IsNest {
			if ctx.NodeKind != nil {
				if ms, ok := manifestNestMetaOverride(manifest, *ctx.NodeKind); ok {
					manifestScopeCoverageRecordNestMeta(cov, *ctx.NodeKind)
					return SublimeContext{MetaScope: ms}
				}
			}
			return SublimeContext{MetaScope: nestLabelToMetaScope(string(ctx.NestLabel))}
		}

		return resolveManifestContext(ctx, manifest, cov)
	}
}

// UnusedManifestScopeOpts configures unused-manifest diagnostics.
type UnusedManifestScopeOpts struct {
	// WarnUnusedBaseTokenScopes reports base_token_scopes keys that never matched a
	// transition without a stronger node binding (can be noisy for large manifests).
	WarnUnusedBaseTokenScopes bool
}

/*
UnusedManifestScopeWarnings compares manifest entries to coverage from
BuildContextProducerFromManifestWithCoverage and returns human-readable issues.
nameNodeKind / nameToken should return stable display names (e.g. sym.NodeKindName).
*/
func UnusedManifestScopeWarnings[
	TToken, TNodeKind comparable,
](
	manifest SemanticManifest[TToken, TNodeKind],
	cov *ManifestScopeCoverage[TToken, TNodeKind],
	nameNodeKind func(TNodeKind) string,
	nameToken func(TToken) string,
	opts UnusedManifestScopeOpts,
) []string {
	if cov == nil {
		return nil
	}
	var out []string

	if strings.TrimSpace(manifest.InvalidScope) != "" && !cov.InvalidHit {
		out = append(out, fmt.Sprintf(
			"invalid_scope %q is never used: no invalid editor context was produced while building IR",
			strings.TrimSpace(manifest.InvalidScope),
		))
	}

	for nk, binding := range manifest.NodeBindings {
		label := nameNodeKind(nk)
		u := manifestScopeCoverageNodeGet(cov, nk)
		used := u != nil && (u.ResolveHit || u.NestMetaHit)
		if !used {
			out = append(out, fmt.Sprintf(
				"node_bindings[%q]: unused — no transition resolved to this output node kind (or nest meta) while building IR",
				label,
			))
			continue
		}
		if len(binding.Scopes) > 0 && u.ResolveHit && !u.ScopesPath && u.TokenScopesPath {
			out = append(out, fmt.Sprintf(
				"node_bindings[%q]: scopes [...] never applied — token_scopes always matched instead",
				label,
			))
		}
		if len(binding.TokenScopes) > 0 && u.ResolveHit && !u.TokenScopesPath {
			out = append(out, fmt.Sprintf(
				"node_bindings[%q]: token_scopes never matched — transitions used scopes/meta_scope only (check token kinds vs manifest keys)",
				label,
			))
		}
	}

	if opts.WarnUnusedBaseTokenScopes {
		for tok, scope := range manifest.BaseTokenScopes {
			if strings.TrimSpace(scope) == "" {
				continue
			}
			if !manifestScopeCoverageBaseTokenHit(cov, tok) {
				out = append(out, fmt.Sprintf(
					"base_token_scopes[%q]: unused — no transition fell back to this base scope (node_bindings may cover these tokens)",
					nameToken(tok),
				))
			}
		}
	}

	return out
}

/*
UnusedManifestScopeWarningsFromStringManifest is like UnusedManifestScopeWarnings for the
JSON toolchain: manifest keys are strings; cov uses uint32 IDs from SemanticManifestRemapFromStrings.
*/
func UnusedManifestScopeWarningsFromStringManifest(
	sym *semantics.CompiledSymbolTable,
	manifest SemanticManifest[string, string],
	cov *ManifestScopeCoverage[uint32, uint32],
	opts UnusedManifestScopeOpts,
) []string {
	if sym == nil || cov == nil {
		return nil
	}
	idToNodeName := make(map[uint32]string, len(manifest.NodeBindings))
	for k := range manifest.NodeBindings {
		id := resolveManifestNodeID(sym, k)
		if _, ok := idToNodeName[id]; !ok {
			idToNodeName[id] = k
		}
	}
	nameNode := func(id uint32) string {
		if s, ok := idToNodeName[id]; ok {
			return s
		}
		return sym.NodeKindName(id)
	}
	idToTokName := make(map[uint32]string, len(manifest.BaseTokenScopes))
	for k := range manifest.BaseTokenScopes {
		id := resolveManifestTokenID(sym, k)
		if _, ok := idToTokName[id]; !ok {
			idToTokName[id] = k
		}
	}
	nameTok := func(id uint32) string {
		if s, ok := idToTokName[id]; ok {
			return s
		}
		return sym.TokenName(id)
	}
	remapped := SemanticManifestRemapFromStrings(sym, manifest)
	return UnusedManifestScopeWarnings(remapped, cov, nameNode, nameTok, opts)
}

func manifestScopeCoverageNodeGet[TToken, TNodeKind comparable](cov *ManifestScopeCoverage[TToken, TNodeKind], nk TNodeKind) *NodeBindingUsage {
	if cov == nil || cov.NodeBinding == nil {
		return nil
	}
	return cov.NodeBinding[nk]
}

func manifestScopeCoverageRecordNestMeta[TToken, TNodeKind comparable](cov *ManifestScopeCoverage[TToken, TNodeKind], nk TNodeKind) {
	if cov == nil {
		return
	}
	if cov.NodeBinding == nil {
		cov.NodeBinding = make(map[TNodeKind]*NodeBindingUsage)
	}
	u := cov.NodeBinding[nk]
	if u == nil {
		u = &NodeBindingUsage{}
		cov.NodeBinding[nk] = u
	}
	u.NestMetaHit = true
}

func manifestScopeCoverageRecordResolve[TToken, TNodeKind comparable](cov *ManifestScopeCoverage[TToken, TNodeKind], nk TNodeKind, scopesPath, tokenScopesPath bool) {
	if cov == nil {
		return
	}
	if cov.NodeBinding == nil {
		cov.NodeBinding = make(map[TNodeKind]*NodeBindingUsage)
	}
	u := cov.NodeBinding[nk]
	if u == nil {
		u = &NodeBindingUsage{}
		cov.NodeBinding[nk] = u
	}
	u.ResolveHit = true
	if scopesPath {
		u.ScopesPath = true
	}
	if tokenScopesPath {
		u.TokenScopesPath = true
	}
}

func manifestScopeCoverageRecordBaseToken[TToken, TNodeKind comparable](cov *ManifestScopeCoverage[TToken, TNodeKind], tok TToken) {
	if cov == nil {
		return
	}
	if cov.BaseToken == nil {
		cov.BaseToken = make(map[TToken]struct{})
	}
	cov.BaseToken[tok] = struct{}{}
}

func manifestScopeCoverageBaseTokenHit[TToken, TNodeKind comparable](cov *ManifestScopeCoverage[TToken, TNodeKind], tok TToken) bool {
	if cov == nil || cov.BaseToken == nil {
		return false
	}
	_, ok := cov.BaseToken[tok]
	return ok
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
	cov *ManifestScopeCoverage[TToken, TNodeKind],
) SublimeContext {
	if ctx.NodeKind != nil {
		if resolvedCtx, ok := resolveNodeBinding(*ctx.NodeKind, ctx.Token, manifest, cov); ok {
			return resolvedCtx
		}
	}

	if ctx.Token != nil {
		if scope, ok := manifest.BaseTokenScopes[*ctx.Token]; ok && scope != "" {
			manifestScopeCoverageRecordBaseToken(cov, *ctx.Token)
			return SublimeContext{Scope: scope}
		}
	}

	return SublimeContext{}
}

func resolveNodeBinding[TToken, TNodeKind comparable](
	nodeKind TNodeKind,
	token *TToken,
	manifest SemanticManifest[TToken, TNodeKind],
	cov *ManifestScopeCoverage[TToken, TNodeKind],
) (SublimeContext, bool) {
	binding, exists := manifest.NodeBindings[nodeKind]
	if !exists {
		return SublimeContext{}, false
	}

	ctx := SublimeContext{MetaScope: binding.MetaScope}

	if token != nil && len(binding.TokenScopes) > 0 {
		if ts, match := binding.TokenScopes[*token]; match && len(ts) > 0 {
			ctx.Scope = joinSublimeScopes(ts)
			manifestScopeCoverageRecordResolve(cov, nodeKind, false, true)
			return ctx, true
		}
	}

	if len(binding.Scopes) > 0 {
		ctx.Scope = joinSublimeScopes(binding.Scopes)
		manifestScopeCoverageRecordResolve(cov, nodeKind, true, false)
		return ctx, true
	}

	if ctx.MetaScope != "" {
		manifestScopeCoverageRecordResolve(cov, nodeKind, false, false)
		return ctx, true
	}

	return SublimeContext{}, false
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
