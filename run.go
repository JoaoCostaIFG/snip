package snip

import (
	"github.com/edouard-claude/snip/internal/cli"
	"github.com/edouard-claude/snip/internal/filter"
)

// Run executes snip the same way cmd/snip does.
// args is argv-shaped (args[0] is the program name); the return value is an exit code.
func Run(args []string) int {
	fs := EmbeddedFilters
	filter.EmbeddedFS = &fs
	return cli.Run(args)
}
