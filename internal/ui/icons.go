package ui

import (
	"strings"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
)

// Icons is the complete marker vocabulary. Nerd mode swaps only icons;
// chrome (gutters, rules, borders) stays box-drawing in unicode and nerd.
// ASCII is printable ASCII only; navigation icons are empty there because
// the label already carries the meaning.
type Icons struct {
	// Task state.
	Pending   string
	Completed string
	Active    string
	Overdue   string

	// Priority arrows, always paired with a word.
	High   string
	Medium string
	Low    string

	// Navigation.
	Inbox         string
	Today         string
	CompletedView string
	Settings      string

	// Selection and focus. SelectionCont marks the second line of a selected
	// row; it is blank in ASCII.
	Selection     string
	SelectionCont string
	Changed       string
	Error         string

	// Sync states, each paired with a word.
	Synced  string
	Syncing string
	Local   string
	Failed  string
	Off     string

	// Text and prompts.
	Chevron  string
	Search   string
	Prompt   string
	More     string
	Dot      string
	Arrow    string
	Skeleton string

	// Chrome.
	Rule       string
	RuleActive string
	Divider    string
	Thumb      string
	CornerTL   string
	CornerTR   string
	CornerBL   string
	CornerBR   string
	TreeTee    string
	TreeElbow  string
	TreePipe   string
	TabOn      string
	TabOff     string
}

func unicodeIcons() Icons {
	return Icons{
		Pending: "□", Completed: "✓", Active: "▶", Overdue: "!",
		High: "↑", Medium: "→", Low: "↓",
		Inbox: "▱", Today: "◷", CompletedView: "✓", Settings: "⚙",
		Selection: "▌", SelectionCont: "▌", Changed: "•", Error: "!",
		Synced: "✓", Syncing: "↻", Local: "•", Failed: "!", Off: "·",
		Chevron: "›", Search: "⌕", Prompt: "›", More: "↓", Dot: "·", Arrow: "→", Skeleton: "░",
		Rule: "─", RuleActive: "━", Divider: "│", Thumb: "┃",
		CornerTL: "╭", CornerTR: "╮", CornerBL: "╰", CornerBR: "╯",
		TreeTee: "├", TreeElbow: "└", TreePipe: "│", TabOn: "•", TabOff: "·",
	}
}

// nerdIcons uses the Font Awesome set bundled with Nerd Fonts. A "Nerd Font
// Mono" variant keeps each icon one cell wide.
func nerdIcons() Icons {
	icons := unicodeIcons()
	icons.Pending = ""
	icons.Completed = ""
	icons.Active = ""
	icons.Overdue = ""
	icons.High = ""
	icons.Medium = ""
	icons.Low = ""
	icons.Inbox = ""
	icons.Today = ""
	icons.CompletedView = ""
	icons.Settings = ""
	icons.Error = ""
	icons.Synced = ""
	icons.Syncing = ""
	icons.Local = ""
	icons.Failed = ""
	icons.Off = ""
	icons.Chevron = ""
	icons.Search = ""
	icons.Prompt = ""
	icons.More = ""
	return icons
}

func asciiIcons() Icons {
	return Icons{
		Pending: "[ ]", Completed: "[x]", Active: "[>]", Overdue: "!",
		High: "^", Medium: "=", Low: "v",
		Selection: ">", SelectionCont: " ", Changed: "*", Error: "!",
		Synced: "*", Syncing: "~", Local: "*", Failed: "!", Off: "-",
		Chevron: ">", Search: "/", Prompt: ">", More: "v", Dot: "-", Arrow: "->", Skeleton: ".",
		Rule: "-", RuleActive: "=", Divider: "|", Thumb: "#",
		CornerTL: "+", CornerTR: "+", CornerBL: "+", CornerBR: "+",
		TreeTee: "+", TreeElbow: "`", TreePipe: "|", TabOn: "*", TabOff: ".",
	}
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

// orUnicode keeps zero-value Icons (common in embedder tests) drawable.
func (i Icons) orUnicode() Icons {
	if i.Rule == "" {
		return unicodeIcons()
	}
	return i
}
