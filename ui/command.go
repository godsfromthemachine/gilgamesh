package ui

import (
	"sort"
	"strings"
)

// Command represents a slash command in the REPL.
type Command struct {
	Name        string
	Usage       string // e.g. "/model [name]"
	Category    string // e.g. "Navigation", "Memory", "Sessions"
	Description string
	Handler     func(args string) bool // returns false to exit REPL
}

// CommandRegistry maps slash command names to handlers.
type CommandRegistry struct {
	commands map[string]*Command
	order    []string // insertion order for listing
}

// NewCommandRegistry creates an empty command registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]*Command),
	}
}

// Register adds a command to the registry.
func (r *CommandRegistry) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
	r.order = append(r.order, cmd.Name)
}

// Execute attempts to dispatch a slash command. Returns (handled, shouldExit).
// If the input doesn't start with "/" or isn't a registered command, handled=false.
func (r *CommandRegistry) Execute(input string) (handled bool, shouldExit bool) {
	if !strings.HasPrefix(input, "/") {
		return false, false
	}

	parts := strings.SplitN(input, " ", 2)
	name := strings.TrimPrefix(parts[0], "/")
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	cmd, ok := r.commands[name]
	if !ok {
		return false, false
	}

	cont := cmd.Handler(args)
	return true, !cont
}

// Lookup returns a command by name, or nil if not found.
func (r *CommandRegistry) Lookup(name string) *Command {
	return r.commands[name]
}

// ListByCategory returns commands grouped by category in registration order.
// Categories are returned in the order of their first appearance.
func (r *CommandRegistry) ListByCategory() []CategoryGroup {
	seen := make(map[string]int)
	var groups []CategoryGroup

	for _, name := range r.order {
		cmd := r.commands[name]
		cat := cmd.Category
		if cat == "" {
			cat = "Other"
		}
		idx, ok := seen[cat]
		if !ok {
			idx = len(groups)
			seen[cat] = idx
			groups = append(groups, CategoryGroup{Category: cat})
		}
		groups[idx].Commands = append(groups[idx].Commands, cmd)
	}

	return groups
}

// Names returns all registered command names sorted alphabetically.
func (r *CommandRegistry) Names() []string {
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CategoryGroup holds commands under a single category heading.
type CategoryGroup struct {
	Category string
	Commands []*Command
}
