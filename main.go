package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
