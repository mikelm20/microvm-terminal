// Package lesson owns server-side predicate evaluation. It mirrors the
// discriminated union declared in shared/lessons/schema.ts. The Evaluator
// subscribes to a session EventBus and emits a step_satisfied event (also
// publishing it back to the bus) when the active step's predicate is met.
package lesson

// Predicate is the union the evaluator understands. Unmarshal JSON or YAML
// into this by keeping the shape flat and discriminating on `Type`. The
// Match sub-struct holds all the per-predicate parameters; zero-valued fields
// mean "do not constrain".
type Predicate struct {
	Type       string   `json:"type" yaml:"type"`
	Match      *Match   `json:"match,omitempty" yaml:"match,omitempty"`
	MatchRegex string   `json:"match_regex,omitempty" yaml:"match_regex,omitempty"`
}

// Match aggregates all predicate-specific parameters. Which keys are honored
// depends on Predicate.Type. The spine treats empty strings and zero numbers
// as "do not constrain".
type Match struct {
	Name         string `json:"name,omitempty" yaml:"name,omitempty"`
	Port         int    `json:"port,omitempty" yaml:"port,omitempty"`
	Tool         string `json:"tool,omitempty" yaml:"tool,omitempty"`
	PathRegex    string `json:"path_regex,omitempty" yaml:"path_regex,omitempty"`
	CommandRegex string `json:"command_regex,omitempty" yaml:"command_regex,omitempty"`
	SubagentType string `json:"subagent_type,omitempty" yaml:"subagent_type,omitempty"`
	Command      string `json:"command,omitempty" yaml:"command,omitempty"`
	Path         string `json:"path,omitempty" yaml:"path,omitempty"`
	Regex        string `json:"regex,omitempty" yaml:"regex,omitempty"`
}

// Step is the minimum shape the evaluator needs. Full lesson schema lives in
// shared/lessons/schema.ts; this is the subset persisted server-side.
type Step struct {
	ID      string    `json:"id"`
	Success Predicate `json:"success"`
}

// Lesson is an ordered list of steps the evaluator walks through.
type Lesson struct {
	ID    string `json:"id"`
	Steps []Step `json:"steps"`
}
