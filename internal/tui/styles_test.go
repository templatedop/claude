package tui

import (
	"testing"
)

func TestGetAgentStyle(t *testing.T) {
	tests := []struct {
		agentType string
		hasStyle  bool
	}{
		{"orchestrator", true},
		{"planner", true},
		{"researcher", true},
		{"coder", true},
		{"reviewer", true},
		{"executor", true},
		{"unknown", true}, // Returns empty style
	}

	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			style := GetAgentStyle(tt.agentType)
			// Just verify it returns without panicking
			_ = style.Render("test")
		})
	}
}

func TestGetStatusStyle(t *testing.T) {
	tests := []struct {
		status   string
		hasStyle bool
	}{
		{"running", true},
		{"Running", true},
		{"RUNNING", true},
		{"completed", true},
		{"Completed", true},
		{"COMPLETED", true},
		{"failed", true},
		{"Failed", true},
		{"FAILED", true},
		{"pending", true},
		{"unknown", true}, // Returns pending style
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			style := GetStatusStyle(tt.status)
			// Just verify it returns without panicking
			_ = style.Render("test")
		})
	}
}

func TestStylesRender(t *testing.T) {
	// Test that all styles can render without panicking
	// Note: Each style is tested individually since lipgloss.Style.Render
	// uses variadic parameters which doesn't match a simple interface

	t.Run("TitleStyle", func(t *testing.T) {
		if TitleStyle.Render("test") == "" {
			t.Error("TitleStyle rendered empty")
		}
	})

	t.Run("SubtitleStyle", func(t *testing.T) {
		if SubtitleStyle.Render("test") == "" {
			t.Error("SubtitleStyle rendered empty")
		}
	})

	t.Run("BoxStyle", func(t *testing.T) {
		if BoxStyle.Render("test") == "" {
			t.Error("BoxStyle rendered empty")
		}
	})

	t.Run("StatusStyles", func(t *testing.T) {
		if StatusRunningStyle.Render("test") == "" {
			t.Error("StatusRunningStyle rendered empty")
		}
		if StatusCompletedStyle.Render("test") == "" {
			t.Error("StatusCompletedStyle rendered empty")
		}
		if StatusFailedStyle.Render("test") == "" {
			t.Error("StatusFailedStyle rendered empty")
		}
		if StatusPendingStyle.Render("test") == "" {
			t.Error("StatusPendingStyle rendered empty")
		}
	})

	t.Run("TaskStyles", func(t *testing.T) {
		if TaskItemStyle.Render("test") == "" {
			t.Error("TaskItemStyle rendered empty")
		}
		if TaskItemSelectedStyle.Render("test") == "" {
			t.Error("TaskItemSelectedStyle rendered empty")
		}
		if TaskCheckboxStyle.Render("test") == "" {
			t.Error("TaskCheckboxStyle rendered empty")
		}
		if TaskCheckboxEmptyStyle.Render("test") == "" {
			t.Error("TaskCheckboxEmptyStyle rendered empty")
		}
	})

	t.Run("ProgressStyles", func(t *testing.T) {
		if ProgressBarStyle.Render("test") == "" {
			t.Error("ProgressBarStyle rendered empty")
		}
		if ProgressTextStyle.Render("test") == "" {
			t.Error("ProgressTextStyle rendered empty")
		}
	})

	t.Run("UIStyles", func(t *testing.T) {
		if HelpStyle.Render("test") == "" {
			t.Error("HelpStyle rendered empty")
		}
		if ErrorStyle.Render("test") == "" {
			t.Error("ErrorStyle rendered empty")
		}
		if SuccessStyle.Render("test") == "" {
			t.Error("SuccessStyle rendered empty")
		}
		if HeaderStyle.Render("test") == "" {
			t.Error("HeaderStyle rendered empty")
		}
	})

	t.Run("TableStyles", func(t *testing.T) {
		if TableHeaderStyle.Render("test") == "" {
			t.Error("TableHeaderStyle rendered empty")
		}
		if TableCellStyle.Render("test") == "" {
			t.Error("TableCellStyle rendered empty")
		}
	})

	t.Run("InputStyles", func(t *testing.T) {
		if SpinnerStyle.Render("test") == "" {
			t.Error("SpinnerStyle rendered empty")
		}
		if InputStyle.Render("test") == "" {
			t.Error("InputStyle rendered empty")
		}
		if InputFocusedStyle.Render("test") == "" {
			t.Error("InputFocusedStyle rendered empty")
		}
	})

	t.Run("ButtonStyles", func(t *testing.T) {
		if ButtonStyle.Render("test") == "" {
			t.Error("ButtonStyle rendered empty")
		}
		if ButtonFocusedStyle.Render("test") == "" {
			t.Error("ButtonFocusedStyle rendered empty")
		}
	})

	t.Run("AgentStyles", func(t *testing.T) {
		if AgentOrchestratorStyle.Render("test") == "" {
			t.Error("AgentOrchestratorStyle rendered empty")
		}
		if AgentPlannerStyle.Render("test") == "" {
			t.Error("AgentPlannerStyle rendered empty")
		}
		if AgentResearcherStyle.Render("test") == "" {
			t.Error("AgentResearcherStyle rendered empty")
		}
		if AgentCoderStyle.Render("test") == "" {
			t.Error("AgentCoderStyle rendered empty")
		}
		if AgentReviewerStyle.Render("test") == "" {
			t.Error("AgentReviewerStyle rendered empty")
		}
		if AgentExecutorStyle.Render("test") == "" {
			t.Error("AgentExecutorStyle rendered empty")
		}
	})
}

func TestIcons(t *testing.T) {
	icons := []struct {
		name  string
		value string
	}{
		{"IconCheck", IconCheck},
		{"IconCross", IconCross},
		{"IconPending", IconPending},
		{"IconRunning", IconRunning},
		{"IconArrow", IconArrow},
		{"IconBullet", IconBullet},
		{"IconSelected", IconSelected},
	}

	for _, tt := range icons {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				t.Errorf("%s is empty", tt.name)
			}
		})
	}
}
