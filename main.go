package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type Config struct {
	EnableReaderMode   bool `json:"enable_reader_mode"`
	EnableBookmarks    bool `json:"enable_bookmarks"`
	EnableHistory      bool `json:"enable_history"`
	EnableSearch       bool `json:"enable_search"`
	EnableTabs         bool `json:"enable_tabs"`
	EnableStatusPanel  bool `json:"enable_status_panel"`
	MaxTabs            int  `json:"max_tabs"`
	PageCacheSize      int  `json:"page_cache_size"`
	EnableMouseSupport bool `json:"enable_mouse_support"`
	StatusPanelTimeout int  `json:"status_panel_timeout"` // seconds
}

func DefaultConfig() Config {
	return Config{
		EnableReaderMode:   true,
		EnableBookmarks:    true,
		EnableHistory:      true,
		EnableSearch:       true,
		EnableTabs:         true,
		EnableStatusPanel:  true,
		MaxTabs:            10,
		PageCacheSize:      50,
		EnableMouseSupport: true,
		StatusPanelTimeout: 5,
	}
}

// Link represents a clickable link
type Link struct {
	Number  int
	Text    string
	URL     string
	FullURL string
	IsImage bool
}

// Bookmark represents a saved website
type Bookmark struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// SearchResult represents a search result
type SearchResult struct {
	Number  int
	Title   string
	URL     string
	Snippet string
}

// Tab represents a browser tab
type Tab struct {
	ID         int
	Title      string
	URL        string
	Content    string
	Links      []Link
	Images     []ImageInfo
	ReaderMode bool
	History    []string
	CurrentPos int
}

// NEW: Status information for bottom panel
type StatusInfo struct {
	Loading      bool
	LoadingStage string
	StartTime    time.Time
	LoadTime     time.Duration
	PageSize     int
	LinkCount    int
	StatusCode   int
	Error        string
}

// ImageInfo represents an image on the page
type ImageInfo struct {
	Number   int
	URL      string
	AltText  string
	Type     string
	IsLinked bool
	LinkURL  string
}

type model struct {
	viewport      viewport.Model
	urlInput      textinput.Model
	content       string
	ready         bool
	loading       bool
	tabs          []Tab
	activeTab     int
	links         []Link
	images        []ImageInfo
	showImages    bool
	showHistory   bool
	bookmarks     []Bookmark
	showBookmarks bool
	bookmarkFile  string
	searchResults []SearchResult
	showSearch    bool
	searchQuery   string
	readerMode    bool
	status        StatusInfo
	config        Config
	currentImage  *ImageInfo
}

type fetchContentMsg struct {
	content    string
	links      []Link
	images     []ImageInfo
	tabID      int
	loadTime   time.Duration
	pageSize   int
	statusCode int
}

type errorMsg struct {
	err   error
	tabID int
}

type searchResultsMsg struct {
	query   string
	results []SearchResult
}

func LoadConfig(filename string) Config {
	config := DefaultConfig()

	// Try to load from file first
	if data, err := os.ReadFile(filename); err == nil {
		json.Unmarshal(data, &config)
	}

	// Environment variables override file config
	config = applyEnvOverrides(config)

	return config
}

func InitialModel(config Config) model {
	ti := textinput.New()
	ti.Placeholder = "Enter URL, search, or commands"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 50

	bookmarkFile := "bookmarks.json"
	bookmarks := loadBookmarks(bookmarkFile)

	// Load help content
	helpContent := loadHelpContent()

	initialTab := Tab{
		ID:         0,
		Title:      "Help",
		URL:        "help://welcome",
		Content:    helpContent,
		Links:      []Link{},
		Images:     []ImageInfo{},
		ReaderMode: false,
		History:    []string{},
		CurrentPos: -1,
	}

	return model{
		urlInput:      ti,
		content:       helpContent,
		loading:       false,
		tabs:          []Tab{initialTab},
		activeTab:     0,
		links:         []Link{},
		images:        []ImageInfo{},
		showHistory:   false,
		bookmarks:     bookmarks,
		showBookmarks: false,
		bookmarkFile:  bookmarkFile,
		searchResults: []SearchResult{},
		showSearch:    false,
		searchQuery:   "",
		readerMode:    false,
		status: StatusInfo{
			Loading:      false,
			LoadingStage: "Ready",
			LoadTime:     0,
			PageSize:     0,
			LinkCount:    0,
			StatusCode:   0,
		},
		config: config,
	}
}

func loadHelpContent() string {
	// Try to load from help.md file first
	if data, err := os.ReadFile("help.md"); err == nil {
		styled, err := renderWithStyle(string(data))
		if err == nil {
			return styled
		}
		return string(data) // Fallback to raw markdown
	}

	// Fallback embedded help
	fallbackHelp := `# 🌐 Terminal Browser Help
    
## Quick Start
- Type a **URL** to visit a website
- Type any **text** to search the web  
- Use **↑/↓** to scroll, **Enter** to submit
- Press **1,2,3...** to follow links
- Type **img1, img2...** to view images

## Essential Shortcuts
- **Ctrl+T** - New tab
- **Ctrl+W** - Close tab  
- **Ctrl+R** - Reload
- **Ctrl+E** - Reader mode
- **Ctrl+D** - Bookmark page
- **history** - View history
- **bookmarks** - View bookmarks

*Type 'help' anytime to see this page*`

	styled, err := renderWithStyle(fallbackHelp)
	if err != nil {
		return fallbackHelp
	}
	return styled
}

func isImageURL(url string) bool {
	imageExtensions := []string{
		".jpg", ".jpeg", ".png", ".gif", ".webp",
		".bmp", ".svg", ".ico", ".tiff", ".tif",
	}

	url = strings.ToLower(url)
	for _, ext := range imageExtensions {
		if strings.HasSuffix(url, ext) {
			return true
		}
	}

	// Also check Content-Type in URL pattern
	if strings.Contains(url, ".jpg") ||
		strings.Contains(url, ".jpeg") ||
		strings.Contains(url, ".png") ||
		strings.Contains(url, ".gif") ||
		strings.Contains(url, ".webp") {
		return true
	}

	return false
}

