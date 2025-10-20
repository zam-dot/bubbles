package main

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
