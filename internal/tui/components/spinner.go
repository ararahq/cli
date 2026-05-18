package components

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const spinnerBrandColor = "#FF6B35"

type Spinner struct {
	Model   spinner.Model
	Message string
}

func NewSpinner() Spinner {
	spinnerModel := spinner.New()
	spinnerModel.Spinner = spinner.Dot
	spinnerModel.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(spinnerBrandColor))

	return Spinner{
		Model:   spinnerModel,
		Message: "Loading...",
	}
}

func (spinnerComponent Spinner) Init() tea.Cmd {
	return spinnerComponent.Model.Tick
}

func (spinnerComponent Spinner) Update(message tea.Msg) (Spinner, tea.Cmd) {
	updatedModel, command := spinnerComponent.Model.Update(message)
	spinnerComponent.Model = updatedModel

	return spinnerComponent, command
}

func (spinnerComponent Spinner) View() string {
	return spinnerComponent.Model.View() + " " + spinnerComponent.Message
}
