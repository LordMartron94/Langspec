package dsl

import (
	"fmt"
	"langspec/validation"
)

// ------------------------------------------------------------- TYPES

type ValidationCode string

const (
	VALIDATION_DECLARATION_ALREADY_SEEN    ValidationCode = "V_D001"
	VALIDATION_DECLARATION_NOT_PRESENT     ValidationCode = "V_D002"
	VALIDATION_DECLARATION_DUPLICATE_ENTRY ValidationCode = "V_D003"
)

func (v ValidationCode) String() string {
	return string(v)
}

// ------------------------------------------------------------- STAGES

func getValidationStages() []*ValidationStage {
	stages := []*ValidationStage{
		validation.ASTValidationStageCreate(
			"Declaration Block Validation",
			"Validates whether declaration blocks are correct in terms of presence an uniqueness.",
			0,
			func(ctx *ValidationStageCtx) {
				root := ctx.RootNode
				blocks := root.FindAllKind(LANG_SPEC_DECLARATION_BLOCK_NODE)

				required := []LangSpecLexerTokenType{
					LANG_SPEC_LEXER_KW_LEXER_STATES,
					LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES,
				}
				seenSet := map[LangSpecLexerTokenType]struct{}{}

				for _, block := range blocks {
					identifierNode := block.Children()[0]
					tokenType := identifierNode.Tokens()[0].Token

					if _, seen := seenSet[tokenType]; seen {
						ctx.ReportError(
							VALIDATION_DECLARATION_ALREADY_SEEN.String(),
							fmt.Sprintf("declaration node '%s' is not unique", identifierNode.GetContent(" ")),
							identifierNode,
						)
					} else {
						seenSet[tokenType] = struct{}{}
					}

					seenDeclarationsSet := map[string]struct{}{}
					identifiers := block.FindAllKind(LANG_SPEC_DECLARE_IDENTIFIER_NODE)
					for _, identifier := range identifiers {
						identifierValue, _ := AttributeAs[string](identifier, ATTRIBUTE_LITERAL_STRING_FORMATTED)
						if _, seen := seenDeclarationsSet[identifierValue]; seen {
							ctx.ReportError(
								VALIDATION_DECLARATION_DUPLICATE_ENTRY.String(),
								fmt.Sprintf("duplicate declaration entry '%s'", identifierValue),
								identifierNode,
							)
						} else {
							seenDeclarationsSet[identifierValue] = struct{}{}
						}
					}
				}

				for _, requiredTk := range required {
					if _, seen := seenSet[requiredTk]; !seen {
						ctx.ReportError(
							VALIDATION_DECLARATION_NOT_PRESENT.String(),
							fmt.Sprintf("declaration node '%s' is not present", requiredTk.String()),
							nil,
						)
					}
				}
			},
		),
	}

	return stages
}
