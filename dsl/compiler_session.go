package dsl

import "langspec"

func langSpecCompilerSessionGet(compiler *LangSpecCompiler, sourceFile string) *langspec.LangParserSession[rune] {
	if compiler.sessionCache != nil {
		compiler.sessionCache.Reset(sourceFile, nil, false)
		return compiler.sessionCache
	}
	session := langspec.LangParserSessionCreate[rune](sourceFile, nil, false)
	compiler.sessionCache = session
	return session
}
