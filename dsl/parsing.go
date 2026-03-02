package dsl

import (
	"langspec"
	"strings"
	"syntaxa"
)

// ATTRIBUTE_LITERAL_STRING_FORMATTED is set on nodes that carry a string literal token
// (e.g. NodeLexRuleTokenName, NodePragmaValue); value is the content with surrounding
// quotes removed. Evaluators should use this attribute rather than parsing token text.
// ATTRIBUTE_CHAR_LITERAL_VALUE is set on NodePatternCharLiteral; value is the decoded
// character (after stripping quotes and resolving \', \\, \n, \r, \t).

const (
	ATTRIBUTE_LITERAL_STRING_FORMATTED = "formattedString"
	ATTRIBUTE_CHAR_LITERAL_VALUE       = "charLiteralValue"
)

/*
unescapeCharLiteralContent interprets escape sequences in the inner content of a
single-quoted char literal (after stripping quotes). Supports \', \\, \n, \r, \t.
Returns the decoded string (typically one rune for a valid char literal).
*/
func unescapeCharLiteralContent(inner string) string {
	var out strings.Builder
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\\' && i+1 < len(inner) {
			switch inner[i+1] {
			case '\'':
				out.WriteByte('\'')
			case '\\':
				out.WriteByte('\\')
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			default:
				out.WriteByte(inner[i+1])
			}
			i++
			continue
		}
		out.WriteByte(inner[i])
	}
	return out.String()
}

// --------------------------------------------------------------- BUILDING

func buildLangSpecDSLParserSpec(programRule Rule) (
	*langspec.ParserSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	Rule,
) {
	finalizationPostProcessor := func(node *Node, finalizationCTX *NodeFinalizationCtx, _ syntaxa.RuleIdentity) {
		lexemes := node.Tokens()
		for _, lexeme := range lexemes {
			if lexeme.Token == TokStringLiteral {
				raw := node.GetContent(" ")
				formatted := strings.Replace(raw, "\"", "", -1)
				finalizationCTX.SetAttribute(node, ATTRIBUTE_LITERAL_STRING_FORMATTED, formatted)
				break
			}
		}
		if node.Kind() == NodePatternCharLiteral && len(lexemes) > 0 && lexemes[0].Token == TokCharLiteral {
			raw := string(lexemes[0].Raw)
			if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
				inner := raw[1 : len(raw)-1]
				unescaped := unescapeCharLiteralContent(inner)
				if unescaped != "" {
					finalizationCTX.SetAttribute(node, ATTRIBUTE_CHAR_LITERAL_VALUE, unescaped)
				}
			}
		}
	}

	grammarPkg := new(syntaxa.GrammarPackage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, LangSpecLexerState])
	*grammarPkg = syntaxa.ProducePackage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, LangSpecLexerState](
		programRule.GetGrammar(),
		"LangSpec DSL",
		"0.0.0",
		&programRule,
		nil,
	)
	parserSpec := langspec.ParserSpecCreate(
		grammarPkg,
		NodeProgram,
		NodeError,
		true, // freeze LST after parse
	)
	parserSpec.WithSkipRoles(
		LANG_SPEC_WHITESPACE_ROLE,
		LANG_SPEC_COMMENT_ROLE,
	)
	parserSpec.WithPostProcessor(finalizationPostProcessor)

	return parserSpec, programRule
}
