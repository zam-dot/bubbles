package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Enhanced View with status panel
func (m *model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderTabBar(),
		m.statusView(),
		m.urlInput.View(),
		"",
		m.viewport.View(),
		m.renderStatusPanel(),
	)
}

func (m *model) renderImages() string {
	if len(m.images) == 0 {
		return "# Images\n\nNo images found on this page."
	}

	var content strings.Builder
	content.WriteString("# Images on This Page\n\n")
	content.WriteString("Type a number to view image details.\n\n")

	for i, img := range m.images {
		altText := img.AltText
		if altText == "" {
			altText = "No description"
		}
		content.WriteString(fmt.Sprintf("## %d. %s\n", i+1, altText))
		content.WriteString(fmt.Sprintf("Type: %s\n", img.Type))
		content.WriteString(fmt.Sprintf("URL: %s\n\n", img.URL))
	}

	content.WriteString(fmt.Sprintf("Total: %d images | Type number for details", len(m.images)))

	styled, err := renderWithStyle(content.String())
	if err != nil {
		return content.String()
	}
	return styled
}

func (m *model) renderHistory() string {
	activeTab := m.activeTabPtr()
	if activeTab == nil || len(activeTab.History) == 0 {
		return "# Browser History\n\nNo history yet. Start browsing to build history!"
	}
	var historyContent strings.Builder
	historyContent.WriteString("# Browser History\n\n")
	historyContent.WriteString(
		"Use ←/→ or B/F to navigate, or type a number to jump to that page.\n\n",
	)
	for i, url := range activeTab.History {
		indicator := "  "
		if i == activeTab.CurrentPos {
			indicator = "➤ "
		}
		displayURL := url
		if len(displayURL) > 60 {
			displayURL = displayURL[:57] + "..."
		}
		historyContent.WriteString(fmt.Sprintf("%s[%d] %s\n", indicator, i+1, displayURL))
	}
	historyContent.WriteString(
		fmt.Sprintf(
			"\nTotal: %d pages | Current position: %d",
			len(activeTab.History),
			activeTab.CurrentPos+1,
		),
	)
	styledHistory, err := renderWithStyle(historyContent.String())
	if err != nil {
		return historyContent.String()
	}
	return styledHistory
}

func (m *model) renderBookmarks() string {
	if len(m.bookmarks) == 0 {
		return "# Bookmarks\n\nNo bookmarks yet! Use Ctrl+D to bookmark the current page.\n\n⭐ **Tip**: Visit your favorite sites and press Ctrl+D to save them!"
	}
	var bookmarksContent strings.Builder
	bookmarksContent.WriteString("# Bookmarks\n\n")
	bookmarksContent.WriteString(
		"Type a number to open that bookmark, or Ctrl+D to bookmark current page.\n\n",
	)
	for i, bookmark := range m.bookmarks {
		displayURL := bookmark.URL
		if len(displayURL) > 50 {
			displayURL = displayURL[:47] + "..."
		}
		bookmarksContent.WriteString(fmt.Sprintf("[%d] **%s**\n", i+1, bookmark.Title))
		bookmarksContent.WriteString(fmt.Sprintf("    %s\n\n", displayURL))
	}
	bookmarksContent.WriteString(
		fmt.Sprintf(
			"Total: %d bookmarks | Ctrl+D: Bookmark current | B: Show bookmarks",
			len(m.bookmarks),
		),
	)
	styledBookmarks, err := renderWithStyle(bookmarksContent.String())
	if err != nil {
		return bookmarksContent.String()
	}
	return styledBookmarks
}

func (m *model) renderSearchResults(query string, results []SearchResult) string {
	if len(results) == 0 {
		return fmt.Sprintf(
			"# Search Results\n\nNo results found for: **%s**\n\nTry a different search query.",
			query,
		)
	}
	var searchContent strings.Builder
	searchContent.WriteString("# Search Results\n\n")
	searchContent.WriteString(fmt.Sprintf("Query: **%s**\n\n", query))
	searchContent.WriteString("Type a number to open that result.\n\n")
	for _, result := range results {
		searchContent.WriteString(fmt.Sprintf("[%d] **%s**\n", result.Number, result.Title))
		searchContent.WriteString(fmt.Sprintf("    %s\n", result.URL))
		if result.Snippet != "" {
			searchContent.WriteString(fmt.Sprintf("    *%s*\n", result.Snippet))
		}
		searchContent.WriteString("\n")
	}
	searchContent.WriteString(
		fmt.Sprintf("Found %d results | Type number to open | Ctrl+S: New search", len(results)),
	)
	styledSearch, err := renderWithStyle(searchContent.String())
	if err != nil {
		return searchContent.String()
	}
	return styledSearch
}
