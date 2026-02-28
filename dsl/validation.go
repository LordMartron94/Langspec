package dsl

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
	stages := []*ValidationStage{}

	return stages
}
