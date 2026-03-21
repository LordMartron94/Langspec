package semantics

import "syntaxa"

/* ProgramRuleName is the required name of the root parse rule in a .lspec file. */
const ProgramRuleName = "PROGRAM"

/*
GrammarPackage is the lowered grammar package type produced when compiling a .lspec file
to string token and node kinds (same as dsl.GrammarPackage).
*/
type GrammarPackage = syntaxa.GrammarPackage[rune, string, string, string, string]
