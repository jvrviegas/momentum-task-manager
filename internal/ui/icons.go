package ui

import (
	"strings"

	"github.com/jvrviegas/momentum/internal/config"
)

// Icons contains the small visual vocabulary used by rows and navigation.
type Icons struct {
	Pending   string
	Completed string
	Active    string
	Overdue   string
	Today     string
	Inbox     string
	Chevron   string
	Search    string
	Sync      string
}

func unicodeIcons() Icons {
	return Icons{Pending: "□", Completed: "✓", Active: "▶", Overdue: "!", Today: "◷", Inbox: "▱", Chevron: "›", Search: "⌕", Sync: "↻"}
}

func nerdIcons() Icons {
	return Icons{Pending: "󰄱", Completed: "󰄬", Active: "󰐊", Overdue: "󰀦", Today: "󰃭", Inbox: "󰇮", Chevron: "󰅂", Search: "󰍉", Sync: "󰑐"}
}

func asciiIcons() Icons {
	return Icons{Pending: "[ ]", Completed: "[x]", Active: ">", Overdue: "!", Today: "D", Inbox: "I", Chevron: ">", Search: "/", Sync: "~"}
}

// IconsFor resolves the explicit configured icon mode; unknown values safely
// fall back to Unicode, matching the no-Nerd-Font default.
func IconsFor(mode string) Icons {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case config.IconsNerd:
		return nerdIcons()
	case config.IconsASCII:
		return asciiIcons()
	default:
		return unicodeIcons()
	}
}
