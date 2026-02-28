package dsl

import (
	"fmt"
	"langspec/validation"
)

// ------------------------------------------------------------- TYPES

type ValidationCtx = validation.ASTValidationStageContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type ValidationCode string

const (
	VALIDATION_DUPLICATE_TOKEN ValidationCode = "V_D001"
)

func (v ValidationCode) String() string {
	return string(v)
}

// ------------------------------------------------------------- STAGES

func getValidationStages() []*ValidationStage {
	stages := []*ValidationStage{
		{
			Name:        "Lex Rule Validation",
			Description: "Validates the lex rule section.",
			Order:       0,
			Processor: func(ctx *ValidationCtx) {
				lexRuleSection := ctx.RootNode.FindFirstKind(NodeLexSection)
				tks := map[string]struct{}{}

				lexRuleTokens := lexRuleSection.FindAllKind(NodeLexRuleTokenName)
				for _, lexRuleToken := range lexRuleTokens {
					value, ok := AttributeAs[string](lexRuleToken, ATTRIBUTE_LITERAL_STRING_FORMATTED)
					if !ok {
						panic(fmt.Errorf("engine error encountered: %v not stored for node %v", ATTRIBUTE_LITERAL_STRING_FORMATTED, lexRuleToken))
					}

					if _, seen := tks[value]; seen {
						msg := fmt.Sprintf("token %s already declared (duplicate entry)", value)
						ctx.ReportError(VALIDATION_DUPLICATE_TOKEN.String(), msg, lexRuleToken)
					} else {
						tks[value] = struct{}{}
					}
				}
			},
		},
	}

	return stages
}
