package toolchain

import (
	"cmp"
	"langspec/editor"
	"langspec/editor/sublime"
	"lexarch"
	"strings"
	"syntaxa"
)

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

type ContextProducerConfig[TToken, TNodeKind comparable] struct {
	InvalidScope string
	GetBaseScope func(token TToken) string
	GetNodeScope func(nodeKind TNodeKind, token *TToken) string
}

func BuildContextProducer[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable](
	cfg ContextProducerConfig[TToken, TNodeKind],
) func(ctx *editor.EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) SublimeContext {
	return func(ctx *editor.EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) SublimeContext {
		if ctx.IsInvalidContext {
			return SublimeContext{Scope: cfg.InvalidScope}
		}

		if ctx.IsNest {
			return SublimeContext{MetaScope: nestLabelToMetaScope(string(ctx.NestLabel))}
		}

		// 1. Node override has highest priority
		if ctx.NodeKind != nil {
			if scope := cfg.GetNodeScope(*ctx.NodeKind, ctx.Token); scope != "" {
				return SublimeContext{Scope: scope}
			}
		}

		// 2. Fallback to base token
		if ctx.Token != nil {
			if scope := cfg.GetBaseScope(*ctx.Token); scope != "" {
				return SublimeContext{Scope: scope}
			}
		}

		return SublimeContext{}
	}
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