// NEW: Update loading status
func (m *model) updateLoading(stage string) {
	m.status.Loading = true
	m.status.LoadingStage = stage
	m.status.StartTime = time.Now()
	m.status.Error = ""
}

// NEW: Complete loading with results
func (m *model) completeLoading(
	loadTime time.Duration,
	pageSize int,
	statusCode int,
	linkCount int,
) {
	m.status.Loading = false
	m.status.LoadTime = loadTime
	m.status.PageSize = pageSize
	m.status.StatusCode = statusCode
	m.status.LinkCount = linkCount
	// You could add m.status.ImageCount = len(m.images) if you want
}

// NEW: Set error status
func (m *model) setError(err string) {
	m.status.Loading = false
	m.status.Error = err
	m.status.LoadTime = 0
}

// Get the active tab
func (m *model) activeTabPtr() *Tab {
	if len(m.tabs) == 0 {
		return nil
	}
	return &m.tabs[m.activeTab]
}

// Create a new tab
func (m *model) newTab(url string) {
	newTabID := len(m.tabs)
	newTab := Tab{
		ID:         newTabID,
		Title:      "New Tab",
		URL:        url,
		Content:    "🔄 Loading...",
		Links:      []Link{},
		ReaderMode: false,
		History:    []string{},
		CurrentPos: -1,
	}

	if url != "" {
		newTab.navigateTo(url)
	}

	m.tabs = append(m.tabs, newTab)
	m.activeTab = newTabID
}

// Close a tab
func (m *model) closeTab(tabID int) {
	if len(m.tabs) <= 1 {
		return
	}

	m.tabs = append(m.tabs[:tabID], m.tabs[tabID+1:]...)
	for i := range m.tabs {
		m.tabs[i].ID = i
	}

	if m.activeTab >= len(m.tabs) {
		m.activeTab = len(m.tabs) - 1
	} else if m.activeTab >= tabID {
		m.activeTab--
	}
}

// Switch to a tab
func (m *model) switchTab(tabID int) {
	if tabID >= 0 && tabID < len(m.tabs) {
		m.activeTab = tabID
		tab := m.tabs[m.activeTab]
		m.content = tab.Content
		m.links = tab.Links
		m.readerMode = tab.ReaderMode

		if m.ready {
			m.viewport.SetContent(m.content)
			m.viewport.GotoTop()
		}
	}
}

// Tab navigation helper
func (t *Tab) navigateTo(url string) {
	if t.CurrentPos < len(t.History)-1 {
		t.History = t.History[:t.CurrentPos+1]
	}
	t.History = append(t.History, url)
	t.CurrentPos = len(t.History) - 1
	t.URL = url
}

// Tab history helpers
func (t *Tab) canGoBack() bool {
	return t.CurrentPos > 0
}

func (t *Tab) canGoForward() bool {
	return t.CurrentPos < len(t.History)-1
}

func (t *Tab) goBack() {
	if t.canGoBack() {
		t.CurrentPos--
		t.URL = t.History[t.CurrentPos]
	}
}

func (t *Tab) goForward() {
	if t.canGoForward() {
		t.CurrentPos++
		t.URL = t.History[t.CurrentPos]
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Let handleKeyMsg process special commands first
		newModel, newCmd := m.handleKeyMsg(msg)
		if newCmd != nil || newModel != m {
			return newModel, newCmd
		}
		// If not handled as special command, pass to viewport for scrolling
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case fetchContentMsg:
		return m.handleFetchContent(msg)
	case searchResultsMsg:
		return m.handleSearchResults(msg)
	case errorMsg:
		return m.handleError(msg)
	}

	m.urlInput, cmd = m.urlInput.Update(msg)
	m.viewport, _ = m.viewport.Update(msg)

	return m, cmd
}

// Handle key messages
func (m *model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "ctrl+t":
		return m.handleNewTab()

	case "ctrl+l", "ctrl+L":
		return m.handleFollowImageLink()

	case "ctrl+w":
		return m.handleCloseTab()

	case "ctrl+tab":
		return m.handleNextTab()

	case "ctrl+shift+tab":
		return m.handlePrevTab()

	case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5":
		if !m.urlInput.Focused() {
			return m.handleTabSwitch(msg.String())
		}

	case "enter":
		if m.urlInput.Focused() {
			return m.handleEnter()
		}

	case "ctrl+r":
		return m.handleReload()

	case "ctrl+e":
		return m.handleReaderToggle()

	case "ctrl+d", "ctrl+b":
		return m.handleBookmark()

	case "ctrl+s":
		return m.handleFocusSearch()

	case "left":
		return m.handleGoBack()

	case "right":
		return m.handleGoForward()

	case "escape":
		return m.handleEscape()

	case "ctrl+o", "ctrl+O":
		return m.handleOpenImage()
	}

	m.urlInput, _ = m.urlInput.Update(msg)
	return m, nil
}

func (m *model) handleFollowImageLink() (tea.Model, tea.Cmd) {
	if m.currentImage != nil && m.currentImage.IsLinked && m.currentImage.LinkURL != "" {
		currentImg := m.currentImage
		m.currentImage = nil

		activeTab := m.activeTabPtr()
		if activeTab != nil {
			m.updateLoading("Following image link...")
			m.content = fmt.Sprintf("🔄 Following link: %s", currentImg.LinkURL)
			activeTab.navigateTo(currentImg.LinkURL)
			m.readerMode = false
			activeTab.ReaderMode = false
			m.urlInput.SetValue("")
			return m, fetchContentWithLinks(currentImg.LinkURL, m.activeTab)
		}
	}
	return m, nil
}

