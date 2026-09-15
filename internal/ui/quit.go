package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// QuitChoice routes the explicit unsynced-change decision.
type QuitChoice string

const (
	QuitNone   QuitChoice = "none"
	QuitSync   QuitChoice = "sync"
	QuitLocal  QuitChoice = "local"
	QuitCancel QuitChoice = "cancel"
)

// QuitModel displays the only choices that can follow an unsynced quit.
type QuitModel struct {
	Open   bool
	Width  int
	Height int
	Styles Styles
}

func NewQuit(styles Styles) QuitModel          { return QuitModel{Styles: styles} }
func (q *QuitModel) SetSize(width, height int) { q.Width, q.Height = width, height }
func (q *QuitModel) OpenQuit()                 { q.Open = true }
func (q *QuitModel) Close()                    { q.Open = false }

func (q *QuitModel) Update(msg tea.Msg) QuitChoice {
	if !q.Open {
		return QuitNone
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return QuitNone
	}
	switch strings.ToLower(keyMsg.String()) {
	case "s":
		q.Close()
		return QuitSync
	case "q":
		q.Close()
		return QuitLocal
	case "esc", "escape":
		q.Close()
		return QuitCancel
	default:
		return QuitNone
	}
}

func (q QuitModel) View() string {
	if !q.Open || q.Width <= 0 || q.Height <= 0 {
		return ""
	}
	lines := []string{
		q.Styles.Title.Render("Unsynced changes"),
		"[s] Sync and quit",
		"[q] Quit without syncing",
		"[Esc] Cancel",
	}
	if len(lines) > q.Height-2 && q.Height > 2 {
		lines = lines[:q.Height-2]
	}
	return q.Styles.Border.Width(max(1, q.Width-2)).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// MinimumSizeMessage is intentionally plain and width-safe.
func MinimumSizeMessage(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	lines := []string{
		"Terminal too small",
		"Resize to at least 28×8 to use Momentum.",
	}
	for index := range lines {
		lines[index] = Truncate(lines[index], width)
	}
	return strings.Join(lines, "\n")
}
