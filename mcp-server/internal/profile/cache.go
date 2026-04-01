package profile

import (
	"encoding/json"
	"os"
	"time"
)

// cacheJSON is the on-disk representation with RFC3339 LastUpdated.
type cacheJSON struct {
	Languages      []Language        `json:"languages"`
	TestFramework  string            `json:"test_framework"`
	CISystem       string            `json:"ci_system"`
	LinterConfigs  []string          `json:"linter_configs"`
	DirConventions map[string]string `json:"dir_conventions"`
	BranchNaming   string            `json:"branch_naming"`
	BuildCommand   string            `json:"build_command"`
	TestCommand    string            `json:"test_command"`
	Monorepo       bool              `json:"monorepo"`
	LastUpdated    string            `json:"last_updated"`
	Staleness      string            `json:"staleness"`
}

// loadCache reads cachePath and deserialises the JSON. Returns nil on any I/O
// or parse error — callers should treat nil as "no valid cache".
func (p *RepoProfiler) loadCache() *RepoProfile {
	if p.cachePath == "" {
		return nil
	}

	data, err := os.ReadFile(p.cachePath)
	if err != nil {
		return nil
	}

	var cj cacheJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return nil
	}

	t, err := time.Parse(time.RFC3339, cj.LastUpdated)
	if err != nil {
		return nil
	}

	prof := &RepoProfile{
		Languages:      cj.Languages,
		TestFramework:  cj.TestFramework,
		CISystem:       cj.CISystem,
		LinterConfigs:  cj.LinterConfigs,
		DirConventions: cj.DirConventions,
		BranchNaming:   cj.BranchNaming,
		BuildCommand:   cj.BuildCommand,
		TestCommand:    cj.TestCommand,
		Monorepo:       cj.Monorepo,
		LastUpdated:    t,
		Staleness:      cj.Staleness,
	}

	return prof
}

// saveCache serialises prof to JSON and writes it to cachePath.
// LastUpdated is formatted as time.RFC3339.
func (p *RepoProfiler) saveCache(prof *RepoProfile) error {
	cj := cacheJSON{
		Languages:      prof.Languages,
		TestFramework:  prof.TestFramework,
		CISystem:       prof.CISystem,
		LinterConfigs:  prof.LinterConfigs,
		DirConventions: prof.DirConventions,
		BranchNaming:   prof.BranchNaming,
		BuildCommand:   prof.BuildCommand,
		TestCommand:    prof.TestCommand,
		Monorepo:       prof.Monorepo,
		LastUpdated:    prof.LastUpdated.UTC().Format(time.RFC3339),
		Staleness:      prof.Staleness,
	}

	data, err := json.MarshalIndent(cj, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p.cachePath, data, 0o600)
}
