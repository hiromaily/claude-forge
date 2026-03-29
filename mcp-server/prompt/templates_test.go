package prompt

import "testing"

func TestPatternFilterConstants(t *testing.T) {
	t.Parallel()

	if patternNone != patternFilter(0) {
		t.Errorf("patternNone should be 0, got %d", patternNone)
	}
	if patternCriticalOnly != patternFilter(1) {
		t.Errorf("patternCriticalOnly should be 1, got %d", patternCriticalOnly)
	}
	if patternAll != patternFilter(2) {
		t.Errorf("patternAll should be 2, got %d", patternAll)
	}
}

func TestInclusionRuleFields(t *testing.T) {
	t.Parallel()

	r := inclusionRule{
		Patterns: patternCriticalOnly,
		Friction: true,
		Similar:  false,
	}
	if r.Patterns != patternCriticalOnly {
		t.Errorf("expected patternCriticalOnly, got %d", r.Patterns)
	}
	if !r.Friction {
		t.Error("expected Friction to be true")
	}
	if r.Similar {
		t.Error("expected Similar to be false")
	}
}

func TestAgentRulesArchitect(t *testing.T) {
	t.Parallel()

	rule := agentRules["architect"]
	if rule.Patterns != patternCriticalOnly {
		t.Errorf("architect: expected patternCriticalOnly, got %d", rule.Patterns)
	}
	if rule.Friction {
		t.Error("architect: expected Friction to be false")
	}
	if !rule.Similar {
		t.Error("architect: expected Similar to be true")
	}
}

func TestAgentRulesImplementer(t *testing.T) {
	t.Parallel()

	rule := agentRules["implementer"]
	if rule.Patterns != patternCriticalOnly {
		t.Errorf("implementer: expected patternCriticalOnly, got %d", rule.Patterns)
	}
	if !rule.Friction {
		t.Error("implementer: expected Friction to be true")
	}
	if rule.Similar {
		t.Error("implementer: expected Similar to be false")
	}
}

func TestAgentRulesImplReviewer(t *testing.T) {
	t.Parallel()

	rule := agentRules["impl-reviewer"]
	if rule.Patterns != patternAll {
		t.Errorf("impl-reviewer: expected patternAll, got %d", rule.Patterns)
	}
	if !rule.Friction {
		t.Error("impl-reviewer: expected Friction to be true")
	}
	if rule.Similar {
		t.Error("impl-reviewer: expected Similar to be false")
	}
}

func TestAgentRulesTaskDecomposer(t *testing.T) {
	t.Parallel()

	rule := agentRules["task-decomposer"]
	if rule.Patterns != patternAll {
		t.Errorf("task-decomposer: expected patternAll, got %d", rule.Patterns)
	}
	if rule.Friction {
		t.Error("task-decomposer: expected Friction to be false")
	}
	if !rule.Similar {
		t.Error("task-decomposer: expected Similar to be true")
	}
}

func TestAgentRulesUnknownReturnsZeroValue(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"situation-analyst", "investigator", "verifier", "unknown", ""} {
		rule := agentRules[name]
		zero := inclusionRule{}
		if rule != zero {
			t.Errorf("agent %q: expected zero value inclusionRule, got %+v", name, rule)
		}
	}
}
