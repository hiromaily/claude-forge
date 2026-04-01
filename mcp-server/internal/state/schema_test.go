package state

import "testing"

// acceptsPipelineState is a compile-time helper that confirms PipelineState
// is accepted anywhere State is expected and vice versa (type alias semantics).
func acceptsPipelineState(ps PipelineState) int { return ps.Version }
func acceptsState(s State) int                  { return s.Version }
func acceptsErrorInfo(ei ErrorInfo) string      { return ei.Phase }
func acceptsPhaseError(pe PhaseError) string    { return pe.Phase }

// TestPipelineStateAlias verifies that PipelineState is an alias for State.
// Because PipelineState = State (type alias, not a distinct type), values of
// either type can be passed to functions expecting the other without conversion.
func TestPipelineStateAlias(t *testing.T) {
	t.Parallel()

	s := State{Version: 2, SpecName: "test"}

	// Pass a State where PipelineState is expected.
	if v := acceptsPipelineState(s); v != 2 {
		t.Errorf("acceptsPipelineState(State{Version:2}) = %d, want 2", v)
	}

	ps := PipelineState{Version: 3, SpecName: "alias"}

	// Pass a PipelineState where State is expected.
	if v := acceptsState(ps); v != 3 {
		t.Errorf("acceptsState(PipelineState{Version:3}) = %d, want 3", v)
	}
}

// TestErrorInfoAlias verifies that ErrorInfo is an alias for PhaseError.
func TestErrorInfoAlias(t *testing.T) {
	t.Parallel()

	pe := PhaseError{Phase: "phase-1", Message: "err"}

	// Pass a PhaseError where ErrorInfo is expected.
	if p := acceptsErrorInfo(pe); p != "phase-1" {
		t.Errorf("acceptsErrorInfo(PhaseError{Phase:'phase-1'}) = %q, want %q", p, "phase-1")
	}

	ei := ErrorInfo{Phase: "phase-2", Message: "err2"}

	// Pass an ErrorInfo where PhaseError is expected.
	if p := acceptsPhaseError(ei); p != "phase-2" {
		t.Errorf("acceptsPhaseError(ErrorInfo{Phase:'phase-2'}) = %q, want %q", p, "phase-2")
	}
}
