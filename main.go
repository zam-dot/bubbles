package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url" // For robust URL parsing
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery" // HTML parsing and DOM traversal
	tea "github.com/charmbracelet/bubbletea"
)

// Link represents a clickable link within the content
type Link struct {
	Text string // The visible link text
	URL  string // The destination URL
	ID   int    // Unique identifier for tracking in the content
}

func main() {
	// Basic command line validation
	if len(os.Args) < 2 {
		log.Println("Usage: bubbles <url>")
		os.Exit(1)
	}

	// Get the starting URL from command line arguments
	url := os.Args[1]

	// Fetch and extract content from the URL
	content, links, err := fetchAndExtractContent(url)
	if err != nil {
		log.Fatal("Error:", err)
	}

	// Initialize and start the TUI application
	// tea.WithAltScreen() gives us a clean terminal canvas to work with
	p := tea.NewProgram(initialModel(content, url, links), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal("Error running TUI:", err)
	}
}

// fetchAndExtractContent is the main workhorse - fetches a URL and extracts readable content
func fetchAndExtractContent(url string) (string, []Link, error) {
	// Create an HTTP client with proper headers to avoid being blocked
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", nil, err
	}

	// Set realistic browser headers to avoid bot detection
	req.Header.Set(
		"User-Agent",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
	)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	// Execute the HTTP request
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close() // Always close the response body

	// Parse the HTML document using goquery (like jQuery for Go)
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", nil, err
	}

	// Remove unwanted elements that aren't part of main content
	// This is key to cleaning up the page
	doc.Find("script, style, meta, link, noscript, svg, iframe").Remove()

	var content strings.Builder // Efficient string concatenation
	var links []Link            // Store all extracted links
	linkCounter := 0            // Unique ID counter for links

	// Extract and style the page title
	title := strings.TrimSpace(doc.Find("title").Text())
	content.WriteString(titleStyle.Render("📖 " + title))
	content.WriteString("\n\n")

	// Find the main content area using smart heuristics
	mainContent := findMainContent(doc)

	// Extract readable text and links from the main content
	extractTextWithLinks(mainContent, &content, &links, &linkCounter, url)

	// Apply final document styling (margins, etc.)
	return docStyle.Render(content.String()), links, nil
}

// extractTextWithLinks processes HTML elements and converts them to styled text
func extractTextWithLinks(
	sel *goquery.Selection,
	content *strings.Builder,
	links *[]Link,
	linkCounter *int,
	baseURL string,
) {
	// Look for meaningful content elements
	sel.Find("p, h1, h2, h3, h4, h5, h6, li, blockquote").Each(func(i int, s *goquery.Selection) {
		// Extract text content and any embedded links
		text, extractedLinks := extractTextFromElement(s, linkCounter, baseURL)
		*links = append(*links, extractedLinks...)

		// Only process substantial text (not tiny fragments)
		if text != "" && len(strings.TrimSpace(text)) > 10 {
			tagName := goquery.NodeName(s) // Get the HTML tag name

			// Apply appropriate styling based on element type
			switch tagName {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				content.WriteString(headingStyle.Render(text))
				content.WriteString("\n\n") // Extra space after headings
			case "li":
				content.WriteString("• " + paragraphStyle.Render(text))
				content.WriteString("\n") // Single line for list items
			case "blockquote":
				content.WriteString(blockquoteStyle.Render(text))
				content.WriteString("\n\n") // Space after blockquotes
			default: // paragraphs
				content.WriteString(paragraphStyle.Render(text))
				content.WriteString("\n\n") // Space between paragraphs
			}
		}
	})
}

// extractTextFromElement processes a single HTML element, handling text and links
func extractTextFromElement(
	sel *goquery.Selection,
	linkCounter *int,
	baseURL string,
) (string, []Link) {
	var links []Link
	var textParts []string

	// Process all child nodes (text nodes and element nodes)
	sel.Contents().Each(func(i int, s *goquery.Selection) {
		if goquery.NodeName(s) == "#text" {
			// Regular text node - just add the text
			textParts = append(textParts, strings.TrimSpace(s.Text()))
		} else if goquery.NodeName(s) == "a" {
			// Anchor tag - extract link information
			if href, exists := s.Attr("href"); exists {
				linkText := strings.TrimSpace(s.Text())
				if linkText != "" {
					// Convert relative URLs to absolute URLs
					resolvedURL := resolveURL(href, baseURL)
					if resolvedURL != "" && !strings.HasPrefix(resolvedURL, "#") {
						links = append(links, Link{
							Text: linkText,
							URL:  resolvedURL,
							ID:   *linkCounter,
						})
						// Use marker [0], [1], etc. to indicate link positions in text
						textParts = append(textParts, fmt.Sprintf("[%d]", *linkCounter))
						*linkCounter++
					} else {
						// Include the link text but don't make it clickable (for anchors, etc.)
						textParts = append(textParts, linkText)
					}
				}
			}
		}
	})

	// Join all text parts with spaces
	return strings.Join(textParts, " "), links
}

