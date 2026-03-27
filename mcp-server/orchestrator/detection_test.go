// Package orchestrator provides pure-logic pipeline orchestration for the forge-state MCP server.
package orchestrator

import (
	"testing"
)

func TestDetectTaskType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		flagTaskType string
		jiraType     string
		githubLabels []string
		text         string
		want         string
	}{
		{name: "flag_override_wins_over_all", flagTaskType: "refactor", jiraType: "Bug", githubLabels: []string{"enhancement"}, text: "fix some bugs", want: "refactor"},
		{name: "jira_bug", jiraType: "Bug", want: "bugfix"},
		{name: "jira_story", jiraType: "Story", want: "feature"},
		{name: "jira_documentation", jiraType: "Documentation", want: "docs"},
		{name: "jira_epic_defaults_feature", jiraType: "Epic", want: "feature"},
		{name: "jira_task_defaults_feature", jiraType: "Task", want: "feature"},
		{name: "jira_unknown_falls_through", jiraType: "UnknownType", want: "feature"},
		{name: "jira_wins_over_github_labels", jiraType: "Bug", githubLabels: []string{"enhancement"}, want: "bugfix"},
		{name: "github_label_bug", githubLabels: []string{"bug"}, want: "bugfix"},
		{name: "github_label_bug_substring", githubLabels: []string{"type:bug"}, want: "bugfix"},
		{name: "github_label_enhancement", githubLabels: []string{"enhancement"}, want: "feature"},
		{name: "github_label_refactor", githubLabels: []string{"refactor"}, want: "refactor"},
		{name: "github_label_investigation", githubLabels: []string{"investigation"}, want: "investigation"},
		{name: "github_label_feature", githubLabels: []string{"feature"}, want: "feature"},
		{name: "github_label_research", githubLabels: []string{"research"}, want: "investigation"},
		{name: "text_heuristic_bugfix", text: "fix the crash bug in login", want: "bugfix"},
		{name: "text_heuristic_docs", text: "update documentation for the API", want: "docs"},
		{name: "default_empty_inputs", want: "feature"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := DetectTaskType(tc.flagTaskType, tc.jiraType, tc.githubLabels, tc.text)
			if got != tc.want {
				t.Errorf("DetectTaskType(%q, %q, %v, %q) = %q, want %q",
					tc.flagTaskType, tc.jiraType, tc.githubLabels, tc.text, got, tc.want)
			}
		})
	}
}

func TestDetectEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		flagEffort  string
		storyPoints int
		text        string
		want        string
	}{
		{name: "flag_override_wins", flagEffort: "L", storyPoints: 1, want: "L"},
		{name: "story_points_1_xs", storyPoints: 1, want: "XS"},
		{name: "story_points_2_s", storyPoints: 2, want: "S"},
		{name: "story_points_4_s_boundary", storyPoints: 4, want: "S"},
		{name: "story_points_5_m", storyPoints: 5, want: "M"},
		{name: "story_points_12_m_boundary", storyPoints: 12, want: "M"},
		{name: "story_points_13_l", storyPoints: 13, want: "L"},
		{name: "story_points_100_l", storyPoints: 100, want: "L"},
		{name: "story_points_zero_default", storyPoints: 0, want: "M"},
		{name: "story_points_negative_default", storyPoints: -1, want: "M"},
		{name: "default_empty_inputs", want: "M"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := DetectEffort(tc.flagEffort, tc.storyPoints, tc.text)
			if got != tc.want {
				t.Errorf("DetectEffort(%q, %d, %q) = %q, want %q",
					tc.flagEffort, tc.storyPoints, tc.text, got, tc.want)
			}
		})
	}
}

func TestDeriveFlowTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		taskType string
		effort   string
		want     string
	}{
		// feature row
		{taskType: "feature", effort: "XS", want: "lite"},
		{taskType: "feature", effort: "S", want: "light"},
		{taskType: "feature", effort: "M", want: "standard"},
		{taskType: "feature", effort: "L", want: "full"},
		// bugfix row
		{taskType: "bugfix", effort: "XS", want: "direct"},
		{taskType: "bugfix", effort: "S", want: "lite"},
		{taskType: "bugfix", effort: "M", want: "light"},
		{taskType: "bugfix", effort: "L", want: "standard"},
		// refactor row
		{taskType: "refactor", effort: "XS", want: "lite"},
		{taskType: "refactor", effort: "S", want: "light"},
		{taskType: "refactor", effort: "M", want: "standard"},
		{taskType: "refactor", effort: "L", want: "full"},
		// docs row
		{taskType: "docs", effort: "XS", want: "direct"},
		{taskType: "docs", effort: "S", want: "direct"},
		{taskType: "docs", effort: "M", want: "lite"},
		{taskType: "docs", effort: "L", want: "light"},
		// investigation row
		{taskType: "investigation", effort: "XS", want: "lite"},
		{taskType: "investigation", effort: "S", want: "lite"},
		{taskType: "investigation", effort: "M", want: "light"},
		{taskType: "investigation", effort: "L", want: "standard"},
		// unknown combinations default to "standard"
		{taskType: "unknown-type", effort: "XL", want: "standard"},
		{taskType: "feature", effort: "XL", want: "standard"},
	}

	for _, tc := range tests {
		t.Run(tc.taskType+"_"+tc.effort, func(t *testing.T) {
			t.Parallel()

			got := DeriveFlowTemplate(tc.taskType, tc.effort)
			if got != tc.want {
				t.Errorf("DeriveFlowTemplate(%q, %q) = %q, want %q", tc.taskType, tc.effort, got, tc.want)
			}
		})
	}
}
