package editor

import "langspec/dsl"

var dslTokenScopeMap = map[dsl.LangSpecLexerTokenType]string{
	dsl.TokEOF:               "meta.eof",
	dsl.TokWhitespace:        "punctuation.whitespace",
	dsl.TokDashes:            "punctuation.definition.separator",
	dsl.TokHeaderSeparator:   "punctuation.section.header",
	dsl.TokStringLiteral:     "string.quoted.double",
	dsl.TokVersion:           "constant.numeric.version",
	dsl.TokKWLSpec:           "keyword.declaration.lspec",
	dsl.TokKWDeclare:         "keyword.control.declare",
	dsl.TokKWLexerTokenTypes: "meta.type.builtin",
	dsl.TokBraceOpen:         "punctuation.section.braces.begin",
	dsl.TokBraceClose:        "punctuation.section.braces.end",
	dsl.TokSemicolon:         "punctuation.terminator.statement",
	dsl.TokComma:             "punctuation.separator.comma",
	dsl.TokLineComment:       "comment.line.double-slash",
	dsl.TokBlockComment:      "comment.block",
}

func dslTokenScope(token dsl.LangSpecLexerTokenType) string {
	if scope, ok := dslTokenScopeMap[token]; ok {
		return scope
	}
	return ""
}