// Command handlers
func (m *model) handleNewTab() (tea.Model, tea.Cmd) {
	if !m.config.EnableTabs {
		m.content = "❌ Tabs are disabled"
		m.setError("Tabs feature disabled")
		if m.ready {
			m.viewport.SetContent(m.content)
		}
		return m, nil
	}
	if len(m.tabs) >= m.config.MaxTabs {
		m.content = fmt.Sprintf("❌ Maximum tabs (%d) reached", m.config.MaxTabs)
		m.setError(fmt.Sprintf("Max tabs limit: %d", m.config.MaxTabs))
		if m.ready {
			m.viewport.SetContent(m.content)
		}
		return m, nil
	}

	m.newTab("")
	m.content = "🌐 New Tab"
	m.links = []Link{}
	m.images = []ImageInfo{}
	m.readerMode = false
	m.showHistory = false
	m.showBookmarks = false
	m.showSearch = false
	m.urlInput.SetValue("")
	m.setError("")
	if m.ready {
		m.viewport.SetContent(m.content)
	}
	return m, nil
}

func (m *model) handleCloseTab() (tea.Model, tea.Cmd) {
	if len(m.tabs) <= 1 {
		return m, nil
	}

	m.closeTab(m.activeTab)
	activeTab := m.activeTabPtr()
	if activeTab != nil {
		m.content = activeTab.Content
		m.links = activeTab.Links
		m.images = activeTab.Images
		m.readerMode = activeTab.ReaderMode
		m.setError("")
		if m.ready {
			m.viewport.SetContent(m.content)
		}
	}
	return m, nil
}

func (m *model) handleNextTab() (tea.Model, tea.Cmd) {
	nextTab := (m.activeTab + 1) % len(m.tabs)
	m.switchTab(nextTab)
	m.setError("")
	return m, nil
}

func (m *model) handlePrevTab() (tea.Model, tea.Cmd) {
	prevTab := (m.activeTab - 1 + len(m.tabs)) % len(m.tabs)
	m.switchTab(prevTab)
	m.setError("")
	return m, nil
}

func (m *model) handleTabSwitch(key string) (tea.Model, tea.Cmd) {
	tabNum, _ := strconv.Atoi(key)
	tabIndex := tabNum - 1
	if tabIndex >= 0 && tabIndex < len(m.tabs) && tabIndex < 9 {
		m.switchTab(tabIndex)
		m.setError("")
	}
	return m, nil
}

func (m *model) handleEnter() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.urlInput.Value())
	activeTab := m.activeTabPtr()
	if activeTab == nil {
		return m, nil
	}

	// Handle special commands
	if cmd, handled := m.handleSpecialCommands(input, activeTab); handled {
		return m, cmd
	}

	// Handle numbers (links, images, etc.)
	if num, err := strconv.Atoi(input); err == nil {
		return m.handleNumberInput(num, activeTab)
	}

	// Handle URLs and search
	return m.handleURLOrSearch(input, activeTab)
}

func (m *model) handleSpecialCommands(input string, activeTab *Tab) (tea.Cmd, bool) {
	switch input {

	case "help", "?":
		m.content = loadHelpContent()
		m.showHistory = false
		m.showBookmarks = false
		m.showSearch = false
		m.showImages = false
		m.readerMode = false
		m.urlInput.SetValue("")
		m.setError("")
		if m.ready {
			m.viewport.SetContent(m.content)
		}
		return nil, true

	case "history", "h":
		if !m.config.EnableHistory {
			m.content = "❌ History is disabled"
			m.setError("History feature disabled")
			if m.ready {
				m.viewport.SetContent(m.content)
			}
			return nil, true
		}
		m.showHistory = true
		m.showBookmarks = false
		m.showSearch = false
		m.showImages = false
		m.readerMode = false
		m.content = m.renderHistory()
		m.urlInput.SetValue("")
		m.setError("")
		if m.ready {
			m.viewport.SetContent(m.content)
		}
		return nil, true

	case "bookmarks", "b", "B":
		if !m.config.EnableBookmarks {
			m.content = "❌ Bookmarks are disabled"
			m.setError("Bookmarks feature disabled")
			if m.ready {
				m.viewport.SetContent(m.content)
			}
			return nil, true
		}
		m.showBookmarks = true
		m.showHistory = false
		m.showSearch = false
		m.showImages = false
		m.readerMode = false
		m.content = m.renderBookmarks()
		m.urlInput.SetValue("")
		m.setError("")
		if m.ready {
			m.viewport.SetContent(m.content)
		}
		return nil, true

	case "images", "i":
		m.showImages = true
		m.showHistory = false
		m.showBookmarks = false
		m.showSearch = false
		m.readerMode = false
		m.content = m.renderImages()
		m.urlInput.SetValue("")
		m.setError("")
		if m.ready {
			m.viewport.SetContent(m.content)
		}
		return nil, true

	case "reader", "r", "R":
		if len(activeTab.History) > 0 && activeTab.CurrentPos >= 0 {
			m.updateLoading("Activating reader mode...")
			m.content = "🔄 Activating reader mode..."
			currentURL := activeTab.History[activeTab.CurrentPos]
			m.readerMode = true
			activeTab.ReaderMode = true
			m.urlInput.SetValue("")
			return fetchContentWithReaderMode(currentURL, m.activeTab), true
		}
	}

	// Handle image commands (img1, img2, etc.) - case insensitive version
	lowerInput := strings.ToLower(input)
	if numStr, found := strings.CutPrefix(lowerInput, "img"); found {
		numStr = strings.TrimSpace(numStr)
		if imgNum, err := strconv.Atoi(numStr); err == nil {
			if imgNum > 0 && imgNum <= len(m.images) {
				image := m.images[imgNum-1]
				m.currentImage = &image

				var options string
				if image.IsLinked && image.LinkURL != "" {
					options = "\nPress 'l' to follow link | 'o' to open image | Enter to go back"
				} else {
					options = "\nPress 'o' to open image | Enter to go back"
				}

				linkedInfo := ""
				if image.IsLinked && image.LinkURL != "" {
					linkedInfo = fmt.Sprintf("\n🔗 Links to: %s", image.LinkURL)
				}

				m.content = fmt.Sprintf(
					"🖼️ Image %d: %s\n\nURL: %s\n\nAlt Text: %s\nType: %s%s\n\n%s",
					imgNum,
					image.AltText,
					image.URL,
					image.AltText,
					image.Type,
					linkedInfo,
					options,
				)
				m.urlInput.SetValue("")
				if m.ready {
					m.viewport.SetContent(m.content)
				}
				return nil, true
			}
		} else {
			m.content = "❌ Invalid image format. Use: img1, img2, etc."
			m.setError("Invalid image format")
			m.urlInput.SetValue("")
			if m.ready {
				m.viewport.SetContent(m.content)
			}
			return nil, true
		}
	}

	return nil, false
}

