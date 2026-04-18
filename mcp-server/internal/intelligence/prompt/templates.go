package prompt

type inclusionRule struct {
	Patterns patternFilter
	Friction bool
	Similar  bool
}

type patternFilter int

const (
	patternNone         patternFilter = iota // 0
	patternCriticalOnly                      // 1
	patternAll                               // 2
)

var agentRules = map[string]inclusionRule{
	"architect":       {Patterns: patternCriticalOnly, Friction: false, Similar: true},
	"implementer":     {Patterns: patternCriticalOnly, Friction: true, Similar: false},
	"impl-reviewer":   {Patterns: patternAll, Friction: true, Similar: false},
	"task-decomposer": {Patterns: patternAll, Friction: false, Similar: true},
}
