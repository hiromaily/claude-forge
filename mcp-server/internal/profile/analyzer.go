// detects and caches repository metadata (languages, CI system,
// linter configs, build commands, etc.) for injection into agent prompts as
// Layer 3 context.

package profile

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Language represents a programming language and its usage percentage in the repo.
type Language struct {
	Name       string `json:"name"`
	Percentage int    `json:"percentage"`
}

// RepoProfile holds the detected repository characteristics.
type RepoProfile struct {
	Languages      []Language        `json:"languages"`
	TestFramework  string            `json:"test_framework"`
	CISystem       string            `json:"ci_system"`
	LinterConfigs  []string          `json:"linter_configs"`
	DirConventions map[string]string `json:"dir_conventions"`
	BranchNaming   string            `json:"branch_naming"`
	BuildCommand   string            `json:"build_command"`
	TestCommand    string            `json:"test_command"`
	Monorepo       bool              `json:"monorepo"`
	LastUpdated    time.Time         `json:"last_updated"`
	Staleness      string            `json:"staleness"` // "fresh" or "stale"
}

// RepoProfiler analyses a git repository and caches the result.
type RepoProfiler struct {
	cachePath string
	repoRoot  string
	profile   *RepoProfile
}

// New constructs a RepoProfiler. cachePath is where the JSON cache is stored;
// repoRoot is the absolute path to the repository being analysed.
func New(cachePath, repoRoot string) *RepoProfiler {
	return &RepoProfiler{
		cachePath: cachePath,
		repoRoot:  repoRoot,
	}
}

// AnalyzeOrUpdate returns the cached profile if it is less than 7 days old;
// otherwise it runs detection helpers, saves the result, and returns the new
// profile. All exec failures degrade gracefully (corresponding field is "").
// Only cache I/O errors are surfaced to the caller.
func (p *RepoProfiler) AnalyzeOrUpdate() (*RepoProfile, error) {
	const staleness = 7 * 24 * time.Hour

	cached := p.loadCache()
	if cached != nil && time.Since(cached.LastUpdated) < staleness {
		cached.Staleness = "fresh"
		p.profile = cached

		return cached, nil
	}

	prof := p.analyze()
	prof.LastUpdated = time.Now().UTC()
	prof.Staleness = "fresh"

	if err := p.saveCache(prof); err != nil {
		return prof, fmt.Errorf("saveCache: %w", err)
	}

	p.profile = prof

	return prof, nil
}

// FormatForPrompt returns a human-readable summary of the profile for use as
// Layer 3 context in agent prompts. Returns "" immediately when p.profile is nil.
func (p *RepoProfiler) FormatForPrompt() string {
	if p.profile == nil {
		return ""
	}

	var sb strings.Builder

	// Languages line.
	if len(p.profile.Languages) > 0 {
		parts := make([]string, 0, len(p.profile.Languages))
		for _, l := range p.profile.Languages {
			parts = append(parts, fmt.Sprintf("%s (%d%%)", l.Name, l.Percentage))
		}

		fmt.Fprintf(&sb, "Languages: %s\n", strings.Join(parts, ", "))
	}

	if p.profile.TestFramework != "" {
		fmt.Fprintf(&sb, "Test framework: %s\n", p.profile.TestFramework)
	}

	if p.profile.CISystem != "" {
		fmt.Fprintf(&sb, "CI system: %s\n", p.profile.CISystem)
	}

	if len(p.profile.LinterConfigs) > 0 {
		fmt.Fprintf(&sb, "Linter configs: %s\n", strings.Join(p.profile.LinterConfigs, ", "))
	}

	if p.profile.BuildCommand != "" {
		fmt.Fprintf(&sb, "Build command: %s\n", p.profile.BuildCommand)
	}

	if p.profile.TestCommand != "" {
		fmt.Fprintf(&sb, "Test command: %s\n", p.profile.TestCommand)
	}

	if p.profile.BranchNaming != "" {
		fmt.Fprintf(&sb, "Branch naming: %s\n", p.profile.BranchNaming)
	}

	return sb.String()
}

