// Package tui provides terminal user interface components using Bubble Tea.
package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	primaryColor   = lipgloss.Color("#7C3AED") // Purple
	secondaryColor = lipgloss.Color("#10B981") // Green
	accentColor    = lipgloss.Color("#F59E0B") // Amber
	errorColor     = lipgloss.Color("#EF4444") // Red
	mutedColor     = lipgloss.Color("#6B7280") // Gray
	textColor      = lipgloss.Color("#F9FAFB") // Light
	bgColor        = lipgloss.Color("#1F2937") // Dark
)

// Styles for different UI components
var (
	// Title styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginBottom(1)

	// Box styles
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	// Status styles
	StatusRunningStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true)

	StatusCompletedStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Bold(true)

	StatusFailedStyle = lipgloss.NewStyle().
				Foreground(errorColor).
				Bold(true)

	StatusPendingStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Task list styles
	TaskItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	TaskItemSelectedStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(primaryColor).
				Bold(true)

	TaskCheckboxStyle = lipgloss.NewStyle().
				Foreground(secondaryColor)

	TaskCheckboxEmptyStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Progress styles
	ProgressBarStyle = lipgloss.NewStyle().
				Foreground(primaryColor)

	ProgressTextStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Help styles
	HelpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// Error styles
	ErrorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Success styles
	SuccessStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	// Header styles
	HeaderStyle = lipgloss.NewStyle().
			Background(primaryColor).
			Foreground(textColor).
			Padding(0, 1).
			Bold(true)

	// Table styles
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor).
				BorderStyle(lipgloss.NormalBorder()).
				BorderBottom(true).
				BorderForeground(mutedColor)

	TableCellStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Agent type styles
	AgentOrchestratorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8B5CF6"))
	AgentPlannerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#3B82F6"))
	AgentResearcherStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#06B6D4"))
	AgentCoderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981"))
	AgentReviewerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B"))
	AgentExecutorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EF4444"))

	// Spinner styles
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	// Input styles
	InputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(0, 1)

	InputFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(secondaryColor).
				Padding(0, 1)

	// Button styles
	ButtonStyle = lipgloss.NewStyle().
			Background(primaryColor).
			Foreground(textColor).
			Padding(0, 2).
			MarginRight(1)

	ButtonFocusedStyle = lipgloss.NewStyle().
				Background(secondaryColor).
				Foreground(textColor).
				Padding(0, 2).
				MarginRight(1)
)

// GetAgentStyle returns the appropriate style for an agent type.
func GetAgentStyle(agentType string) lipgloss.Style {
	switch agentType {
	case "orchestrator":
		return AgentOrchestratorStyle
	case "planner":
		return AgentPlannerStyle
	case "researcher":
		return AgentResearcherStyle
	case "coder":
		return AgentCoderStyle
	case "reviewer":
		return AgentReviewerStyle
	case "executor":
		return AgentExecutorStyle
	default:
		return lipgloss.NewStyle()
	}
}

// GetStatusStyle returns the appropriate style for a status.
func GetStatusStyle(status string) lipgloss.Style {
	switch status {
	case "running", "Running", "RUNNING":
		return StatusRunningStyle
	case "completed", "Completed", "COMPLETED":
		return StatusCompletedStyle
	case "failed", "Failed", "FAILED":
		return StatusFailedStyle
	default:
		return StatusPendingStyle
	}
}

// Icons for different states
const (
	IconCheck    = "✓"
	IconCross    = "✗"
	IconPending  = "○"
	IconRunning  = "◐"
	IconArrow    = "→"
	IconBullet   = "•"
	IconSelected = "▶"
)
