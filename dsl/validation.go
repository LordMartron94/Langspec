package dsl

import (
	"fmt"
	"langspec/validation"
	"slices"
)

// ------------------------------------------------------------- TYPES

type ValidationCtx = validation.ASTValidationStageContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type ValidationCode string

const (
	VALIDATION_DUPLICATE_TOKEN ValidationCode = "V_R001"

	VALIDATION_UNKNOWN_PRAGMA_KEY   ValidationCode = "V_P001"
	VALIDATION_UNKNOWN_PRAGMA_VALUE ValidationCode = "V_P002"
	VALIDATION_UNKNOWN_META_KEY     ValidationCode = "V_P003"
)

func (v ValidationCode) String() string {
	return string(v)
}

// ------------------------------------------------------------- STAGES

var validPragmaKeys = []string{
	"enable-mode",
}

var validPragmaValues = []string{
	"sublime",
}

var validMetaKeys = []string{
	"scope",
}

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
					value := getStringValue(lexRuleToken)

					if _, seen := tks[value]; seen {
						msg := fmt.Sprintf("token '%s' already declared (duplicate entry)", value)
						ctx.ReportError(VALIDATION_DUPLICATE_TOKEN.String(), msg, lexRuleToken)
					} else {
						tks[value] = struct{}{}
					}
				}
			},
		},
		{
			Name:        "Pragma Validation",
			Description: "Validates all pragmas and meta sections.",
			Order:       1,
			Processor: func(ctx *ValidationCtx) {
				pragmaKeys := ctx.RootNode.FindAllKind(NodePragmaKey)
				pragmaValues := ctx.RootNode.FindAllKind(NodePragmaValue)
				metaKeys := ctx.RootNode.FindAllKind(NodeMetaKey)

				for _, pragmaKey := range pragmaKeys {
					value := string(pragmaKey.Tokens()[0].Raw)
					if !slices.Contains(validPragmaKeys, value) {
						msg := fmt.Sprintf("unknown pragma key '%s'", value)
						ctx.ReportError(VALIDATION_UNKNOWN_PRAGMA_KEY.String(), msg, pragmaKey)
					}
				}

				for _, pragmaValue := range pragmaValues {
					value := getStringValue(pragmaValue)

					if !slices.Contains(validPragmaValues, value) {
						msg := fmt.Sprintf("unknown pragma value '%s'", value)
						ctx.ReportWarning(VALIDATION_UNKNOWN_PRAGMA_VALUE.String(), msg, pragmaValue)
					}
				}

				for _, metaKey := range metaKeys {
					value := string(metaKey.Tokens()[0].Raw)

					if !slices.Contains(validMetaKeys, value) {
						msg := fmt.Sprintf("unknown meta key '%s'", value)
						ctx.ReportDiagnostic(VALIDATION_UNKNOWN_META_KEY.String(), msg, metaKey)
					}
				}
			},
		},
	}

	return stages
}

func getStringValue(node *Node) string {
	value, ok := AttributeAs[string](node, ATTRIBUTE_LITERAL_STRING_FORMATTED)
	if !ok {
		panic(fmt.Errorf("engine error encountered: %v not stored for node %v", ATTRIBUTE_LITERAL_STRING_FORMATTED, node))
	}

	return value
}
