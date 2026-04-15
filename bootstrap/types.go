package bootstrap

import (
	"langspec"
	"langspec/editor"
)

type LangParser[TNodeKind ~uint32] = langspec.LangParser[rune, string, uint32, uint32, TNodeKind]

type EditorCtx = editor.EditorCtx[rune, uint32, uint32, string, uint32]

type EditorOverride[TEditorCtx any] = editor.EditorOverride[rune, uint32, uint32, string, uint32, TEditorCtx]