func (m *model) handleNumberInput(num int, activeTab *Tab) (tea.Model, tea.Cmd) {
	if m.showSearch && num > 0 && num <= len(m.searchResults) {
		result := m.searchResults[num-1]
		m.updateLoading("Opening search result...")
		m.content = fmt.Sprintf("🔄 Opening: %s", result.Title)
		activeTab.navigateTo(result.URL)
		m.showSearch = false
		m.readerMode = false
		m.currentImage = nil
		activeTab.ReaderMode = false
		m.urlInput.SetValue("")
		return m, fetchContentWithLinks(result.URL, m.activeTab)

	} else if m.showBookmarks && num > 0 && num <= len(m.bookmarks) {
		bookmark := m.bookmarks[num-1]
		m.updateLoading("Opening bookmark...")
		m.content = fmt.Sprintf("🔄 Opening bookmark: %s", bookmark.Title)
		activeTab.navigateTo(bookmark.URL)
		m.showBookmarks = false
		m.readerMode = false
		activeTab.ReaderMode = false
		m.urlInput.SetValue("")
		return m, fetchContentWithLinks(bookmark.URL, m.activeTab)

	} else if num > 0 && num <= len(m.links) {
		link := m.links[num-1]

		// Check if this link points to an image
		if isImageURL(link.FullURL) {
			m.content = fmt.Sprintf("🖼️ Image Link: %s\n\nURL: %s\n\nPress 'o' to open image externally",
				link.Text, link.FullURL)
			m.currentImage = &ImageInfo{
				URL:     link.FullURL,
				AltText: link.Text,
				Type:    getImageType(link.FullURL),
			}
			m.urlInput.SetValue("")
			if m.ready {
				m.viewport.SetContent(m.content)
			}
			return m, nil
		}

		m.updateLoading("Following link...")
		m.content = fmt.Sprintf("🔄 Navigating to: %s", link.Text)
		activeTab.navigateTo(link.FullURL)
		m.readerMode = false
		activeTab.ReaderMode = false
		m.urlInput.SetValue("")
		return m, fetchContentWithLinks(link.FullURL, m.activeTab)

	} else {
		m.content = fmt.Sprintf("❌ Invalid number. Available links: 1-%d", len(m.links))
		m.setError("Invalid link number")
		m.urlInput.SetValue("")
		if m.ready {
			m.viewport.SetContent(m.content)
		}
	}

	return m, nil
}

func (m *model) handleURLOrSearch(input string, activeTab *Tab) (tea.Model, tea.Cmd) {
	if input != "" {
		if strings.Contains(input, ".") ||
			strings.HasPrefix(input, "http://") ||
			strings.HasPrefix(input, "https://") {
			url := input
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				url = "https://" + url
			}

			// Check if this is an image URL
			if isImageURL(url) {
				m.content = fmt.Sprintf(
					"🖼️ Image URL detected: %s\n\nPress 'o' to open image externally",
					url,
				)
				m.currentImage = &ImageInfo{
					URL:     url,
					AltText: "Direct image link",
					Type:    getImageType(url),
				}
				m.urlInput.SetValue("")
				if m.ready {
					m.viewport.SetContent(m.content)
				}
				return m, nil
			}

			m.updateLoading("Fetching page...")
			m.content = "🔄 Loading..."
			activeTab.navigateTo(url)
			m.showBookmarks = false
			m.showHistory = false
			m.showSearch = false
			m.showImages = true
			m.readerMode = false
			activeTab.ReaderMode = false
			m.urlInput.SetValue("")
			return m, fetchContentWithLinks(url, m.activeTab)
		} else {
			m.updateLoading("Searching...")
			m.content = fmt.Sprintf("🔍 Searching for: %s", input)
			m.searchQuery = input
			m.showSearch = true
			m.showBookmarks = false
			m.showHistory = false
			m.showImages = false
			m.readerMode = true
			activeTab.ReaderMode = false
			m.urlInput.SetValue("")
			return m, performSearch(input)
		}
	}
	return m, nil
}

func (m *model) handleReload() (tea.Model, tea.Cmd) {
	activeTab := m.activeTabPtr()
	if activeTab != nil && len(activeTab.History) > 0 && activeTab.CurrentPos >= 0 {
		m.updateLoading("Reloading...")
		m.content = "🔄 Reloading..."
		currentURL := activeTab.History[activeTab.CurrentPos]
		if m.readerMode {
			return m, fetchContentWithReaderMode(currentURL, m.activeTab)
		}
		return m, fetchContentWithLinks(currentURL, m.activeTab)
	}
	return m, nil
}

func (m *model) handleReaderToggle() (tea.Model, tea.Cmd) {
	activeTab := m.activeTabPtr()
	if activeTab != nil && len(activeTab.History) > 0 && activeTab.CurrentPos >= 0 {
		if m.readerMode {
			m.readerMode = false
			activeTab.ReaderMode = false
			m.updateLoading("Loading original view...")
			m.content = "🔄 Loading original view..."
			currentURL := activeTab.History[activeTab.CurrentPos]
			m.urlInput.SetValue("")
			return m, fetchContentWithLinks(currentURL, m.activeTab)
		} else {
			m.updateLoading("Activating reader mode...")
			m.content = "🔄 Activating reader mode..."
			currentURL := activeTab.History[activeTab.CurrentPos]
			m.readerMode = true
			activeTab.ReaderMode = true
			m.urlInput.SetValue("")
			return m, fetchContentWithReaderMode(currentURL, m.activeTab)
		}
	}
	return m, nil
}

