package main

import (
	"encoding/json"
	"log"
	"os"
)

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
