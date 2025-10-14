package main

import "github.com/charmbracelet/lipgloss"

// ============================================================================
// STYLING SYSTEM
// ============================================================================
// Lip Gloss is a CSS-inspired styling system for terminal applications
// It uses method chaining to build up styles, similar to CSS classes

var (
	// ============================================================================
	// DOCUMENT LAYOUT STYLES
	// ============================================================================

	// docStyle provides overall page margins
	// This creates breathing space around the content
	docStyle = lipgloss.NewStyle().Margin(1, 2) // 1 line top/bottom, 2 spaces left/right

	// ============================================================================
	// TYPOGRAPHY STYLES
	// ============================================================================

	// titleStyle styles the page title at the top
	titleStyle = lipgloss.NewStyle().
			Bold(true).                       // Make it stand out
			Foreground(lipgloss.Color("63")). // Purple color
			MarginBottom(1)                   // Space below title

	// headingStyle styles all heading levels (h1-h6)
	// Headings create visual hierarchy in the document
	headingStyle = lipgloss.NewStyle().
			Bold(true).                        // Bold for importance
			Foreground(lipgloss.Color("203")). // Red-orange color
			MarginTop(1).                      // Space above headings
			MarginBottom(1)                    // Space below headings

	// paragraphStyle styles regular paragraph text
	// This is the main body text style
	paragraphStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")). // Light gray - easy on the eyes
			Width(80).                         // Optimal line length for readability
			MaxWidth(80)                       // Prevent lines from getting too long

	// blockquoteStyle styles quoted text with visual distinction
	// Blockquotes are used for quotations, highlights, or side content
	blockquoteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246")).       // Medium gray - slightly dimmed
			BorderLeft(true).                        // Left border like traditional quotes
			BorderStyle(lipgloss.NormalBorder()).    // Solid line border
			BorderForeground(lipgloss.Color("240")). // Dark gray border
			PaddingLeft(1).                          // Space between border and text
			Width(78).                               // Slightly narrower than paragraphs
			MaxWidth(78)                             // Maintain narrow width
)

// ============================================================================
// COLOR SYSTEM EXPLANATION
// ============================================================================
// Lip Gloss uses 256-color terminal colors. Common colors:
//
// "63"  - Purple      (titles)
// "203" - Red-Orange  (headings)
// "252" - Light Gray  (paragraphs)
// "246" - Medium Gray (blockquotes)
// "241" - Dark Gray   (status bar text)
// "236" - Very Dark   (status bar background)
// "240" - Border Gray (blockquote borders)
// "205" - Pink        (command prompt)
// "255" - White       (command input text)
//
// These colors work across most terminal themes and provide good contrast
// while being easy on the eyes for long reading sessions.