// analyze runs all detection helpers and assembles a RepoProfile.
// Exec failures leave the corresponding field as "".
func (p *RepoProfiler) analyze() *RepoProfile {
	prof := &RepoProfile{
		DirConventions: map[string]string{},
		LinterConfigs:  []string{},
	}

	prof.Languages = p.detectLanguages()
	prof.TestFramework = p.detectTestFramework()
	prof.CISystem = p.detectCI()
	prof.LinterConfigs = p.detectLinters()
	prof.DirConventions = p.detectDirConventions()
	prof.BranchNaming = p.detectBranchNaming()
	prof.BuildCommand, prof.TestCommand = p.detectCommands()
	prof.Monorepo = p.detectMonorepo()

	return prof
}

// detectLanguages lists all tracked files via `git ls-files` and counts
// extensions. Returns a slice ordered by descending percentage (top 5 max).
func (p *RepoProfiler) detectLanguages() []Language {
	out, err := p.run("git", "ls-files")
	if err != nil || out == "" {
		return []Language{}
	}

	extMap := map[string]string{
		".go":   "Go",
		".ts":   "TypeScript",
		".js":   "JavaScript",
		".py":   "Python",
		".sh":   "Shell",
		".bash": "Shell",
		".zsh":  "Shell",
		".md":   "Markdown",
		".json": "JSON",
		".yaml": "YAML",
		".yml":  "YAML",
	}

	counts := map[string]int{}
	total := 0

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		ext := strings.ToLower(filepath.Ext(line))

		if lang, ok := extMap[ext]; ok {
			counts[lang]++
			total++
		}
	}

	if total == 0 {
		return []Language{}
	}

	return sortedLanguages(counts, total)
}

// sortedLanguages converts a counts map to a sorted Language slice (descending, top 5).
func sortedLanguages(counts map[string]int, total int) []Language {
	type pair struct {
		lang  string
		count int
	}

	pairs := make([]pair, 0, len(counts))
	for lang, cnt := range counts {
		pairs = append(pairs, pair{lang, cnt})
	}

	// Sort descending.
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].count > pairs[j].count
	})

	// Take top 5.
	if len(pairs) > 5 {
		pairs = pairs[:5]
	}

	langs := make([]Language, 0, len(pairs))
	for _, pr := range pairs {
		pct := pr.count * 100 / total
		if pct > 0 {
			langs = append(langs, Language{Name: pr.lang, Percentage: pct})
		}
	}

	return langs
}

// detectTestFramework checks for go.mod (-> "go test") or package.json (-> jest/vitest/mocha).
// Subdir go.mod files (monorepo with Go inside) take precedence over a root-level package.json
// that may belong to a documentation toolchain rather than the primary test suite.
func (p *RepoProfiler) detectTestFramework() string {
	if fileExists(filepath.Join(p.repoRoot, "go.mod")) {
		return "go test"
	}

	// Check subdirs for go.mod (monorepo with Go inside) before falling back to package.json.
	entries, err := os.ReadDir(p.repoRoot)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				if fileExists(filepath.Join(p.repoRoot, e.Name(), "go.mod")) {
					return "go test"
				}
			}
		}
	}

	pkgJSON := filepath.Join(p.repoRoot, "package.json")
	if !fileExists(pkgJSON) {
		return ""
	}

	data, err := os.ReadFile(pkgJSON)
	if err != nil {
		return ""
	}

	return detectNodeTestFramework(string(data))
}

// detectNodeTestFramework reads package.json content and returns the test framework name.
func detectNodeTestFramework(content string) string {
	switch {
	case strings.Contains(content, `"vitest"`):
		return "vitest"
	case strings.Contains(content, `"jest"`):
		return "jest"
	case strings.Contains(content, `"mocha"`):
		return "mocha"
	default:
		return "npm test"
	}
}

