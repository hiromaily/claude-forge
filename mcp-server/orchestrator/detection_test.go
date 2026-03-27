// Package orchestrator provides pure-logic pipeline orchestration for the forge-state MCP server.
package orchestrator

import (
	"testing"
)

// TestDetectTaskType_FlagOverride verifies flag takes precedence over all other inputs.
func TestDetectTaskType_FlagOverride(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("refactor", "Bug", []string{"enhancement"}, "fix some bugs")
	if got != "refactor" {
		t.Errorf("expected %q, got %q", "refactor", got)
	}
}

// TestDetectTaskType_JiraBug verifies Jira "Bug" maps to "bugfix".
func TestDetectTaskType_JiraBug(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "Bug", nil, "")
	if got != "bugfix" {
		t.Errorf("expected %q, got %q", "bugfix", got)
	}
}

// TestDetectTaskType_JiraStory verifies Jira "Story" maps to "feature".
func TestDetectTaskType_JiraStory(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "Story", nil, "")
	if got != "feature" {
		t.Errorf("expected %q, got %q", "feature", got)
	}
}

// TestDetectTaskType_JiraDocumentation verifies Jira "Documentation" maps to "docs".
func TestDetectTaskType_JiraDocumentation(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "Documentation", nil, "")
	if got != "docs" {
		t.Errorf("expected %q, got %q", "docs", got)
	}
}

// TestDetectTaskType_JiraEpic verifies Jira "Epic" maps to "feature".
func TestDetectTaskType_JiraEpic(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "Epic", nil, "")
	if got != "feature" {
		t.Errorf("expected %q, got %q", "feature", got)
	}
}

// TestDetectTaskType_JiraTask verifies Jira "Task" maps to "feature".
func TestDetectTaskType_JiraTask(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "Task", nil, "")
	if got != "feature" {
		t.Errorf("expected %q, got %q", "feature", got)
	}
}

// TestDetectTaskType_GitHubLabelBug verifies GitHub label containing "bug" maps to "bugfix".
func TestDetectTaskType_GitHubLabelBug(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", []string{"bug"}, "")
	if got != "bugfix" {
		t.Errorf("expected %q, got %q", "bugfix", got)
	}
}

// TestDetectTaskType_GitHubLabelBugWithPrefix verifies GitHub label substring "bug" matches (e.g., "bug-fix").
func TestDetectTaskType_GitHubLabelBugWithPrefix(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", []string{"type:bug"}, "")
	if got != "bugfix" {
		t.Errorf("expected %q, got %q", "bugfix", got)
	}
}

// TestDetectTaskType_GitHubLabelEnhancement verifies GitHub label "enhancement" maps to "feature".
func TestDetectTaskType_GitHubLabelEnhancement(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", []string{"enhancement"}, "")
	if got != "feature" {
		t.Errorf("expected %q, got %q", "feature", got)
	}
}

// TestDetectTaskType_GitHubLabelRefactor verifies GitHub label "refactor" maps to "refactor".
func TestDetectTaskType_GitHubLabelRefactor(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", []string{"refactor"}, "")
	if got != "refactor" {
		t.Errorf("expected %q, got %q", "refactor", got)
	}
}

// TestDetectTaskType_GitHubLabelInvestigation verifies GitHub label "investigation" maps to "investigation".
func TestDetectTaskType_GitHubLabelInvestigation(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", []string{"investigation"}, "")
	if got != "investigation" {
		t.Errorf("expected %q, got %q", "investigation", got)
	}
}

// TestDetectTaskType_GitHubLabelFeature verifies GitHub label "feature" maps to "feature".
func TestDetectTaskType_GitHubLabelFeature(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", []string{"feature"}, "")
	if got != "feature" {
		t.Errorf("expected %q, got %q", "feature", got)
	}
}

// TestDetectTaskType_GitHubLabelResearch verifies GitHub label "research" maps to "investigation".
func TestDetectTaskType_GitHubLabelResearch(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", []string{"research"}, "")
	if got != "investigation" {
		t.Errorf("expected %q, got %q", "investigation", got)
	}
}

// TestDetectTaskType_TextHeuristicBugfix verifies text heuristic detects bugfix.
func TestDetectTaskType_TextHeuristicBugfix(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", nil, "fix the crash bug in login")
	if got != "bugfix" {
		t.Errorf("expected %q, got %q", "bugfix", got)
	}
}

// TestDetectTaskType_TextHeuristicDocs verifies text heuristic detects docs.
func TestDetectTaskType_TextHeuristicDocs(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", nil, "update documentation for the API")
	if got != "docs" {
		t.Errorf("expected %q, got %q", "docs", got)
	}
}

// TestDetectTaskType_Default verifies default is "feature".
func TestDetectTaskType_Default(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "", nil, "")
	if got != "feature" {
		t.Errorf("expected %q, got %q", "feature", got)
	}
}