func (m *model) handleBookmark() (tea.Model, tea.Cmd) {
	if !m.config.EnableBookmarks {
		m.content = "❌ Bookmarks are disabled"
		m.setError("Bookmarks feature disabled")
		if m.ready {
			m.viewport.SetContent(m.content)
		}
		return m, nil
	}

	activeTab := m.activeTabPtr()
	if activeTab != nil && len(activeTab.History) > 0 && activeTab.CurrentPos >= 0 {
		currentURL := activeTab.History[activeTab.CurrentPos]
		if !m.isBookmarked(currentURL) {
			title := "Untitled"
			if len(m.links) > 0 {
				for _, link := range m.links {
					if strings.Contains(strings.ToLower(link.Text), "title") ||
						strings.Contains(strings.ToLower(link.Text), "heading") {
						title = link.Text
						break
					}
				}
			}
			m.addBookmark(title)
			m.content = fmt.Sprintf("⭐ Bookmarked: %s", title)
			m.setError("")
			if m.ready {
				m.viewport.SetContent(m.content)
			}
		} else {
			m.content = "✅ Already bookmarked!"
			m.setError("")
			if m.ready {
				m.viewport.SetContent(m.content)
			}
		}
	}
	return m, nil
}

func (m *model) handleFocusSearch() (tea.Model, tea.Cmd) {
	m.urlInput.SetValue("")
	m.urlInput.Focus()
	return m, nil
}

func (m *model) handleGoBack() (tea.Model, tea.Cmd) {
	activeTab := m.activeTabPtr()
	if activeTab != nil && activeTab.canGoBack() {
		activeTab.goBack()
		m.updateLoading("Going back...")
		m.content = "🔄 Going back..."
		m.readerMode = false
		activeTab.ReaderMode = false
		return m, fetchContentWithLinks(activeTab.URL, m.activeTab)
	}
	return m, nil
}

func (m *model) handleGoForward() (tea.Model, tea.Cmd) {
	activeTab := m.activeTabPtr()
	if activeTab != nil && activeTab.canGoForward() {
		activeTab.goForward()
		m.updateLoading("Going forward...")
		m.content = "🔄 Going forward..."
		m.readerMode = false
		activeTab.ReaderMode = false
		return m, fetchContentWithLinks(activeTab.URL, m.activeTab)
	}
	return m, nil
}

func (m *model) handleEscape() (tea.Model, tea.Cmd) {
	if m.showHistory || m.showBookmarks || m.showSearch || m.showImages {
		m.showHistory = false
		m.showBookmarks = false
		m.showSearch = false
		m.showImages = false
		m.readerMode = false
		m.currentImage = nil
		activeTab := m.activeTabPtr()
		if activeTab != nil && len(activeTab.History) > 0 && activeTab.CurrentPos >= 0 {
			m.updateLoading("Loading current page...")
			m.content = "🔄 Loading current page..."
			return m, fetchContentWithLinks(activeTab.History[activeTab.CurrentPos], m.activeTab)
		} else {
			m.content = "🌐 Enter a URL or search query to start browsing"
			m.setError("")
			if m.ready {
				m.viewport.SetContent(m.content)
			}
		}
	} else if m.readerMode {
		m.readerMode = false
		activeTab := m.activeTabPtr()
		if activeTab != nil {
			activeTab.ReaderMode = false
		}
		m.updateLoading("Loading original view...")
		m.content = "🔄 Loading original view..."
		currentURL := activeTab.History[activeTab.CurrentPos]
		m.urlInput.SetValue("")
		return m, fetchContentWithLinks(currentURL, m.activeTab)
	}
	return m, nil
}

func (m *model) handleOpenImage() (tea.Model, tea.Cmd) {
	if m.currentImage != nil {
		currentImg := m.currentImage
		m.currentImage = nil // Reset so we don't get stuck in image view
		m.content = fmt.Sprintf("📤 Opening image in external viewer: %s", currentImg.URL)
		if m.ready {
			m.viewport.SetContent(m.content)
		}
		return m, m.openImageExternally(currentImg.URL)
	}
	return m, nil
}

// Message handlers
func (m *model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	headerHeight := 4
	footerHeight := 2

	if !m.ready {
		m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
		m.viewport.YPosition = headerHeight
		m.viewport.SetContent(m.content)
		m.ready = true
	} else {
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight
	}

	m.urlInput.Width = msg.Width - 2
	return m, nil
}

func (m *model) handleFetchContent(msg fetchContentMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.content = msg.content
	m.links = msg.links
	m.images = msg.images

	m.completeLoading(msg.loadTime, msg.pageSize, msg.statusCode, len(msg.links))

	if msg.tabID >= 0 && msg.tabID < len(m.tabs) {
		m.tabs[msg.tabID].Content = msg.content
		m.tabs[msg.tabID].Links = msg.links
		m.tabs[msg.tabID].Images = msg.images
	}

	m.showHistory = false
	m.showBookmarks = false
	m.showSearch = false
	m.showImages = false
	if m.ready {
		m.viewport.SetContent(m.content)
		m.viewport.GotoTop()
	}

	return m, nil
}

func (m *model) handleSearchResults(msg searchResultsMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.searchResults = msg.results
	m.showSearch = true
	m.content = m.renderSearchResults(msg.query, msg.results)
	m.setError("")
	if m.ready {
		m.viewport.SetContent(m.content)
		m.viewport.GotoTop()
	}
	return m, nil
}

func (m *model) handleError(msg errorMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.content = fmt.Sprintf("❌ Error: %v\n\nPress Enter to try another URL or search", msg.err)
	m.setError(msg.err.Error())

	if msg.tabID >= 0 && msg.tabID < len(m.tabs) {
		m.tabs[msg.tabID].Content = m.content
	}

	m.showHistory = false
	m.showBookmarks = false
	m.showSearch = false
	m.showImages = false
	m.readerMode = false
	if m.ready {
		m.viewport.SetContent(m.content)
	}

	return m, nil
}

