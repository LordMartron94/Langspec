// Package editor provides the editor IR and construction for LangSpec-based editor/IDE integration.
//
// The EditorIR holds tokens (with patterns and priorities), global trivia token IDs, and a map of
// contexts. Each context has a name (e.g. grammar rule ID), boundaries (token-triggered enter/exit),
// and valid paths (sequences of tokens and context references). A start context ID and optional
// metadata providers complete the IR.
//
// Use EditorIRConstruct to build an IR from a syntaxa GrammarPackage and lexarch lexical rules.
// Use EditorIRCreate when you already have tokens, contexts, and start context. For debug dumps,
// use the EditorIRDebugger and EditorIRDebugFormatter types in this package.
package editor
