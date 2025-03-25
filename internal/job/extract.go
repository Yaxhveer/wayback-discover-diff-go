package job

import (
	"bytes"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// extractHTMLFeatures processes an HTML document and extracts key features as a map.
func extractHTMLFeatures(htmlStr string) map[string]int {
	// Parse HTML and remove script/style tags
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	tagsToRemove := map[string]struct{}{
		"script": {}, "style": {},
		"noscript": {}, "meta": {},
		"img": {}, "audio": {},
		"video": {},
	}

	text := stripTags(doc, tagsToRemove)

	// Process text: lowercase, remove punctuation, split into words
	text = strings.ToLower(text)
	text = removePunctuation(text)

	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}

	var chunks []string
	for _, line := range lines {
		for _, phrase := range strings.Split(line, "  ") {
			trimmed := strings.TrimSpace(phrase)
			if trimmed != "" {
				chunks = append(chunks, trimmed)
			}
		}
	}

	text = strings.Join(chunks, "\n")
	words := strings.Fields(text)
	sort.Strings(words)

	wordCounts := make(map[string]int)
	for i := 0; i < len(words); {
		word := words[i]
		count := 1
		for i+count < len(words) && words[i+count] == word {
			count++
		}
		wordCounts[word] = count
		i += count
	}

	// wordCounts := make(map[string]int)
	// words := strings.Fields(text)
	// for _, word := range words {
	// 	wordCounts[word]++
	// }

	return wordCounts
}

func stripTags(doc *html.Node, tagsToRemove map[string]struct{}) string {
	var buffer bytes.Buffer
	var extractText func(*html.Node)
	extractText = func(n *html.Node) {
		if n.Type == html.TextNode {
			buffer.WriteString(n.Data + " ")
		} else if n.Type == html.ElementNode {
			if _, found := tagsToRemove[n.Data]; found {
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c)
		}
	}

	extractText(doc)
	return strings.TrimSpace(buffer.String())
}

// removePunctuation removes punctuation from a string.
func removePunctuation(s string) string {
	var b strings.Builder
	punct := map[rune]struct{}{
		'!': {}, '"': {}, '#': {}, '$': {}, '%': {}, '&': {}, '\'': {},
		'(': {}, ')': {}, '*': {}, '+': {}, ',': {}, '-': {}, '.': {},
		'/': {}, ':': {}, ';': {}, '<': {}, '=': {}, '>': {}, '?': {},
		'@': {}, '[': {}, '\\': {}, ']': {}, '^': {}, '_': {}, '`': {},
		'{': {}, '|': {}, '}': {}, '~': {},
	}
	for _, r := range s {
		if _, exists := punct[r]; exists {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
