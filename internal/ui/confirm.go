package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ConfirmAction is the only result a destructive confirmation can emit.
type ConfirmAction string

const (
	ConfirmNone   ConfirmAction = "none"
	ConfirmYes    ConfirmAction = "yes"
	ConfirmNo     ConfirmAction = "no"
	ConfirmCancel ConfirmAction = "cancel"
)

// ConfirmModel owns an explicit y/n modal.
type ConfirmModel struct {
	Open   bool
	Title  string
	Prompt string
	Width  int
	Height int
	Styles Styles
}

func NewConfirm(styles Styles) ConfirmModel { return ConfirmModel{Styles: styles} }

func (c *ConfirmModel) OpenFor(title, prompt string) {
	c.Open = true
	c.Title = title
	c.Prompt = prompt
}

func (c *ConfirmModel) Close() { c.Open = false }

func (c *ConfirmModel) SetSize(width, height int) { c.Width, c.Height = width, height }

func (c *ConfirmModel) Update(msg tea.Msg) ConfirmAction {
	if !c.Open {
		return ConfirmNone
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return ConfirmNone
	}
	switch strings.ToLower(keyMsg.String()) {
	case "y":
		c.Close()
		return ConfirmYes
	case "n":
		c.Close()
		return ConfirmNo
	case "esc", "escape":
		c.Close()
		return ConfirmCancel
	default:
		return ConfirmNone
	}
}

func (c ConfirmModel) View() string {
	if !c.Open || c.Width <= 0 || c.Height <= 0 {
		return ""
	}
	lines := []string{c.Styles.Title.Render(c.Title), Truncate(c.Prompt, max(1, c.Width-4)), "[y] Yes   [n] No"}
	if len(lines) > c.Height-2 && c.Height > 2 {
		lines = lines[:c.Height-2]
	}
	return c.Styles.Border.Width(max(1, c.Width-2)).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}