// detectCI checks for CI configuration directories/files.
func (p *RepoProfiler) detectCI() string {
	if dirExists(filepath.Join(p.repoRoot, ".github", "workflows")) {
		return "GitHub Actions"
	}

	if dirExists(filepath.Join(p.repoRoot, ".circleci")) {
		return "CircleCI"
	}

	if fileExists(filepath.Join(p.repoRoot, ".travis.yml")) {
		return "Travis CI"
	}

	if fileExists(filepath.Join(p.repoRoot, "Jenkinsfile")) {
		return "Jenkins"
	}

	return ""
}

// detectLinters scans for known linter configuration files.
func (p *RepoProfiler) detectLinters() []string {
	var linters []string

	checks := []struct {
		pattern string
		name    string
	}{
		{".golangci.yml", "golangci-lint"},
		{".golangci.yaml", "golangci-lint"},
		{".eslintrc.js", "eslint"},
		{".eslintrc.cjs", "eslint"},
		{".eslintrc.json", "eslint"},
		{".eslintrc.yml", "eslint"},
		{".eslintrc.yaml", "eslint"},
		{".eslintrc", "eslint"},
		{"pyproject.toml", "python-linters"},
	}

	seen := map[string]bool{}

	for _, c := range checks {
		if fileExists(filepath.Join(p.repoRoot, c.pattern)) && !seen[c.name] {
			linters = append(linters, c.name)
			seen[c.name] = true
		}
	}

	// Also check for go.mod sub-dirs that contain .golangci.yml.
	if !seen["golangci-lint"] {
		entries, err := os.ReadDir(p.repoRoot)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					if fileExists(filepath.Join(p.repoRoot, e.Name(), ".golangci.yml")) ||
						fileExists(filepath.Join(p.repoRoot, e.Name(), ".golangci.yaml")) {
						if !seen["golangci-lint"] {
							linters = append(linters, "golangci-lint")
							seen["golangci-lint"] = true
						}
					}
				}
			}
		}
	}

	return linters
}

// detectDirConventions maps known top-level directory names to descriptions.
func (p *RepoProfiler) detectDirConventions() map[string]string {
	knownDirs := map[string]string{
		"agents":     "agent definitions",
		"scripts":    "shell scripts",
		"tools":      "CLI tools",
		"cmd":        "command entrypoints",
		"pkg":        "shared packages",
		"internal":   "internal packages",
		"api":        "API definitions",
		"docs":       "documentation",
		"test":       "test utilities",
		"testdata":   "test fixtures",
		"vendor":     "vendored dependencies",
		"web":        "frontend assets",
		"static":     "static assets",
		"hooks":      "lifecycle hooks",
		"skills":     "skill definitions",
		".claude":    "Claude configuration",
		".github":    "GitHub configuration",
		"mcp-server": "MCP server implementation",
	}

	result := map[string]string{}

	entries, err := os.ReadDir(p.repoRoot)
	if err != nil {
		return result
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		if desc, ok := knownDirs[e.Name()]; ok {
			result[e.Name()+"/"] = desc
		}
	}

	return result
}

// detectBranchNaming lists remote branches and extracts the most common prefix pattern.
func (p *RepoProfiler) detectBranchNaming() string {
	out, err := p.run("git", "branch", "-r")
	if err != nil || out == "" {
		return ""
	}

	prefixCounts := map[string]int{}

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		// Strip "origin/" prefix.
		if idx := strings.Index(line, "/"); idx >= 0 {
			line = line[idx+1:]
		}

		if slash := strings.Index(line, "/"); slash > 0 {
			prefix := line[:slash]
			prefixCounts[prefix]++
		}
	}

	if len(prefixCounts) == 0 {
		return ""
	}

	// Find most common prefix.
	best := ""
	bestCount := 0

	for prefix, count := range prefixCounts {
		if count > bestCount || (count == bestCount && prefix < best) {
			best = prefix
			bestCount = count
		}
	}

	if best == "" {
		return ""
	}

	return best + "/{name}"
}

