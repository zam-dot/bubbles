package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ============================================================================
// MESSAGE TYPES FOR ASYNC OPERATIONS
// ============================================================================

// contentLoadedMsg is sent when a web page has been successfully loaded and parsed
// This allows the UI to update without blocking during HTTP requests
type contentLoadedMsg struct {
	content string // The formatted content to display
	links   []Link // All links found on the page
	url     string // The URL that was loaded
}

// errorMsg is sent when a web page fails to load
type errorMsg struct {
	err error  // The error that occurred
	url string // The URL that failed
}

// ============================================================================
// LIST ITEM IMPLEMENTATION FOR LINKS
// ============================================================================

// linkItem wraps a Link to make it compatible with Bubble Tea's list component
// This is the adapter pattern - we adapt our Link type to work with the list
type linkItem struct {
	link Link
}

// FilterValue is used by the list component for searching/filtering
// When the user types in the list, this value is searched
func (i linkItem) FilterValue() string {
	// Search both the link text and URL
	return i.link.Text + " " + i.link.URL
}

// Title is displayed as the main text in the list
func (i linkItem) Title() string {
	// Use link text, fall back to URL if no text
	text := i.link.Text
	if text == "" {
		text = i.link.URL
	}
	// Truncate very long text to keep the list readable
	if len(text) > 50 {
		text = text[:47] + "..."
	}
	return text
}

// Description is displayed as secondary text in the list (usually grayed out)
func (i linkItem) Description() string {
	// Show the full URL, truncated if too long
	url := i.link.URL
	if len(url) > 60 {
		url = url[:57] + "..."
	}
	return url
}

// ============================================================================
// MAIN APPLICATION MODEL
// ============================================================================

// model holds all the state for our TUI application
// This is the central data structure that Bubble Tea manages
type model struct {
	viewport   viewport.Model  // Handles scrolling through content
	textInput  textinput.Model // Handles URL input in command mode
	linksList  list.Model      // Handles displaying and selecting links
	content    string          // The current page content (styled)
	links      []Link          // All links on the current page
	ready      bool            // Whether the UI has been initialized
	mode       string          // Current mode: "view", "command", or "links"
	currentURL string          // The URL of the current page
}

// initialModel creates a new model with the starting state
func initialModel(initialContent string, initialURL string, initialLinks []Link) model {
	// ============================================================================
	// TEXT INPUT CONFIGURATION (for command mode)
	// ============================================================================
	ti := textinput.New()
	ti.Placeholder = "Enter URL..."                                        // Hint text when empty
	ti.CharLimit = 500                                                     // Maximum URL length
	ti.Width = 50                                                          // Visual width of input field
	ti.Prompt = "> "                                                       // Characters shown before input
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")) // Pink prompt
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))   // White text
	ti.Blur()                                                              // Start without focus (view mode is default)

	// ============================================================================
	// LINKS LIST CONFIGURATION
	// ============================================================================

	// Convert our []Link slice to []list.Item for the list component
	items := make([]list.Item, len(initialLinks))
	for i, link := range initialLinks {
		items[i] = linkItem{link: link}
	}

	// Create and configure the list component
	// list.NewDefaultDelegate() provides nice default styling for list items
	linksList := list.New(items, list.NewDefaultDelegate(), 0, 0)
	linksList.Title = "Links (Press ESC to go back, ENTER to follow)"
	linksList.SetShowStatusBar(false)   // Hide the "X of Y items" bar
	linksList.SetFilteringEnabled(true) // Allow searching through links by typing

	// ============================================================================
	// INITIAL MODEL STATE
	// ============================================================================
	return model{
		content:    initialContent, // The formatted page content
		links:      initialLinks,   // Links extracted from the page
		textInput:  ti,             // Configured text input
		linksList:  linksList,      // Configured links list
		mode:       "view",         // Start in view mode (reading content)
		currentURL: initialURL,     // Track the current URL
	}
}

// ============================================================================
// BUBBLE TEA LIFECYCLE METHODS
// ============================================================================

// Init is called when the application starts
// It can return initial commands to run (like loading data)
// We don't need any initial commands, so return nil
func (m model) Init() tea.Cmd {
	return nil
}

