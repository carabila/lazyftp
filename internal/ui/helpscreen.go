package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// helpScreenLayout renders the full key reference, grouped by context, from
// the exact same bindings keys.go declares for the footer -- so the two
// cannot list a key differently. Content is wrapped to the box's interior
// width before it is handed to the viewport.
func helpScreenLayout(maxWidth, maxHeight int) (width, height int, content string) {
	hm := help.New()
	hm.Styles.FullKey = lipgloss.NewStyle().Bold(true).Foreground(colorEmphasis)
	hm.Styles.FullDesc = lipgloss.NewStyle().Foreground(colorMuted)
	hm.Styles.FullSeparator = lipgloss.NewStyle()

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	var sections []string
	for i, group := range helpGroups() {
		title := titleStyle.Render(helpGroupTitles[i])
		sections = append(sections, title+"\n"+hm.FullHelpView([][]key.Binding{group}))
	}
	body := strings.Join(sections, "\n\n")

	if maxWidth < 5 {
		maxWidth = 5
	}
	if maxHeight < 3 {
		maxHeight = 3
	}

	width = min(borderOuterWidth(lipgloss.Width(body)), maxWidth)
	contentWidth := borderInteriorWidth(width)
	if contentWidth < 1 {
		contentWidth = 1
	}
	content = lipgloss.NewStyle().Width(contentWidth).Render(body)
	height = min(lipgloss.Height(content)+2, maxHeight)
	return width, height, content
}

func helpScreenView(maxWidth, maxHeight int, content viewport.Model) string {
	width, height, _ := helpScreenLayout(maxWidth, maxHeight)
	return borderWithTitle(content.View(), "Help", width, height, colorAccent)
}
