package tui

import "github.com/charmbracelet/lipgloss"

// designSystem centralizes the TUI's visual language so every screen uses the
// same hierarchy and remains readable when Lip Gloss disables color output.
type designSystem struct {
	app          lipgloss.Style
	panel        lipgloss.Style
	compactPanel lipgloss.Style
	chrome       lipgloss.Style
	header       lipgloss.Style
	activeTab    lipgloss.Style
	inactiveTab  lipgloss.Style
	selected     lipgloss.Style
	unread       lipgloss.Style
	metadata     lipgloss.Style
	status       lipgloss.Style
	help         lipgloss.Style
	loading      lipgloss.Style
	error        lipgloss.Style
	errorPanel   lipgloss.Style
	compactError lipgloss.Style
	empty        lipgloss.Style
}

var styles = newDesignSystem()

func newDesignSystem() designSystem {
	accent := lipgloss.AdaptiveColor{Light: "#0550AE", Dark: "#79C0FF"}
	muted := lipgloss.AdaptiveColor{Light: "#57606A", Dark: "#8B949E"}
	border := lipgloss.AdaptiveColor{Light: "#D0D7DE", Dark: "#30363D"}
	warning := lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}
	danger := lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#FF7B72"}

	return designSystem{
		app:          lipgloss.NewStyle().Padding(0, 1),
		panel:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1),
		compactPanel: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border),
		chrome:       lipgloss.NewStyle().Bold(true).Foreground(accent),
		header:       lipgloss.NewStyle().Bold(true).Foreground(accent),
		activeTab:    lipgloss.NewStyle().Bold(true).Foreground(accent).Underline(true),
		inactiveTab:  lipgloss.NewStyle().Foreground(muted),
		selected:     lipgloss.NewStyle().Bold(true).Foreground(accent),
		unread:       lipgloss.NewStyle().Bold(true).Foreground(warning),
		metadata:     lipgloss.NewStyle().Foreground(muted).Faint(true),
		status:       lipgloss.NewStyle().Bold(true).Foreground(accent),
		help:         lipgloss.NewStyle().Foreground(muted),
		loading:      lipgloss.NewStyle().Bold(true).Foreground(accent),
		error:        lipgloss.NewStyle().Bold(true).Foreground(danger),
		errorPanel:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(danger).Padding(0, 1),
		compactError: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(danger),
		empty:        lipgloss.NewStyle().Foreground(muted).Italic(true),
	}
}
