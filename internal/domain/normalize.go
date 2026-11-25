package domain

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
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

// DisplayNormalizeTitle normalizes a title for display purposes while
// preserving the original casing and punctuation exactly as published.
// It trims whitespace, collapses multiple whitespace characters into a single
// space, and applies NFC Unicode normalization. Unlike NormalizeTitle it does
// NOT lowercase, so "Chiefs sign RB James Cook" is preserved as-is (issue #201).
func DisplayNormalizeTitle(title string) string {
	if title == "" {
		return ""
	}

	// Trim leading/trailing whitespace
	s := strings.TrimSpace(title)

	// Collapse multiple whitespace characters into a single space
	fields := strings.Fields(s)
	s = strings.Join(fields, " ")

	// Apply NFC Unicode normalization (no lowercasing: display keeps case)
	s = NormalizeUnicode(s)

	return s
}

// NormalizeUnicode applies NFC normalization to the input string.
func NormalizeUnicode(s string) string {
	return norm.NFC.String(s)
}

// CanonicalURLEntry returns a normalized URL string for FeedEntry.
// Unlike CanonicalURL, this doesn't return an error for invalid URLs - it returns the original.
// DEPRECATED: Use CanonicalURL instead for proper URL canonicalization.
func CanonicalURLEntry(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	return strings.TrimSpace(rawURL)
}

// CanonicalURL returns a fully canonicalized URL that treats:
// - Tracking parameters (utm_*, fbclid, fb_xo_api, etc.)
// - AMP parameters (?amp=1, &amp)
// - Case differences (scheme, host)
// - www. prefix
// - Trailing slashes
// as equivalent. Returns trimmed original on parse failure.
func CanonicalURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	rawURL = strings.TrimSpace(rawURL)

	// Parse the URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// On parse failure, return trimmed original (matches CanonicalURLEntry behavior)
		return rawURL
	}

	// Lowercase scheme and host
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	// Strip www. prefix from host
	parsed.Host = strings.TrimPrefix(parsed.Host, "www.")

	// Remove trailing slash (except for root paths)
	if parsed.Path != "" && strings.HasSuffix(parsed.Path, "/") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}

	// Remove tracking parameters (sorted for determinism)
	query := parsed.Query()

	// Parameters to strip (case-insensitive matching)
	trackingParams := map[string]bool{
		"utm_source": true, "utm_medium": true, "utm_campaign": true,
		"utm_term": true, "utm_content": true, "fbclid": true,
		"fb_xo_api": true, "fb_ref": true, "fb_action_types": true,
		"fb_action_ids": true, "fb_action_names": true, "fb_source": true,
		"ref": true, "campaign": true, "source": true, "medium": true,
		"type": true, "id": true, "cid": true, "aid": true, "nid": true,
		"oid": true, "mid": true, "iid": true, "mt": true, "ct": true,
		"dt": true, "st": true, "pt": true, "vt": true, "wt": true,
		"xt": true, "ut": true, "_openstat": true, "utm_referer": true,
		"utm_referrer": true, "ref_src": true, "ref_url": true, "trk": true,
		"trk_info": true, "track": true, "trackid": true, "trackingScript": true,
		"tracking": true, "tracking_id": true, "trackingid": true,
		"tracking_pixel": true, "u": true, "utr": true, "v": true, "w": true,
		"x": true, "y": true, "z": true, "action": true, "action_type": true,
		"action_id": true, "action_name": true, "action_object": true,
		"client_id": true, "cookie": true, "code": true, "callback": true,
		"redirect": true, "redirect_uri": true, "redirect_url": true,
		"return": true, "return_to": true, "returnurl": true, "return_url": true,
		"returnUri": true, "returnURL": true, "destination": true, "dest": true,
		"next": true, "continue": true, "path": true, "file": true, "page": true,
		"page_uri": true, "pageuri": true, "pagename": true, "pagename_uri": true,
		"pagenameuri": true, "title": true, "message": true, "description": true,
		"picture": true, "caption": true, "name": true, "redirect_target": true,
		"response_type": true, "scope": true, "state": true, "response": true,
		"status": true, "error": true, "error_uri": true, "error_description": true,
		"error_code": true, "error_message": true, "request_id": true,
		"requestid": true, "timestamp": true, "timestamp_ms": true, "version": true,
		"api_key": true, "apikey": true, "api_key_id": true, "apikeyid": true,
		"access_token": true, "accesstoken": true, "token": true, "bearer": true,
		"auth": true, "authorization": true, "cookie_id": true, "session_id": true,
		"sessionid": true, "visitor_id": true, "visitorid": true, "user_id": true,
		"userid": true, "client_secret": true,
	}

	// Build sorted list of keys to remove (for deterministic behavior)
	var keysToRemove []string
	for key := range query {
		lowerKey := strings.ToLower(key)
		if trackingParams[lowerKey] {
			keysToRemove = append(keysToRemove, key)
		}
	}
	sort.Strings(keysToRemove)

	// Remove tracking parameters
	for _, key := range keysToRemove {
		query.Del(key)
	}

	// Rebuild query string
	parsed.RawQuery = query.Encode()

	// Reconstruct URL
	canonicalized := parsed.String()

	return canonicalized
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