// TestDetectTaskType_DefaultWithUnknownJira verifies unknown Jira type falls through to default.
func TestDetectTaskType_DefaultWithUnknownJira(t *testing.T) {
	t.Parallel()

	got := DetectTaskType("", "UnknownType", nil, "")
	if got != "feature" {
		t.Errorf("expected %q, got %q", "feature", got)
	}
}

// TestDetectTaskType_Precedence_JiraOverLabels verifies Jira wins over GitHub labels.
func TestDetectTaskType_Precedence_JiraOverLabels(t *testing.T) {
	t.Parallel()

	// Jira "Bug" → "bugfix", GitHub label "enhancement" → "feature"
	// Jira takes precedence.
	got := DetectTaskType("", "Bug", []string{"enhancement"}, "")
	if got != "bugfix" {
		t.Errorf("expected %q, got %q", "bugfix", got)
	}
}

// TestDetectEffort_FlagOverride verifies flag wins over story points.
func TestDetectEffort_FlagOverride(t *testing.T) {
	t.Parallel()

	got := DetectEffort("L", 1, "")
	if got != "L" {
		t.Errorf("expected %q, got %q", "L", got)
	}
}

// TestDetectEffort_StoryPoints1 verifies story point 1 maps to "XS".
func TestDetectEffort_StoryPoints1(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 1, "")
	if got != "XS" {
		t.Errorf("expected %q, got %q", "XS", got)
	}
}

// TestDetectEffort_StoryPoints2 verifies story points 2 maps to "S".
func TestDetectEffort_StoryPoints2(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 2, "")
	if got != "S" {
		t.Errorf("expected %q, got %q", "S", got)
	}
}

// TestDetectEffort_StoryPoints4 verifies story points 4 maps to "S" (boundary).
func TestDetectEffort_StoryPoints4(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 4, "")
	if got != "S" {
		t.Errorf("expected %q, got %q", "S", got)
	}
}

// TestDetectEffort_StoryPoints5 verifies story points 5 maps to "M".
func TestDetectEffort_StoryPoints5(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 5, "")
	if got != "M" {
		t.Errorf("expected %q, got %q", "M", got)
	}
}

// TestDetectEffort_StoryPoints12 verifies story points 12 maps to "M" (boundary).
func TestDetectEffort_StoryPoints12(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 12, "")
	if got != "M" {
		t.Errorf("expected %q, got %q", "M", got)
	}
}

// TestDetectEffort_StoryPoints13 verifies story points 13 maps to "L".
func TestDetectEffort_StoryPoints13(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 13, "")
	if got != "L" {
		t.Errorf("expected %q, got %q", "L", got)
	}
}

// TestDetectEffort_StoryPoints100 verifies large story points map to "L".
func TestDetectEffort_StoryPoints100(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 100, "")
	if got != "L" {
		t.Errorf("expected %q, got %q", "L", got)
	}
}

// TestDetectEffort_StoryPointsZero verifies 0 story points falls through to default.
func TestDetectEffort_StoryPointsZero(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 0, "")
	if got != "M" {
		t.Errorf("expected %q, got %q", "M", got)
	}
}

// TestDetectEffort_StoryPointsNegative verifies negative story points falls through to default.
func TestDetectEffort_StoryPointsNegative(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", -1, "")
	if got != "M" {
		t.Errorf("expected %q, got %q", "M", got)
	}
}

// TestDetectEffort_Default verifies default is "M".
func TestDetectEffort_Default(t *testing.T) {
	t.Parallel()

	got := DetectEffort("", 0, "")
	if got != "M" {
		t.Errorf("expected %q, got %q", "M", got)
	}
}

// TestDeriveFlowTemplate_FeatureXS verifies (feature, XS) → "lite".
func TestDeriveFlowTemplate_FeatureXS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("feature", "XS")
	if got != "lite" {
		t.Errorf("expected %q, got %q", "lite", got)
	}
}

// TestDeriveFlowTemplate_FeatureS verifies (feature, S) → "light".
func TestDeriveFlowTemplate_FeatureS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("feature", "S")
	if got != "light" {
		t.Errorf("expected %q, got %q", "light", got)
	}
}

// TestDeriveFlowTemplate_FeatureM verifies (feature, M) → "standard".
func TestDeriveFlowTemplate_FeatureM(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("feature", "M")
	if got != "standard" {
		t.Errorf("expected %q, got %q", "standard", got)
	}
}

// TestDeriveFlowTemplate_FeatureL verifies (feature, L) → "full".
func TestDeriveFlowTemplate_FeatureL(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("feature", "L")
	if got != "full" {
		t.Errorf("expected %q, got %q", "full", got)
	}
}

// TestDeriveFlowTemplate_BugfixXS verifies (bugfix, XS) → "direct".
func TestDeriveFlowTemplate_BugfixXS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("bugfix", "XS")
	if got != "direct" {
		t.Errorf("expected %q, got %q", "direct", got)
	}
}

