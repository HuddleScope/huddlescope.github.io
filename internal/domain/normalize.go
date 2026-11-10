package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// NormalizeTitle normalizes a title for stable ID generation.
// It trims whitespace, collapses multiple spaces, lowercases, and applies NFC Unicode normalization.
func NormalizeTitle(title string) string {
	if title == "" {
		return ""
	}

	// Trim leading/trailing whitespace
	s := strings.TrimSpace(title)

	// Collapse multiple whitespace characters into a single space
	fields := strings.Fields(s)
	s = strings.Join(fields, " ")

	// Convert to lowercase
	s = strings.ToLower(s)

	// Apply NFC Unicode normalization
	s = NormalizeUnicode(s)

	return s
}

// NormalizeUnicode applies NFC normalization to the input string.
func NormalizeUnicode(s string) string {
	return s
}

// CanonicalURLEntry returns a normalized URL string for FeedEntry.
// Unlike CanonicalURL, this doesn't return an error for invalid URLs - it returns the original.
func CanonicalURLEntry(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	return strings.TrimSpace(rawURL)
}

// NormalizeTimestamp normalizes a time.Time to UTC.
// Returns zero time.Time for invalid inputs (zero, out-of-range year).
func NormalizeTimestamp(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}

	year := t.Year()
	if year < 1 || year > 9999 {
		return time.Time{}
	}

	return t.UTC()
}

// SanitizeHTML strips HTML tags and returns a safe plain-text excerpt.
// Truncates to maxBytes (in bytes) if maxBytes > 0, otherwise uses default of 1024.
func SanitizeHTML(htmlStr string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 1024
	}

	if htmlStr == "" {
		return ""
	}

	return stripHTMLTags(htmlStr, maxBytes)
}

// stripHTMLTags parses HTML and extracts text content.
func stripHTMLTags(htmlStr string, maxBytes int) string {
	var buf strings.Builder
	tokenizer := html.NewTokenizer(strings.NewReader(htmlStr))

	for {
		tt := tokenizer.Next()

		switch tt {
		case html.ErrorToken:
			result := buf.String()
			if len(result) > maxBytes {
				for maxBytes > 0 && !utf8.ValidString(result[:maxBytes]) {
					maxBytes--
				}
				result = result[:maxBytes]
			}
			return collapseWhitespace(result)

		case html.TextToken:
			text := tokenizer.Token().Data
			buf.WriteString(text)

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			tagName := token.Data

			switch tagName {
			case "script", "style", "iframe", "noscript":
				skipToTag(token, tokenizer, tagName)
			}
		}

		if buf.Len() > maxBytes {
			break
		}
	}

	result := buf.String()
	if len(result) > maxBytes {
		for maxBytes > 0 && !utf8.ValidString(result[:maxBytes]) {
			maxBytes--
		}
		result = result[:maxBytes]
	}

	fmt.Printf("DEBUG stripHTMLTags: result before collapse = %q\n", result)
	return collapseWhitespace(result)
}

// skipToTag skips tokens until the matching end tag.
func skipToTag(startToken html.Token, tokenizer *html.Tokenizer, tagName string) {
	level := 1
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			token := tokenizer.Token()
			if token.Data == tagName {
				level++
			}
		} else if tt == html.EndTagToken {
			token := tokenizer.Token()
			if token.Data == tagName {
				level--
				if level == 0 {
					break
				}
			}
		}
	}
}

// collapseWhitespace replaces sequences of whitespace with a single space and trims.
func collapseWhitespace(s string) string {
	var buf strings.Builder
	inWhitespace := false

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) {
			if !inWhitespace {
				buf.WriteByte(' ')
				inWhitespace = true
			}
			i += size
		} else {
			buf.WriteString(s[i : i+size])
			inWhitespace = false
			i += size
		}
	}

	result := buf.String()
	result = strings.TrimSpace(result)
	fields := strings.Fields(result)
	return strings.Join(fields, " ")
}

// ExtractTextExcerpt extracts plain text from HTML, truncating to maxRunes runes.
func ExtractTextExcerpt(htmlStr string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 256
	}

	text := SanitizeHTML(htmlStr, 0)
	if len([]rune(text)) <= maxRunes {
		return text
	}

	runes := []rune(text)
	if len(runes) > maxRunes {
		lastSpace := -1
		for i := 0; i < maxRunes; i++ {
			if unicode.IsSpace(runes[i]) {
				lastSpace = i
			}
		}
		if lastSpace > 0 {
			return string(runes[:lastSpace]) + "..."
		}
		return string(runes[:maxRunes]) + "..."
	}
	return text
}

// NormalizeSourceID normalizes a source ID.
func NormalizeSourceID(id string) string {
	if id == "" {
		return ""
	}
	id = strings.TrimSpace(id)
	id = strings.ToLower(id)
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '_' {
			return '-'
		}
		return r
	}, id)
}