func (m *model) openImageExternally(imageURL string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd

		// Detect terminal and use appropriate method
		term := os.Getenv("TERM")

		if strings.Contains(term, "kitty") {
			// Kitty terminal - display inline
			cmd = exec.Command("kitty", "+kitten", "icat", imageURL)
		} else {
			// Try system default first
			cmd = exec.Command("imv", imageURL)
		}

		if err := cmd.Start(); err != nil {
			// Fallback to other viewers
			fallbacks := []string{"imv", "shotwell", "feh", "gpicview"}
			for _, viewer := range fallbacks {
				cmd = exec.Command(viewer, imageURL)
				if err := cmd.Start(); err == nil {
					return nil
				}
			}
			return errorMsg{err: fmt.Errorf("failed to open image with any viewer")}
		}
		return nil
	}
}

func fetchContentWithLinks(pageURL string, tabID int) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		doc, err := fetchHTML(pageURL)
		loadTime := time.Since(start)

		if err != nil {
			return errorMsg{err: err, tabID: tabID}
		}

		rawContent, links, images := extractContentWithLinks(doc, pageURL)
		pageSize := len(rawContent)

		// DEBUG: Check what's being extracted
		log.Printf("DEBUG: Found %d images, %d links, content length: %d",
			len(images), len(links), len(rawContent))

		styledContent, err := renderWithStyle(rawContent)
		if err != nil {
			return errorMsg{err: err, tabID: tabID}
		}

		return fetchContentMsg{
			content:    styledContent,
			links:      links,
			images:     images,
			tabID:      tabID,
			loadTime:   loadTime,
			pageSize:   pageSize,
			statusCode: 200,
		}
	}
}

func fetchContentWithReaderMode(pageURL string, tabID int) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		doc, err := fetchHTML(pageURL)
		loadTime := time.Since(start)

		if err != nil {
			return errorMsg{err: err, tabID: tabID}
		}

		rawContent, links := extractReaderContent(doc, pageURL)
		pageSize := len(rawContent)

		styledContent, err := renderWithStyle(rawContent)
		if err != nil {
			return errorMsg{err: err, tabID: tabID}
		}

		return fetchContentMsg{
			content:    styledContent,
			links:      links,
			tabID:      tabID,
			loadTime:   loadTime,
			pageSize:   pageSize,
			statusCode: 200,
		}
	}
}

// NEW: Render status panel (bottom panel)
func (m *model) renderStatusPanel() string {
	status := m.status

	var statusText string

	if status.Loading {
		// Show loading animation and stage
		dots := strings.Repeat(".", (int(time.Now().Unix())%3)+1)
		statusText = fmt.Sprintf("🔄 %s%s", status.LoadingStage, dots)
	} else if status.Error != "" {
		// Show error
		statusText = fmt.Sprintf("❌ %s", status.Error)
	} else if status.LoadTime > 0 {
		// Show success status with metrics
		if status.StatusCode != 200 {
			statusText = fmt.Sprintf("⚠️ HTTP %d | ⏱️ %v | 📄 %d KB | 🔗 %d links",
				status.StatusCode,
				status.LoadTime.Round(time.Millisecond),
				status.PageSize/1024,
				status.LinkCount)
		} else {
			statusText = fmt.Sprintf("✅ Loaded | ⏱️ %v | 📄 %d KB | 🔗 %d links | 🖼️ %d images",
				status.LoadTime.Round(time.Millisecond),
				status.PageSize/1024,
				status.LinkCount,
				len(m.images))
		}
	} else {
		// Ready state
		activeTab := m.activeTabPtr()
		if activeTab != nil && len(activeTab.History) > 0 && activeTab.CurrentPos >= 0 {
			statusText = "✅ Ready"
		} else {
			statusText = "🌐 Enter a URL to start browsing"
		}
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		Width(m.viewport.Width).
		Align(lipgloss.Left).
		Render(statusText)
}

// Render tab bar
func (m *model) renderTabBar() string {
	if len(m.tabs) == 0 {
		return ""
	}

	var tabBar strings.Builder

	for i, tab := range m.tabs {
		title := tab.Title
		if title == "" {
			title = "New Tab"
		}
		if len(title) > 15 {
			title = title[:12] + "..."
		}

		if i == m.activeTab {
			tabBar.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("62")).
				Padding(0, 1).
				Render(fmt.Sprintf("%d: %s", i+1, title)))
		} else {
			tabBar.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Background(lipgloss.Color("235")).
				Padding(0, 1).
				Render(fmt.Sprintf("%d: %s", i+1, title)))
		}

		if i < len(m.tabs)-1 {
			tabBar.WriteString(" ")
		}
	}

	return tabBar.String()
}

// Enhanced status view
func (m *model) statusView() string {
	activeTab := m.activeTabPtr()
	if activeTab == nil {
		return "🌐 Terminal Browser | No tabs"
	}

	status := "🌐 Terminal Browser"

	if len(activeTab.History) > 0 && activeTab.CurrentPos >= 0 {
		currentURL := activeTab.History[activeTab.CurrentPos]
		if len(currentURL) > 30 {
			currentURL = currentURL[:27] + "..."
		}
		status += " | " + currentURL

		if m.isBookmarked(currentURL) {
			status += " | ⭐"
		}

		if m.readerMode {
			status += " | 📖"
		}
	}

	// Simplified navigation hints
	navHints := "↑↓ Scroll"
	if activeTab.canGoBack() {
		navHints += " | ← Back"
	}
	if activeTab.canGoForward() {
		navHints += " | → Forward"
	}
	navHints += " | ? Help"

	status += " | " + navHints

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("62")).
		Padding(0, 1).
		Width(m.viewport.Width).
		Align(lipgloss.Left).
		Render(status)
}

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

