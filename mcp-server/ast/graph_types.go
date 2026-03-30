package ast

import (
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/mod/modfile"
)

// Edge represents a directed import dependency between two files.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// DependencyGraph holds a file-level import graph for a source tree.
type DependencyGraph struct {
	Root  string   `json:"root"`
	Lang  string   `json:"lang"`
	Nodes []string `json:"nodes"`
	Edges []Edge   `json:"edges"`
}

// ImpactEntry represents a file that is affected by a change to a target symbol.
//
// Distance value semantics:
//
//	1, 2, ... = BFS import distance (Go, Bash)
//	-1        = confirmed caller, BFS not available (TypeScript, Python)
//	0 is never produced.
type ImpactEntry struct {
	File     string `json:"file"`
	Distance int    `json:"distance"`
}

// CallSite represents a location in source code where a symbol is called.
type CallSite struct {
	File   string `json:"file"`
	Symbol string `json:"symbol"`
	Line   int    `json:"line"`
}

// langExtension returns the canonical file extension (with dot) for a Language.
func langExtension(lang Language) string {
	switch lang {
	case Go:
		return ".go"
	case TypeScript:
		return ".ts"
	case Python:
		return ".py"
	case Bash:
		return ".sh"
	default:
		return ""
	}
}

// readGoModuleName reads the module name from a go.mod file in rootPath.
// Returns an empty string if go.mod is absent or unreadable.
func readGoModuleName(rootPath string) string {
	data, err := os.ReadFile(filepath.Join(rootPath, "go.mod"))
	if err != nil {
		return ""
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil || f.Module == nil {
		return ""
	}
	return f.Module.Mod.Path
}

// deduplicateEdges removes duplicate Edge entries (same From+To).
func deduplicateEdges(edges []Edge) []Edge {
	seen := make(map[string]bool, len(edges))
	result := make([]Edge, 0, len(edges))
	for _, e := range edges {
		key := e.From + "\x00" + e.To
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}

// ReverseDependencies performs a BFS over the reverse of graph.Edges starting
// from targetFile and returns all files that (transitively) depend on targetFile,
// sorted by BFS distance (ascending), then alphabetically within the same distance.
// The target file itself is never emitted. Returns an empty (non-nil) slice when
// targetFile is not in the graph or has no dependents.
func ReverseDependencies(graph DependencyGraph, targetFile string) []ImpactEntry {
	// Build reverse adjacency: file → files that import it.
	reverseAdj := make(map[string][]string)
	for _, e := range graph.Edges {
		reverseAdj[e.To] = append(reverseAdj[e.To], e.From)
	}

	// BFS starting from targetFile (distance 0, never emitted).
	visited := map[string]bool{targetFile: true}
	queue := []string{targetFile}
	distance := map[string]int{targetFile: 0}
	var results []ImpactEntry

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		currentDist := distance[current]

		for _, dependent := range reverseAdj[current] {
			if visited[dependent] {
				continue
			}
			visited[dependent] = true
			dist := currentDist + 1
			distance[dependent] = dist
			results = append(results, ImpactEntry{File: dependent, Distance: dist})
			queue = append(queue, dependent)
		}
	}

	// Sort by distance ascending, then alphabetically.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Distance != results[j].Distance {
			return results[i].Distance < results[j].Distance
		}
		return results[i].File < results[j].File
	})

	// Ensure non-nil empty slice when there are no results.
	if len(results) == 0 {
		return []ImpactEntry{}
	}

	return results
}