// detectCommands checks for a Makefile with build/test targets or package.json scripts.
func (p *RepoProfiler) detectCommands() (buildCmd, testCmd string) {
	buildCmd, testCmd = p.detectMakeCommands()

	if buildCmd == "" || testCmd == "" {
		buildCmd, testCmd = p.detectSubdirMakeCommands(buildCmd, testCmd)
	}

	if buildCmd == "" || testCmd == "" {
		buildCmd, testCmd = p.detectNodeCommands(buildCmd, testCmd)
	}

	return buildCmd, testCmd
}

// detectMakeCommands checks the root Makefile for build/test targets.
func (p *RepoProfiler) detectMakeCommands() (buildCmd, testCmd string) {
	makefilePath := filepath.Join(p.repoRoot, "Makefile")
	if !fileExists(makefilePath) {
		return "", ""
	}

	data, err := os.ReadFile(makefilePath)
	if err != nil {
		return "", ""
	}

	content := string(data)

	if hasMakeTarget(content, "build") {
		buildCmd = "make build"
	}

	if hasMakeTarget(content, "test") {
		testCmd = "make test"
	}

	return buildCmd, testCmd
}

// detectSubdirMakeCommands checks sub-directory Makefiles when root targets are absent.
func (p *RepoProfiler) detectSubdirMakeCommands(buildCmd, testCmd string) (string, string) {
	entries, err := os.ReadDir(p.repoRoot)
	if err != nil {
		return buildCmd, testCmd
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		subMakefile := filepath.Join(p.repoRoot, e.Name(), "Makefile")
		if !fileExists(subMakefile) {
			continue
		}

		data, err := os.ReadFile(subMakefile)
		if err != nil {
			continue
		}

		content := string(data)

		if buildCmd == "" && hasMakeTarget(content, "build") {
			buildCmd = "make build"
		}

		if testCmd == "" && hasMakeTarget(content, "test") {
			testCmd = "make test"
		}
	}

	return buildCmd, testCmd
}

// detectNodeCommands falls back to package.json scripts when Makefile targets are absent.
func (p *RepoProfiler) detectNodeCommands(buildCmd, testCmd string) (string, string) {
	pkgJSON := filepath.Join(p.repoRoot, "package.json")
	if !fileExists(pkgJSON) {
		return buildCmd, testCmd
	}

	data, err := os.ReadFile(pkgJSON)
	if err != nil {
		return buildCmd, testCmd
	}

	content := string(data)

	if buildCmd == "" && strings.Contains(content, `"build"`) {
		buildCmd = "npm run build"
	}

	if testCmd == "" && strings.Contains(content, `"test"`) {
		testCmd = "npm test"
	}

	return buildCmd, testCmd
}

// detectMonorepo checks for multiple go.mod or package.json files at non-root depth.
func (p *RepoProfiler) detectMonorepo() bool {
	out, err := p.run("git", "ls-files")
	if err != nil || out == "" {
		return false
	}

	goModCount := 0
	pkgJSONCount := 0

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		base := filepath.Base(line)
		dir := filepath.Dir(line)

		// Only count non-root occurrences.
		if dir == "." {
			continue
		}

		switch base {
		case "go.mod":
			goModCount++
		case "package.json":
			pkgJSONCount++
		}
	}

	if goModCount > 0 {
		return true
	}

	return pkgJSONCount > 0
}

// run executes a command in the repo root and returns combined stdout output.
// Returns "" on any error — callers degrade gracefully.
func (p *RepoProfiler) run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = p.repoRoot

	var stdout bytes.Buffer

	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return stdout.String(), nil
}

// fileExists returns true if path exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

// dirExists returns true if path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// hasMakeTarget reports whether content contains a Makefile target named target.
func hasMakeTarget(content, target string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Makefile targets start at column 0 and end with ':'.
		if strings.HasPrefix(line, target+":") {
			return true
		}
	}

	return false
}
