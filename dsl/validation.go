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

	VALIDATION_DUPLICATE_PATTERN_NAME ValidationCode = "V_PAT001"
	VALIDATION_UNRESOLVED_PATTERN_REF ValidationCode = "V_PAT002"
	VALIDATION_EMPTY_PATTERN_EXPRESSION ValidationCode = "V_PAT003"
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
		{
			Name:        "Pattern Validation",
			Description: "Validates pattern section: duplicate declarations and unresolved variable references. Variables must be declared before use.",
			Order:       2,
			Processor: func(ctx *ValidationCtx) {
				allDeclared := make(map[string]struct{})
				for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
					defNameNode := def.FindFirstKind(NodePatternDefName)
					if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
						continue
					}
					allDeclared[string(defNameNode.Tokens()[0].Raw)] = struct{}{}
				}

				declaredSoFar := make(map[string]struct{})
				for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
					defNameNode := def.FindFirstKind(NodePatternDefName)
					if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
						continue
					}
					if !patternDefinitionHasExpression(def) {
						ctx.ReportError(VALIDATION_EMPTY_PATTERN_EXPRESSION.String(), "pattern definition must have an expression", def)
					}
					name := string(defNameNode.Tokens()[0].Raw)
					if _, already := declaredSoFar[name]; already {
						msg := fmt.Sprintf("pattern name '%s' already declared (duplicate)", name)
						ctx.ReportError(VALIDATION_DUPLICATE_PATTERN_NAME.String(), msg, defNameNode)
					} else {
						declaredSoFar[name] = struct{}{}
					}

					for _, varRef := range def.FindAllKind(NodePatternVarRef) {
						tokens := varRef.Tokens()
						if len(tokens) < 2 {
							continue
						}
						refName := string(tokens[1].Raw)
						if _, ok := declaredSoFar[refName]; !ok {
							var msg string
							if _, declaredLater := allDeclared[refName]; declaredLater {
								msg = fmt.Sprintf("pattern reference '%s' used before declaration", refName)
							} else {
								msg = fmt.Sprintf("unresolved pattern reference '%s'", refName)
							}
							ctx.ReportError(VALIDATION_UNRESOLVED_PATTERN_REF.String(), msg, varRef)
						}
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

var patternExpressionKinds = []LangSpecParserNodeKind{
	NodePatternConcat,
	NodePatternAlternation,
	NodePatternVarRef,
	NodePatternRange,
	NodePatternCharLiteral,
	NodePatternStar,
}

func patternDefinitionHasExpression(def *Node) bool {
	for _, c := range def.Children() {
		if slices.Contains(patternExpressionKinds, c.Kind()) {
			return true
		}
	}
	return false
}
