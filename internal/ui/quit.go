package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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
	contentWidth := ModalContentWidth(q.Width, ModalMaxWidth)
	contentHeight := ModalContentHeight(q.Height, 0)
	lines := []string{
		q.Styles.ModalTitle.Render("Unsynced changes"),
		q.Styles.ModalBody.Render("Your local changes have not been synchronized."),
		q.Styles.ModalAction.Render("[s] Sync and quit"),
		q.Styles.ModalAction.Render("[q] Quit without syncing"),
		q.Styles.ModalAction.Render("[Esc] Cancel"),
	}
	return renderBoundedPanel(lines, contentWidth, contentHeight, q.Styles)
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