// Update is called whenever there's a message (user input, timer, custom message)
// This is where we handle all state changes
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd // We can batch multiple commands

	// Handle different types of messages
	switch msg := msg.(type) {

	// ============================================================================
	// CUSTOM MESSAGES FOR ASYNC OPERATIONS
	// ============================================================================
	case contentLoadedMsg:
		// A web page has been successfully loaded in the background
		m.content = msg.content // Update the displayed content
		m.links = msg.links     // Update the available links
		m.currentURL = msg.url  // Update the current URL

		// Refresh the links list with the new links
		items := make([]list.Item, len(msg.links))
		for i, link := range msg.links {
			items[i] = linkItem{link: link}
		}
		m.linksList.SetItems(items)

		// Update the viewport with new content and scroll to top
		m.viewport.SetContent(m.content)
		m.viewport.GotoTop()
		return m, nil

	case errorMsg:
		// A web page failed to load
		m.content = fmt.Sprintf("Error loading %s: %v", msg.url, msg.err)
		m.viewport.SetContent(m.content)
		return m, nil

	// ============================================================================
	// WINDOW RESIZE EVENTS
	// ============================================================================
	case tea.WindowSizeMsg:
		if !m.ready {
			// First time window size is known - initialize viewport
			// Reserve 1 line at bottom for status bar
			m.viewport = viewport.New(msg.Width, msg.Height-1)
			m.viewport.SetContent(m.content)
			m.linksList.SetSize(msg.Width, msg.Height-1)
			m.ready = true
		} else {
			// Window was resized - update component dimensions
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 1
			m.linksList.SetSize(msg.Width, msg.Height-1)
		}

	// ============================================================================
	// KEYBOARD INPUT HANDLING
	// ============================================================================
	case tea.KeyMsg:
		switch msg.String() {

		// GLOBAL KEYBINDS (work in any mode)
		case "q", "ctrl+c":
			// Quit the application
			return m, tea.Quit

		// VIEW MODE KEYBINDS (reading content)
		case ":":
			if m.mode == "view" {
				// Switch to command mode to enter a URL
				m.mode = "command"
				m.textInput.Focus()       // Give focus to text input
				m.textInput.SetValue("")  // Clear any previous input
				return m, textinput.Blink // Start cursor blinking
			}

		case "l", "L":
			if m.mode == "view" && len(m.links) > 0 {
				// Switch to links mode to browse page links
				m.mode = "links"
				return m, nil
			}

		// LINKS MODE KEYBINDS (browsing links list)
		case "enter":
			if m.mode == "links" {
				// Follow the currently selected link
				if selected := m.linksList.SelectedItem(); selected != nil {
					link := selected.(linkItem).link

					// Switch to view mode immediately with loading message
					// This provides instant feedback while content loads
					m.mode = "view"
					m.content = "Loading " + link.URL + "..."
					m.viewport.SetContent(m.content)

					// Start loading content in the background
					// This prevents the UI from freezing during HTTP requests
					return m, tea.Batch(
						func() tea.Msg {
							content, newLinks, err := fetchAndExtractContent(link.URL)
							if err != nil {
								return errorMsg{err: err, url: link.URL}
							}
							return contentLoadedMsg{
								content: content,
								links:   newLinks,
								url:     link.URL,
							}
						},
					)
				}
				return m, nil
			} else if m.mode == "command" {
				// Handle URL entry in command mode
				url := strings.TrimSpace(m.textInput.Value())
				if url != "" {
					// Switch to view mode immediately with loading message
					m.mode = "view"
					m.textInput.Blur() // Remove focus from text input
					m.content = "Loading " + url + "..."
					m.viewport.SetContent(m.content)

					// Start loading content in the background
					return m, tea.Batch(
						func() tea.Msg {
							normalizedURL := normalizeURL(url, m.currentURL)
							if normalizedURL == "" {
								return errorMsg{err: fmt.Errorf("invalid URL: %s", url), url: url}
							}
							content, newLinks, err := fetchAndExtractContent(normalizedURL)
							if err != nil {
								return errorMsg{err: err, url: normalizedURL}
							}
							return contentLoadedMsg{
								content: content,
								links:   newLinks,
								url:     normalizedURL,
							}
						},
					)
				}
			}

		// MODE NAVIGATION KEYBINDS
		case "esc":
			// Escape key cancels current mode and returns to view mode
			if m.mode == "command" || m.mode == "links" {
				m.mode = "view"
				m.textInput.Blur() // Important: remove focus from text input
			}
		}
	}

	// ============================================================================
	// COMPONENT-SPECIFIC UPDATES
	// ============================================================================

	// Update the appropriate component based on current mode
	// Only one component should be active at a time
	switch m.mode {
	case "command":
		// Command mode: text input is active
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	case "links":
		// Links mode: list is active
		m.linksList, cmd = m.linksList.Update(msg)
		cmds = append(cmds, cmd)
	default: // view mode
		// View mode: viewport is active (scrolling content)
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Batch all commands together for efficiency
	return m, tea.Batch(cmds...)
}

// ============================================================================
// RENDERING THE USER INTERFACE
// ============================================================================

// View builds the complete UI representation based on current state
func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var mainContent string
	var statusLine string

	// ============================================================================
	// MODE-SPECIFIC UI RENDERING
	// ============================================================================
	switch m.mode {
	case "command":
		// Command mode: show content with URL input at bottom
		mainContent = m.viewport.View()
		statusLine = fmt.Sprintf("Go to URL: %s", m.textInput.View())
	case "links":
		// Links mode: show the links list full screen
		mainContent = m.linksList.View()
		statusLine = "Use ↑↓ to navigate, ENTER to follow link, ESC to go back"
	default: // view mode
		// View mode: show the page content with navigation hints
		mainContent = m.viewport.View()
		statusLine = fmt.Sprintf(
			"Current: %s | ':' for command, 'L' for links (%d available), 'q' to quit",
			m.currentURL,
			len(m.links),
		)
	}

	// ============================================================================
	// STATUS BAR STYLING
	// ============================================================================
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")). // Dark gray text
		Background(lipgloss.Color("236")). // Dark background
		Padding(0, 1).                     // Horizontal padding
		Width(m.viewport.Width)            // Full width

	// ============================================================================
	// FINAL UI COMPOSITION
	// ============================================================================
	// Stack main content above status bar
	return lipgloss.JoinVertical(
		lipgloss.Left,
		mainContent,
		statusStyle.Render(statusLine),
	)
}