func extractReaderContent(doc *goquery.Document, baseURL string) (string, []Link) {
	var content strings.Builder
	var links []Link
	var images []ImageInfo

	selectors := []string{
		"article", "main", "[role='main']", ".content", ".post-content",
		".entry-content", ".article-content", ".post-body", ".story-content", ".main-content",
	}

	var mainContent *goquery.Selection
	for _, selector := range selectors {
		mainContent = doc.Find(selector).First()
		if mainContent.Length() > 0 {
			break
		}
	}

	if mainContent == nil || mainContent.Length() == 0 {
		mainContent = doc.Find("body")
	}

	mainContent.Find("nav, header, footer, aside, .sidebar, .ad, .advertisement, .navbar, .menu, .navigation, script, style, iframe, .comments, .social-share").
		Remove()

	linkCounter := 1
	mainContent.Find("h1, h2, h3, h4, h5, h6, p, a, blockquote, ul, ol, li").
		Each(func(i int, s *goquery.Selection) {
			tagName := goquery.NodeName(s)
			text := strings.TrimSpace(s.Text())

			if text == "" || len(text) < 10 {
				return
			}

			if isNavigationText(text) {
				return
			}

			switch tagName {
			case "h1":
				content.WriteString(fmt.Sprintf("# %s\n\n", text))
			case "h2":
				content.WriteString(fmt.Sprintf("## %s\n\n", text))
			case "h3":
				content.WriteString(fmt.Sprintf("### %s\n\n", text))
			case "h4", "h5", "h6":
				content.WriteString(fmt.Sprintf("#### %s\n\n", text))
			case "p":
				if len(text) > 20 {
					content.WriteString(fmt.Sprintf("%s\n\n", text))
				}
			case "blockquote":
				content.WriteString(fmt.Sprintf("> %s\n\n", text))
			case "ul", "ol":
				s.Find("li").Each(func(j int, li *goquery.Selection) {
					liText := strings.TrimSpace(li.Text())
					if len(liText) > 10 {
						content.WriteString(fmt.Sprintf("- %s\n\n", liText))
					}
				})
				content.WriteString("\n")
			case "a":
				if href, exists := s.Attr("href"); exists && href != "" {
					fullURL := resolveURL(baseURL, href)
					if shouldIncludeLink(text, href) && strings.HasPrefix(fullURL, "http") {
						links = append(links, Link{
							Number:  linkCounter,
							Text:    text,
							URL:     href,
							FullURL: fullURL,
						})
						if text == "" {
							text = fullURL
						}
						content.WriteString(
							fmt.Sprintf("[%d] %s ", linkCounter, text),
						) // ! removed \n\n
						linkCounter++
					}
				}
			}
		})

	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		alt, _ := s.Attr("alt")
		if src != "" && shouldIncludeImage(alt, src) {
			fullURL := resolveURL(baseURL, src)
			images = append(images, ImageInfo{
				Number:  len(images) + 1,
				URL:     fullURL,
				AltText: alt,
				Type:    getImageType(src),
			})
		}
	})

	// Add images to content
	if len(images) > 0 {
		content.WriteString("\n--- Images ---\n\n")
		for _, img := range images {
			altText := img.AltText
			if altText == "" {
				altText = "Image"
			}
			content.WriteString(fmt.Sprintf("🖼️ %s\n", altText))
		}
	}

	return content.String(), links
}

func shouldIncludeImage(alt, src string) bool {
	// Skip common icon/logo file patterns
	skipPatterns := []string{
		"icon", "logo", "sprite", "button", "arrow",
		"spacer", "pixel", "tracking", "ads",
	}

	lowerSrc := strings.ToLower(src)
	lowerAlt := strings.ToLower(alt)

	for _, pattern := range skipPatterns {
		if strings.Contains(lowerSrc, pattern) ||
			strings.Contains(lowerAlt, pattern) {
			return false
		}
	}

	return true
}

func isNavigationText(text string) bool {
	navigationWords := []string{
		"home", "about", "contact", "login", "sign up", "register", "shop", "buy now",
		"subscribe", "follow", "share", "menu", "navigation", "categories", "tags",
		"archives", "search", "advertise", "sponsored", "popular", "trending",
	}
	lowerText := strings.ToLower(text)
	for _, word := range navigationWords {
		if strings.Contains(lowerText, word) {
			return true
		}
	}
	return false
}

func shouldIncludeLink(text, href string) bool {
	if isNavigationText(text) {
		return false
	}
	skipURLs := []string{
		"#",
		"javascript:",
		"mailto:",
		"tel:",
		"/home",
		"/about",
		"/contact",
		"/login",
		"/signup",
		"/register",
		"/shop",
		"/buy",
	}
	for _, skip := range skipURLs {
		if strings.Contains(href, skip) {
			return false
		}
	}
	return true
}

func loadBookmarks(filename string) []Bookmark {
	data, err := os.ReadFile(filename)
	if err != nil {
		return []Bookmark{}
	}
	var bookmarks []Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		log.Printf("Error loading bookmarks: %v", err)
		return []Bookmark{}
	}
	return bookmarks
}

func (m *model) saveBookmarks() {
	data, err := json.MarshalIndent(m.bookmarks, "", "  ")
	if err != nil {
		log.Printf("Error saving bookmarks: %v", err)
		return
	}
	if err := os.WriteFile(m.bookmarkFile, data, 0644); err != nil {
		log.Printf("Error writing bookmarks file: %v", err)
	}
}

func (m *model) isBookmarked(url string) bool {
	for _, bookmark := range m.bookmarks {
		if bookmark.URL == url {
			return true
		}
	}
	return false
}

