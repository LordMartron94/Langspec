package validation

import (
	"cmp"
	"fmt"
	"foundation/extensions"
	"syntaxa"
)

/* ValidationSeverity indicates the severity of a validation entry. */
type ValidationSeverity uint8

const (
	VALIDATION_SEVERITY_DIAGNOSTIC ValidationSeverity = iota + 1
	VALIDATION_SEVERITY_INFO
	VALIDATION_SEVERITY_WARNING
	VALIDATION_SEVERITY_ERROR
	VALIDATION_SEVERITY_FATAL
)

/* String returns a fixed-width name for the severity (for logging and display). */
func (v ValidationSeverity) String() string {
	switch v {
	case VALIDATION_SEVERITY_DIAGNOSTIC:
		return "DIAGNOSTIC"
	case VALIDATION_SEVERITY_INFO:
		return "INFO"
	case VALIDATION_SEVERITY_WARNING:
		return "WARNING"
	case VALIDATION_SEVERITY_ERROR:
		return "ERROR"
	case VALIDATION_SEVERITY_FATAL:
		return "FATAL"
	default:
		return "UNKNOWN SEVERITY"
	}
}

/* ValidationEntry is a single validation finding (code, message, severity, node span). */
type ValidationEntry[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	Code     string
	Message  string
	Severity ValidationSeverity
	Node     *syntaxa.SyntaxaLSTNode[TNodeKind]
	Start    int
	End      int
	Notes    []string
}

/* StageValidationResult holds all entries produced by one validation stage. */
type StageValidationResult[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	StageName string
	Order     int
	Entries   []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]
}

/* ValidationEntries aggregates results from all stages run in a single LSTValidatorRun. */
type ValidationEntries[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	Results []StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind]
}

func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) getStageResult(stageName string) *StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind] {
	for i := range v.Results {
		if v.Results[i].StageName == stageName {
			return &v.Results[i]
		}
	}
	return nil
}

func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) beginStage(stageName string, order int) {
	v.Results = append(v.Results, StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind]{
		StageName: stageName,
		Order:     order,
		Entries:   []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]{},
	})
}

/* GetStageEntries returns all entries for the given stage name, or nil if the stage was not run. */
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) GetStageEntries(stageName string) []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind] {
	if result := v.getStageResult(stageName); result != nil {
		return result.Entries
	}
	return nil
}

func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) reportForStage(stageName string, entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]) {
	result := v.getStageResult(stageName)
	result.Entries = append(result.Entries, entry)
}

/* CountBySeverity returns the number of entries with the given severity across all stages. */
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) CountBySeverity(severity ValidationSeverity) int {
	amount := 0
	for _, result := range v.Results {
		for _, entry := range result.Entries {
			if entry.Severity == severity {
				amount++
			}
		}
	}
	return amount
}

/* HasErrors returns true if any entry has severity ERROR or higher. */
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) HasErrors() bool {
	for _, result := range v.Results {
		for _, entry := range result.Entries {
			if entry.Severity >= VALIDATION_SEVERITY_ERROR {
				return true
			}
		}
	}
	return false
}

/* HasFatal returns true if any entry has severity FATAL. */
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) HasFatal() bool {
	for _, result := range v.Results {
		for _, entry := range result.Entries {
			if entry.Severity == VALIDATION_SEVERITY_FATAL {
				return true
			}
		}
	}
	return false
}

/* LSTValidationStageContext is passed to each stage processor; it holds the LST root, typed run state, and report helpers. */
type LSTValidationStageContext[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable, TState any] struct {
	RootNode *syntaxa.SyntaxaLSTNode[TNodeKind]
	RunState TState

	NewValidationEntry func(code, message string, severity ValidationSeverity, node *syntaxa.SyntaxaLSTNode[TNodeKind]) ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]

	ReportDiagnostic func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind])
	ReportInfo       func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind])
	ReportWarning    func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind])
	ReportError      func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind])
	ReportFatal      func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind])

	ReportValidationEntry func(entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind])
}

/* LSTValidationStageProcessor is the function type for a single validation stage; it receives the context including RunState. */
type LSTValidationStageProcessor[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable, TState any] func(
	ctx *LSTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind, TState],
)

/* LSTValidationStageSummarizer is called after each stage with the stage and its entries (e.g. for logging). */
type LSTValidationStageSummarizer[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable, TState any] func(
	stage *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState],
	entries []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind],
)

/* LSTValidationStage defines one validation stage (name, order, processor). */
type LSTValidationStage[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable, TState any] struct {
	Name        string
	Description string
	Order       int
	Processor   LSTValidationStageProcessor[TObservation, TToken, TTokenRole, TNodeKind, TState]
}

