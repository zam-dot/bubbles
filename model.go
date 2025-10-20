package main

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