func (m *model) addBookmark(title string) {
	activeTab := m.activeTabPtr()
	if activeTab == nil || len(activeTab.History) == 0 || activeTab.CurrentPos < 0 {
		return
	}
	currentURL := activeTab.History[activeTab.CurrentPos]
	if m.isBookmarked(currentURL) {
		return
	}
	bookmark := Bookmark{
		Title: title,
		URL:   currentURL,
	}
	m.bookmarks = append(m.bookmarks, bookmark)
	m.saveBookmarks()
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

func performSearch(query string) tea.Cmd {
	return func() tea.Msg {
		searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
		doc, err := fetchHTML(searchURL)
		if err != nil {
			return errorMsg{err: err}
		}
		var results []SearchResult
		resultCounter := 1
		doc.Find(".result").Each(func(i int, s *goquery.Selection) {
			if resultCounter > 10 {
				return
			}
			titleElem := s.Find(".result__a")
			title := strings.TrimSpace(titleElem.Text())
			link, _ := titleElem.Attr("href")
			snippet := strings.TrimSpace(s.Find(".result__snippet").Text())
			if title != "" && link != "" {
				if strings.HasPrefix(link, "//duckduckgo.com/l/") {
					if parsed, err := url.Parse(link); err == nil {
						if realURL := parsed.Query().Get("uddg"); realURL != "" {
							link = realURL
						}
					}
				}
				results = append(results, SearchResult{
					Number:  resultCounter,
					Title:   title,
					URL:     link,
					Snippet: snippet,
				})
				resultCounter++
			}
		})
		if len(results) == 0 {
			results = append(results, SearchResult{
				Number:  1,
				Title:   fmt.Sprintf("Search for: %s", query),
				URL:     fmt.Sprintf("https://www.google.com/search?q=%s", url.QueryEscape(query)),
				Snippet: "Click to view search results on Google",
			})
		}
		return searchResultsMsg{
			query:   query,
			results: results,
		}
	}
}

func extractContentWithLinks(doc *goquery.Document, baseURL string) (string, []Link, []ImageInfo) {
	var content strings.Builder
	var links []Link
	var images []ImageInfo

	linkCounter := 1
	imageCounter := 1

	// Extract images first
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		alt, _ := s.Attr("alt")

		// Try lazy loading attributes if src is empty
		if src != "" {
			// Clean up the URL
			src = cleanURL(src)
			fullURL := resolveURL(baseURL, src)

			// Check if this image is inside a link
			parentLink := s.ParentsFiltered("a").First()
			isLinked := parentLink.Length() > 0
			linkURL := ""

			if isLinked {
				linkHref, _ := parentLink.Attr("href")
				if linkHref != "" {
					linkURL = resolveURL(baseURL, linkHref)
				}
			}

			images = append(images, ImageInfo{
				Number:   imageCounter,
				URL:      fullURL,
				AltText:  alt,
				Type:     getImageType(src),
				IsLinked: isLinked,
				LinkURL:  linkURL, // Store the link destination
			})

			imageCounter++
		}
	})

	// Extract links (including those that contain images)
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		text := strings.TrimSpace(s.Text())
		// If the link contains an image but no text, use the image alt text
		if text == "" {
			img := s.Find("img").First()
			if img.Length() > 0 {
				alt, _ := img.Attr("alt")
				if alt != "" {
					text = fmt.Sprintf("🖼️ %s", alt)
				} else {
					text = "🖼️ Image link"
				}
			}
		}

		if text == "" {
			text = href
		}

		fullURL := resolveURL(baseURL, href)
		if strings.HasPrefix(fullURL, "http") {
			links = append(links, Link{
				Number:  linkCounter,
				Text:    text,
				URL:     href,
				FullURL: fullURL,
			})
			linkCounter++
		}
	})

	// Extract regular content (headers, paragraphs)
	doc.Find("h1, h2, h3, h4, h5, h6, p").Each(func(i int, s *goquery.Selection) {
		tagName := goquery.NodeName(s)
		text := strings.TrimSpace(s.Text())

		if text != "" {
			switch tagName {
			case "h1":
				content.WriteString(fmt.Sprintf("# %s\n\n", text))
			case "h2":
				content.WriteString(fmt.Sprintf("## %s\n\n", text))
			case "h3":
				content.WriteString(fmt.Sprintf("### %s\n\n", text))
			case "h4", "h5", "h6":
				content.WriteString(fmt.Sprintf("#### %s\n\n", text))
			case "p":
				content.WriteString(fmt.Sprintf("%s\n\n", text))
			}
		}
	})

	// Add image section to content
	if len(images) > 0 {
		content.WriteString("\n--- 🖼️ Images ---\n\n")
		for _, img := range images {
			altText := img.AltText
			if altText == "" {
				altText = "No description"
			}

			indicator := "🖼️"
			if img.IsLinked {
				indicator = "🔗🖼️" // Show that image is linked
			}

			content.WriteString(fmt.Sprintf("%s [img%d] %s\n", indicator, img.Number, altText))
			content.WriteString(fmt.Sprintf("    %s\n\n", img.URL))
		}
	}

	return content.String(), links, images
}

func cleanURL(url string) string {
	// Remove newlines and carriage returns
	url = strings.ReplaceAll(url, "\n", "")
	url = strings.ReplaceAll(url, "\r", "")

	// Remove extra spaces
	url = strings.Join(strings.Fields(url), "")

	return url
}

func getImageType(url string) string {
	url = strings.ToLower(url)

	switch {
	case strings.Contains(url, ".jpg") || strings.Contains(url, ".jpeg"):
		return "JPEG"
	case strings.Contains(url, ".png"):
		return "PNG"
	case strings.Contains(url, ".gif"):
		return "GIF"
	case strings.Contains(url, ".webp"):
		return "WebP"
	case strings.Contains(url, ".svg"):
		return "SVG"
	case strings.Contains(url, ".bmp"):
		return "BMP"
	case strings.Contains(url, ".ico"):
		return "ICO"
	case strings.Contains(url, ".tiff") || strings.Contains(url, ".tif"):
		return "TIFF"
	default:
		return "Image"
	}
}

func resolveURL(baseURL, href string) string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func fetchHTML(url string) (*goquery.Document, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set(
		"User-Agent",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	)
	req.Header.Set(
		"Accept",
		"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
	)
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %v", err)
	}
	return doc, nil
}

func renderWithStyle(content string) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		return "", err
	}
	return renderer.Render(content)
}

// Init runs when the program starts
func (m *model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		textinput.Blink,
	)
}

func main() {
	// Call ParseFlags ONLY here
	config := ParseFlags()
	// Pass config to InitialModel
	m := InitialModel(config)

	// Update tea program initialization with mouse support based on config
	opts := []tea.ProgramOption{
		tea.WithAltScreen(),
	}

	if config.EnableMouseSupport {
		opts = append(opts, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(&m, opts...)

	if _, err := p.Run(); err != nil {
		log.Fatal("Error running program:", err)
	}
}
