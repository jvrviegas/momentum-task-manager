// Command momentum is a keyboard-first terminal frontend for Taskwarrior.
package main

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type model struct {
	input textinput.Model
}

func newModel() model {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Momentum"
	input.Focus()
	return model{input: input}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() tea.View {
	return tea.NewView(lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render(m.input.View()))
}

func main() {
	program := tea.NewProgram(newModel())
	_, _ = program.Run()
}
