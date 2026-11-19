package rss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestValidator_Check(t *testing.T) {
	tests := []struct {
		name      string
		setupServer func() *httptest.Server
		source    domain.FeedSource
		wantStatus    int
		wantType      string
		wantParseable bool
		wantItems     int
		wantVerdict   Verdict
	}{
		{
			name: "200 XML feed",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/rss+xml")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><item><title>Test</title><pubDate>Fri, 01 Nov 2024 00:00:00 -0000</pubDate><link>http://example.com/test</link></item></channel></rss>`))
				}))
			},
			source:   domain.FeedSource{ID: "test", URL: "http://example.com/feed", Enabled: true},
			wantStatus: http.StatusOK,
			wantType:  "application/rss+xml",
			wantParseable: true,
			wantItems:  1,
			wantVerdict: VerdictHealthy,
		},
		{
			name: "404 Not Found",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			source: domain.FeedSource{ID: "test", URL: "http://example.com/feed", Enabled: true},
			wantStatus: http.StatusNotFound,
			wantVerdict: VerdictFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			client := server.Client()
			validator := NewValidator(client, SourceCheckOptions{BodySizeLimit: 10 * 1024 * 1024})
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Update source URL to use test server
			tt.source.URL = server.URL

			result, err := validator.Check(ctx, tt.source)
			assert.Nil(t, err, "unexpected error")

			if tt.wantStatus != 0 {
				assert.Equal(t, tt.wantStatus, result.HTTPStatus, "unexpected HTTP status")
			}
			if tt.wantType != "" {
				assert.Contains(t, result.ContentType, tt.wantType, "unexpected content type")
			}
			assert.Equal(t, tt.wantParseable, result.Parseable, "unexpected parseable")
			assert.Equal(t, tt.wantItems, result.ItemCount, "unexpected item count")
			assert.Equal(t, tt.wantVerdict, result.Verdict, "unexpected verdict")
		})
	}
}

func TestValidator_Check_MalformedXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid xml"))
	}))
	defer server.Close()

	client := server.Client()
	validator := NewValidator(client, SourceCheckOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, _ := validator.Check(ctx, domain.FeedSource{ID: "test", URL: server.URL, Enabled: true})
	assert.Equal(t, VerdictFailed, result.Verdict)
	assert.Equal(t, false, result.Parseable)
}

func TestValidator_Check_EmptyFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`))
	}))
	defer server.Close()

	client := server.Client()
	validator := NewValidator(client, SourceCheckOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, _ := validator.Check(ctx, domain.FeedSource{ID: "test", URL: server.URL, Enabled: true})
	assert.Equal(t, VerdictFailed, result.Verdict)
	assert.Equal(t, 0, result.ItemCount)
}

func TestValidator_Check_304_NotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client := server.Client()
	validator := NewValidator(client, SourceCheckOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, _ := validator.Check(ctx, domain.FeedSource{ID: "test", URL: server.URL, Enabled: true})
	assert.Equal(t, VerdictFailed, result.Verdict)
	assert.Equal(t, http.StatusNotModified, result.HTTPStatus)
}
