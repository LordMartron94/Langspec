package validation

import (
	"cmp"
	"fmt"
	"foundation/extensions"
	"syntaxa"
)

type ValidationSeverity uint8

const (
	VALIDATION_SEVERITY_DIAGNOSTIC ValidationSeverity = iota + 1
	VALIDATION_SEVERITY_INFO
	VALIDATION_SEVERITY_WARNING
	VALIDATION_SEVERITY_ERROR
	VALIDATION_SEVERITY_FATAL
)

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

/* ValidationEntry is a single validation error. */
type ValidationEntry[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	// Stable identifier (for tooling, tests, suppression, docs)
	Code string

	// Human message
	Message string

	// How serious this is
	Severity ValidationSeverity

	// Where it happened (prefer LST node — spans can be derived)
	Node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]

	// Optional explicit span override (for tokens, ranges, etc.)
	Start int
	End   int

	// Optional extra context
	Notes []string
}

/* StageValidationResult encapsulates a result of stage validation. */
type StageValidationResult[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	StageName string
	Order     int
	Entries   []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]
}

/* ValidationEntries encapsulates the errors during validation. */
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

func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) beginStage(stage *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind]) {
	v.Results = append(v.Results, StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind]{
		StageName: stage.Name,
		Order:     stage.Order,
		Entries:   []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]{},
	})
}

/*
GetStageEntries returns the entries for a given stage.

If the stage does not exist or doesn't have a result struct associated, it will return nil.
*/
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) GetStageEntries(stageName string) []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind] {
	if result := v.getStageResult(stageName); result != nil {
		return result.Entries
	}

	return nil
}

func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) reportForStage(stage *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind], entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]) {
	result := v.getStageResult(stage.Name)
	result.Entries = append(result.Entries, entry)
}

/* CountBySeverity shows the amount of entries having this severity. */
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

/* HasErrors checks if there are errors (including fatals). */
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

/* HasFatal checks if there are fatal errors. */
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

/* LSTValidationStageContext encapsulates the available functions for a validation stage. */
type LSTValidationStageContext[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	RootNode *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]

	NewValidationEntry func(code, message string, severity ValidationSeverity, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]) ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]

	ReportDiagnostic func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind])
	ReportInfo       func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind])
	ReportWarning    func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind])
	ReportError      func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind])
	ReportFatal      func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind])

	ReportValidationEntry func(entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind])
}

/*
LSTValidationStageProcessor processes a single validation stage.

Errors encountered during validation should be reported using the context.
*/
type LSTValidationStageProcessor[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] func(
	ctx *LSTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind],
)

/*
LSTValidationStageSummarizer summarizes the validation errors from this stage.

This is commonly used for logging and user feedback.
*/
type LSTValidationStageSummarizer[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] func(
	stage *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind],
	entries []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind],
)

/*
LSTValidationStage encapsulates a single validation stage.

Stages with a higher order are executed later (stage 0 executes before stage 100)
*/
type LSTValidationStage[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	Name        string
	Description string

	Order int

	Processor LSTValidationStageProcessor[TObservation, TToken, TTokenRole, TNodeKind]
}

/* LSTValidationStageCreate creates an LSTValidationStage instance. */
func LSTValidationStageCreate[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable](
	name, description string,
	order int,
	processor LSTValidationStageProcessor[TObservation, TToken, TTokenRole, TNodeKind],
) *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind] {
	return &LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind]{
		Name:        name,
		Description: description,
		Processor:   processor,
		Order:       order,
	}
}

// -------------------------------------------------------------------- CONFIG

/* LSTValidatorConfiguration encapsulates the configuration for the LST validator. */
type LSTValidatorConfiguration[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	stageReporter LSTValidationStageSummarizer[TObservation, TToken, TTokenRole, TNodeKind]

	stages []*LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind]
}

/* LSTValidatorConfigurationCreate creates a validation config instance. */
func LSTValidatorConfigurationCreate[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable]() *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind] {
	return &LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind]{
		stageReporter: nil,
		stages:        make([]*LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind], 0),
	}
}

/* WithStageReporter sets the stage reporter. */
func (a *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind]) WithStageReporter(
	stageReporter LSTValidationStageSummarizer[TObservation, TToken, TTokenRole, TNodeKind],
) *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind] {
	a.stageReporter = stageReporter
	return a
}

/* WithStages adds stages to the configuration. */
func (a *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind]) WithStages(
	stages ...*LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind],
) *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind] {
	a.stages = append(a.stages, stages...)
	return a
}

func (a *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind]) getSortedStages() []*LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind] {
	sorted := extensions.SortedCopyShallow(a.stages, func(a, b *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind]) int {
		return cmp.Compare(a.Order, b.Order)
	})
	return sorted
}

// -------------------------------------------------------------------- VALIDATOR

/*
LSTValidatorRun runs a validation configuration on an LST.

It returns the entries and an error if one of the stages reports one or more fatal errors.
If the stage reporter is not nil, it will also call that for each stage.
*/
func LSTValidatorRun[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable](
	configuration *LSTValidatorConfiguration[TObservation, TToken, TTokenRole, TNodeKind],
	rootNode *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind],
) (*ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind], error) {
	validationEntries := &ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]{
		Results: make([]StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind], 0),
	}

	validationStages := configuration.getSortedStages()
	for _, stage := range validationStages {
		validationEntries.beginStage(stage)

		validationCtx := buildValidationContextForStage(stage, rootNode, validationEntries)
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
](
	stage *LSTValidationStage[TObservation, TToken, TTokenRole, TNodeKind],
	rootNode *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind],
	sharedValidationEntries *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind],
) *LSTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind] {
	newValidationEntry := func(code, message string, severity ValidationSeverity, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]) ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind] {
		return ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]{
			Code:     code,
			Message:  message,
			Severity: severity,
			Node:     node,
		}
	}

	reportValidationEntry := func(entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]) {
		sharedValidationEntries.reportForStage(stage, entry)
	}

	return &LSTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind]{
		RootNode:           rootNode,
		NewValidationEntry: newValidationEntry,
		ReportDiagnostic: func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_DIAGNOSTIC, node)
			reportValidationEntry(entry)
		},
		ReportInfo: func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_INFO, node)
			reportValidationEntry(entry)
		},
		ReportWarning: func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_WARNING, node)
			reportValidationEntry(entry)
		},
		ReportError: func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_ERROR, node)
			reportValidationEntry(entry)
		},
		ReportFatal: func(code, message string, node *syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_FATAL, node)
			reportValidationEntry(entry)
		},
		ReportValidationEntry: reportValidationEntry,
	}
}
