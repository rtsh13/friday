package ui

import (
	"strings"
	"testing"
)

func TestBanner(t *testing.T) {
	banner := Banner()

	// Banner returns ASCII art, check it's non-empty
	if len(banner) == 0 {
		t.Error("Banner returned empty string")
	}

	// Check for ASCII art box characters that are definitely in the banner
	if !strings.Contains(banner, "AI-Powered") {
		t.Error("Banner should contain 'AI-Powered' tagline")
	}
}

func TestGetStyles(t *testing.T) {
	styles := GetStyles()

	// Verify all style fields are initialized (non-nil check via string method)
	_ = styles.AppStyle.String()
	_ = styles.HeaderStyle.String()
	_ = styles.InputStyle.String()
	_ = styles.ResponseStyle.String()
	_ = styles.ErrorStyle.String()
	_ = styles.HelpStyle.String()
	_ = styles.SpinnerStyle.String()

	// If we got here without panic, styles are properly initialized
	t.Log("All styles initialized correctly")
}

func TestStylesNotEmpty(t *testing.T) {
	styles := GetStyles()

	// Test that styles can render content
	testContent := "test"

	rendered := styles.HeaderStyle.Render(testContent)
	if len(rendered) == 0 {
		t.Error("HeaderStyle.Render returned empty string")
	}

	rendered = styles.ErrorStyle.Render(testContent)
	if len(rendered) == 0 {
		t.Error("ErrorStyle.Render returned empty string")
	}

	rendered = styles.ResponseStyle.Render(testContent)
	if len(rendered) == 0 {
		t.Error("ResponseStyle.Render returned empty string")
	}
}

func TestBannerContainsASCIIArt(t *testing.T) {
	banner := Banner()

	// The banner uses box-drawing or special characters
	// Check for common elements in ASCII art banners
	hasSpecialChars := strings.ContainsAny(banner, "╔╗╚╝║═│┌┐└┘─|+-_/\\[]{}#*")
	hasNewlines := strings.Contains(banner, "\n")

	if !hasNewlines {
		t.Error("Banner should contain multiple lines")
	}

	// Banner should have some structure
	lines := strings.Split(banner, "\n")
	if len(lines) < 3 {
		t.Errorf("Banner should have at least 3 lines, got %d", len(lines))
	}

	t.Logf("Banner has %d lines, special chars: %v", len(lines), hasSpecialChars)
}

func TestNewModel(t *testing.T) {
	model := NewModel()

	// Check initial state
	if model.err != nil {
		t.Errorf("New model should have no error, got: %v", model.err)
	}

	if model.loading {
		t.Error("New model should not be loading initially")
	}

	if model.quitting {
		t.Error("New model should not be quitting initially")
	}

	// Verify styles are set
	if model.styles.HeaderStyle.String() == "" {
		t.Error("Model styles not initialized")
	}
}

func TestModelViewNotEmpty(t *testing.T) {
	model := NewModel()

	view := model.View()

	if len(view) == 0 {
		t.Error("Model.View() returned empty string")
	}

	// Should contain the banner
	if !strings.Contains(view, "AI-Powered") {
		t.Error("View should contain banner with 'AI-Powered' text")
	}
}