/* LSTValidationStageCreate builds a validation stage with the given name, description, order, and processor. */
func LSTValidationStageCreate[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable, TState any](
	name, description string,
	order int,
	processor LSTValidationStageProcessor[TObservation, TToken, TTokenRole, TNodeKind, TState],
) *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState] {
	return &LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState]{
		Name:        name,
		Description: description,
		Processor:   processor,
		Order:       order,
	}
}

/* LSTValidatorConfiguration holds the list of stages and optional stage reporter; parameterized by LST types and run state type. */
type LSTValidatorConfiguration[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable, TState any] struct {
	stageReporter LSTValidationStageSummarizer[TObservation, TToken, TTokenRole, TNodeKind, TState]
	stages        []*LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState]
}

/* LSTValidatorConfigurationCreate returns a new validator configuration with no stages. */
func LSTValidatorConfigurationCreate[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable, TState any]() *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind, TState] {
	return &LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind, TState]{
		stageReporter: nil,
		stages:        make([]*LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState], 0),
	}
}

/* WithStageReporter sets the optional summarizer invoked after each stage. */
func (a *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind, TState]) WithStageReporter(
	stageReporter LSTValidationStageSummarizer[TObservation, TToken, TTokenRole, TNodeKind, TState],
) *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind, TState] {
	a.stageReporter = stageReporter
	return a
}

/* WithStages appends the given stages to the configuration (order determines execution). */
func (a *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind, TState]) WithStages(
	stages ...*LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState],
) *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind, TState] {
	a.stages = append(a.stages, stages...)
	return a
}

func (a *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind, TState]) getSortedStages() []*LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState] {
	return extensions.SortedCopyShallow(a.stages, func(a, b *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState]) int {
		return cmp.Compare(a.Order, b.Order)
	})
}

/*
LSTValidatorRun runs a validation configuration on an LST.
stageFilter may be nil to run all stages; otherwise only stages for which it returns true run.
runState is passed to each stage's processor as ctx.RunState (type TState). Stop on first fatal.
*/
func LSTValidatorRun[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable, TState any](
	configuration *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind, TState],
	rootNode *syntaxa.SyntaxaLSTNode[TNodeKind],
	stageFilter func(stage *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind, TState]) bool,
	runState TState,
) (*ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind], error) {
	validationEntries := &ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]{
		Results: make([]StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind], 0),
	}

	validationStages := configuration.getSortedStages()
	for _, stage := range validationStages {
		if stageFilter != nil && !stageFilter(stage) {
			continue
		}

		validationEntries.beginStage(stage.Name, stage.Order)

		validationCtx := buildValidationContextForStage(stage.Name, rootNode, validationEntries, runState)
		stage.Processor(validationCtx)

		if configuration.stageReporter != nil {
			result := validationEntries.GetStageEntries(stage.Name)
			configuration.stageReporter(stage, result)
		}

		if validationEntries.HasFatal() {
			return validationEntries, fmt.Errorf("stopped because of a fatal validation stage")
		}
	}

	return validationEntries, nil
}

func buildValidationContextForStage[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TNodeKind comparable,
	TState any,
](
	stageName string,
	rootNode *syntaxa.SyntaxaLSTNode[TNodeKind],
	sharedValidationEntries *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind],
	runState TState,
) *LSTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind, TState] {
	newValidationEntry := func(code, message string, severity ValidationSeverity, node *syntaxa.SyntaxaLSTNode[TNodeKind]) ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind] {
		return ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]{
			Code:     code,
			Message:  message,
			Severity: severity,
			Node:     node,
		}
	}

	reportValidationEntry := func(entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]) {
		sharedValidationEntries.reportForStage(stageName, entry)
	}

	return &LSTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind, TState]{
		RootNode:           rootNode,
		RunState:           runState,
		NewValidationEntry: newValidationEntry,
		ReportDiagnostic: func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind]) {
			reportValidationEntry(newValidationEntry(code, message, VALIDATION_SEVERITY_DIAGNOSTIC, node))
		},
		ReportInfo: func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind]) {
			reportValidationEntry(newValidationEntry(code, message, VALIDATION_SEVERITY_INFO, node))
		},
		ReportWarning: func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind]) {
			reportValidationEntry(newValidationEntry(code, message, VALIDATION_SEVERITY_WARNING, node))
		},
		ReportError: func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind]) {
			reportValidationEntry(newValidationEntry(code, message, VALIDATION_SEVERITY_ERROR, node))
		},
		ReportFatal: func(code, message string, node *syntaxa.SyntaxaLSTNode[TNodeKind]) {
			reportValidationEntry(newValidationEntry(code, message, VALIDATION_SEVERITY_FATAL, node))
		},
		ReportValidationEntry: reportValidationEntry,
	}
}
