package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

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