// resolveURL converts relative URLs to absolute URLs using Go's robust url package
func resolveURL(href, baseURL string) string {
	if href == "" {
		return ""
	}

	// Skip non-http links that we can't handle
	if strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:") ||
		strings.HasPrefix(href, "javascript:") {
		return ""
	}

	// Parse the base URL to understand its structure
	base, err := url.Parse(baseURL)
	if err != nil {
		return "" // Invalid base URL
	}

	// Use Go's URL resolution to handle all edge cases
	resolved, err := base.Parse(href)
	if err != nil {
		return "" // Invalid href
	}

	// Only allow http/https schemes for web browsing
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}

	return resolved.String()
}

// normalizeURL handles user-friendly URL input like "google.com" -> "https://google.com"
func normalizeURL(inputURL, currentURL string) string {
	if inputURL == "" {
		return ""
	}

	inputURL = strings.TrimSpace(inputURL)

	// If it already has a scheme, use as-is
	if strings.HasPrefix(inputURL, "http://") || strings.HasPrefix(inputURL, "https://") {
		if _, err := url.Parse(inputURL); err == nil {
			return inputURL
		}
		return "" // Invalid URL
	}

	// Protocol-relative URLs (//example.com)
	if strings.HasPrefix(inputURL, "//") {
		resolved := "https:" + inputURL
		if _, err := url.Parse(resolved); err == nil {
			return resolved
		}
		return ""
	}

	// Absolute paths (/about, /contact)
	if strings.HasPrefix(inputURL, "/") {
		return resolveURL(inputURL, currentURL)
	}

	// If it looks like a domain (contains dot, no spaces), assume https
	if strings.Contains(inputURL, ".") && !strings.Contains(inputURL, " ") {
		if !strings.Contains(inputURL, "://") {
			inputURL = "https://" + inputURL
		}

		if parsed, err := url.Parse(inputURL); err == nil && parsed.Host != "" {
			return parsed.String()
		}
	}

	// Otherwise, treat as relative to current page
	return resolveURL(inputURL, currentURL)
}

// findMainContent uses heuristics to locate the main article content
func findMainContent(doc *goquery.Document) *goquery.Selection {
	// First, try common content container selectors
	contentSelectors := []string{
		"article", "main", "[role='main']",
		".content", ".main-content", "#content", "#main",
		"#mw-content-text", ".mw-parser-output", // Wikipedia
	}

	for _, selector := range contentSelectors {
		if doc.Find(selector).Length() > 0 {
			return doc.Find(selector).First()
		}
	}

	// Fallback: use text density analysis to find the best content
	return findBestContentByDensity(doc.Find("body"))
}

// findBestContentByDensity uses heuristics to find the element with the most substantive text
func findBestContentByDensity(sel *goquery.Selection) *goquery.Selection {
	bestScore := 0.0
	var bestElement *goquery.Selection

	// Examine potential content containers
	sel.Find("div, section, main, article").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		linkCount := s.Find("a").Length()

		// Skip elements that are too short or have too many links (likely navigation)
		if len(text) < 100 || float64(linkCount)/float64(len(strings.Fields(text))) > 0.3 {
			return
		}

		// Calculate text density (ratio of text to HTML)
		textLength := len(text)
		htmlLength := len(s.Text()) // Rough HTML length

		if htmlLength > 0 {
			density := float64(textLength) / float64(htmlLength)
			wordCount := len(strings.Fields(text))

			// Score favors longer, denser content
			score := density * float64(wordCount)

			if score > bestScore {
				bestScore = score
				bestElement = s
			}
		}
	})

	if bestElement != nil {
		return bestElement
	}

	// Final fallback: use the entire body
	return sel
}