// TestDeriveFlowTemplate_BugfixS verifies (bugfix, S) → "lite".
func TestDeriveFlowTemplate_BugfixS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("bugfix", "S")
	if got != "lite" {
		t.Errorf("expected %q, got %q", "lite", got)
	}
}

// TestDeriveFlowTemplate_BugfixM verifies (bugfix, M) → "light".
func TestDeriveFlowTemplate_BugfixM(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("bugfix", "M")
	if got != "light" {
		t.Errorf("expected %q, got %q", "light", got)
	}
}

// TestDeriveFlowTemplate_BugfixL verifies (bugfix, L) → "standard".
func TestDeriveFlowTemplate_BugfixL(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("bugfix", "L")
	if got != "standard" {
		t.Errorf("expected %q, got %q", "standard", got)
	}
}

// TestDeriveFlowTemplate_RefactorXS verifies (refactor, XS) → "lite".
func TestDeriveFlowTemplate_RefactorXS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("refactor", "XS")
	if got != "lite" {
		t.Errorf("expected %q, got %q", "lite", got)
	}
}

// TestDeriveFlowTemplate_RefactorS verifies (refactor, S) → "light".
func TestDeriveFlowTemplate_RefactorS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("refactor", "S")
	if got != "light" {
		t.Errorf("expected %q, got %q", "light", got)
	}
}

// TestDeriveFlowTemplate_RefactorM verifies (refactor, M) → "standard".
func TestDeriveFlowTemplate_RefactorM(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("refactor", "M")
	if got != "standard" {
		t.Errorf("expected %q, got %q", "standard", got)
	}
}

// TestDeriveFlowTemplate_RefactorL verifies (refactor, L) → "full".
func TestDeriveFlowTemplate_RefactorL(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("refactor", "L")
	if got != "full" {
		t.Errorf("expected %q, got %q", "full", got)
	}
}

// TestDeriveFlowTemplate_DocsXS verifies (docs, XS) → "direct".
func TestDeriveFlowTemplate_DocsXS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("docs", "XS")
	if got != "direct" {
		t.Errorf("expected %q, got %q", "direct", got)
	}
}

// TestDeriveFlowTemplate_DocsS verifies (docs, S) → "direct".
func TestDeriveFlowTemplate_DocsS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("docs", "S")
	if got != "direct" {
		t.Errorf("expected %q, got %q", "direct", got)
	}
}

// TestDeriveFlowTemplate_DocsM verifies (docs, M) → "lite".
func TestDeriveFlowTemplate_DocsM(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("docs", "M")
	if got != "lite" {
		t.Errorf("expected %q, got %q", "lite", got)
	}
}

// TestDeriveFlowTemplate_DocsL verifies (docs, L) → "light".
func TestDeriveFlowTemplate_DocsL(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("docs", "L")
	if got != "light" {
		t.Errorf("expected %q, got %q", "light", got)
	}
}

// TestDeriveFlowTemplate_InvestigationXS verifies (investigation, XS) → "lite".
func TestDeriveFlowTemplate_InvestigationXS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("investigation", "XS")
	if got != "lite" {
		t.Errorf("expected %q, got %q", "lite", got)
	}
}

// TestDeriveFlowTemplate_InvestigationS verifies (investigation, S) → "lite".
func TestDeriveFlowTemplate_InvestigationS(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("investigation", "S")
	if got != "lite" {
		t.Errorf("expected %q, got %q", "lite", got)
	}
}

// TestDeriveFlowTemplate_InvestigationM verifies (investigation, M) → "light".
func TestDeriveFlowTemplate_InvestigationM(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("investigation", "M")
	if got != "light" {
		t.Errorf("expected %q, got %q", "light", got)
	}
}

// TestDeriveFlowTemplate_InvestigationL verifies (investigation, L) → "standard".
func TestDeriveFlowTemplate_InvestigationL(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("investigation", "L")
	if got != "standard" {
		t.Errorf("expected %q, got %q", "standard", got)
	}
}

// TestDeriveFlowTemplate_UnknownCombinationDefaultsToStandard verifies unknown defaults to "standard".
func TestDeriveFlowTemplate_UnknownCombinationDefaultsToStandard(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("unknown-type", "XL")
	if got != "standard" {
		t.Errorf("expected %q, got %q", "standard", got)
	}
}

// TestDeriveFlowTemplate_UnknownEffortDefaultsToStandard verifies unknown effort for known type defaults to "standard".
func TestDeriveFlowTemplate_UnknownEffortDefaultsToStandard(t *testing.T) {
	t.Parallel()

	got := DeriveFlowTemplate("feature", "XL")
	if got != "standard" {
		t.Errorf("expected %q, got %q", "standard", got)
	}
}
