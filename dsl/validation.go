package dsl

import (
	"fmt"
	"langspec/validation"
)

// ------------------------------------------------------------- TYPES

type ValidationCtx = validation.LSTValidationStageContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type ValidationCode string

const (
	VALIDATION_DUPLICATE_TOKEN ValidationCode = "V_R001"

	VALIDATION_DUPLICATE_PATTERN_NAME   ValidationCode = "V_PAT001"
	VALIDATION_UNRESOLVED_PATTERN_REF   ValidationCode = "V_PAT002"
	VALIDATION_EMPTY_PATTERN_EXPRESSION ValidationCode = "V_PAT003"
	VALIDATION_LOCAL_REF_OUTSIDE_SECTION ValidationCode = "V_PAT004"
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
			Name:        "Pattern Validation",
			Description: "Validates pattern section: duplicate declarations, unresolved variable references, and local variables used outside the pattern section. Variables must be declared before use; locals may be used in any pattern definition but not outside the pattern section.",
			Order:       1,
			Processor: func(ctx *ValidationCtx) {
				allDeclared := make(map[string]struct{})
				localNameToDef := make(map[string]*Node)
				for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
					defNameNode := def.FindFirstKind(NodePatternDefName)
					if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
						continue
					}
					name := string(defNameNode.Tokens()[0].Raw)
					allDeclared[name] = struct{}{}
					if def.FindFirstKind(NodeLocalVariable) != nil {
						localNameToDef[name] = def
					}
				}

				declaredSoFar := make(map[string]struct{})
				for _, def := range ctx.RootNode.FindAllKind(NodePatternDefinition) {
					defNameNode := def.FindFirstKind(NodePatternDefName)
					if defNameNode == nil || len(defNameNode.Tokens()) == 0 {
						continue
					}

					name := string(defNameNode.Tokens()[0].Raw)
					if _, already := declaredSoFar[name]; already {
						msg := fmt.Sprintf("pattern name '%s' already declared (duplicate)", name)
						ctx.ReportError(VALIDATION_DUPLICATE_PATTERN_NAME.String(), msg, defNameNode)
					} else {
						declaredSoFar[name] = struct{}{}
					}

					for _, varRef := range def.FindAllKind(NodeVarRef) {
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

				for _, varRef := range ctx.RootNode.FindAllKind(NodeVarRef) {
					if enclosingPatternDef(varRef) != nil {
						continue
					}
					tokens := varRef.Tokens()
					if len(tokens) < 2 {
						continue
					}
					refName := string(tokens[1].Raw)
					if _, isLocal := localNameToDef[refName]; isLocal {
						msg := fmt.Sprintf("local variable '%s' referenced outside pattern section (e.g. in lex section)", refName)
						ctx.ReportError(VALIDATION_LOCAL_REF_OUTSIDE_SECTION.String(), msg, varRef)
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

/*
enclosingPatternDef returns the innermost NodePatternDefinition that contains node, or nil if node is not inside any pattern definition. Walks Parent() upward.
*/
func enclosingPatternDef(node *Node) *Node {
	for n := node; n != nil; n = n.Parent() {
		if n.Kind() == NodePatternDefinition {
			return n
		}
	}
	return nil
}
