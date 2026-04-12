package bootstrap

import "langspec"

type LangParser[TNodeKind ~uint32] = langspec.LangParser[rune, string, uint32, uint32, TNodeKind]
