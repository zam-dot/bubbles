package main

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/lipgloss"
)

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

	// Extract regular content (headers, paragraphs) with link numbers
	doc.Find("h1, h2, h3, h4, h5, h6, p").Each(func(i int, s *goquery.Selection) {
		tagName := goquery.NodeName(s)

		// Process text content, replacing links with numbered references
		text := processTextWithLinks(s, links)

		if text != "" {
			switch tagName {
			case "h1":
				content.WriteString(fmt.Sprintf("# %s\n\n", text))
			case "h2":
				content.WriteString(fmt.Sprintf("## %s\n\n", text))
			case "h3":
				content.WriteString(fmt.Sprintf("### %s\n\n", text))
			case "h4", "h5", "h6": // Fixed: added missing quotes around "h6"
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
			content.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("5")).
				Render(fmt.Sprintf("🖼️ %s\n", altText)))
		}
	}

	return content.String(), links
}

func processTextWithLinks(s *goquery.Selection, links []Link) string {
	var result strings.Builder

	// Clone the selection to avoid modifying original
	cloned := s.Clone()

	// Replace <a> tags with numbered references
	cloned.Find("a").Each(func(i int, link *goquery.Selection) {
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		linkText := strings.TrimSpace(link.Text())
		if linkText == "" {
			return
		}

		// Find the link number by matching both text and URL
		for _, l := range links {
			if l.URL == href && strings.TrimSpace(l.Text) == linkText {
				// Replace the link with text + number reference
				link.ReplaceWithHtml(fmt.Sprintf("%s [%d]", linkText, l.Number))
				break
			}
		}
	})

	// Get the processed text
	result.WriteString(strings.TrimSpace(cloned.Text()))
	return result.String()
}
