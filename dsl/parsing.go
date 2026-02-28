package dsl

import (
	"langspec"
	"strings"
	"syntaxa"
)

// --------------------------------------------------------------- ATTRIBUTES (node kinds live in constructs.go)

const (
	ATTRIBUTE_LITERAL_STRING_FORMATTED = "formattedString"
	ATTRIBUTE_RULE_NAME                = "ruleName"
)

// --------------------------------------------------------------- BUILDING

func buildLangSpecDSLParserSpec(programRule Rule) (
	*langspec.ParserSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	Rule,
) {
	finalizationPostProcessor := func(node *Node, finalizationCTX *NodeFinalizationCtx, ruleIdentity syntaxa.RuleIdentity) {
		finalizationCTX.SetAttribute(node, ATTRIBUTE_RULE_NAME, ruleIdentity.RuleName)

		lexemes := node.Tokens()
		for _, lexeme := range lexemes {
			if lexeme.Token == TokStringLiteral {
				raw := node.GetContent(" ")
				formatted := strings.Replace(raw, "\"", "", -1)
				finalizationCTX.SetAttribute(node, ATTRIBUTE_LITERAL_STRING_FORMATTED, formatted)
				break
			}
		}
	}

	parserSpec := langspec.ParserSpecCreate(
		NodeProgram,
		NodeError,
		programRule,
		true, // freeze AST after parse
	)
	parserSpec.WithSkipRoles(
		LANG_SPEC_WHITESPACE_ROLE,
		LANG_SPEC_COMMENT_ROLE,
	)
	parserSpec.WithPostProcessor(finalizationPostProcessor)

	return parserSpec, programRule
}
