package state

// PipelineState is an alias for State, introduced for compatibility
// with the v2 design nomenclature. All field definitions live in state.go.
type PipelineState = State

// ErrorInfo is an alias for PhaseError.
type ErrorInfo = PhaseError
