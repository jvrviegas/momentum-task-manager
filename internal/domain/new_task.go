package domain

// NewTask contains user-authored values that can be translated to a
// Taskwarrior add command. Date values intentionally remain expressions so
// Taskwarrior performs final date parsing and validation.
type NewTask struct {
	Description string
	Project     string
	Priority    string
	Due         string
	Scheduled   string
	Tags        []string
}
