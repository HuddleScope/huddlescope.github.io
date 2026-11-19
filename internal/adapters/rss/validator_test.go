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
		name        string
		setupFn     func() *httptest.Server
		source      domain.FeedSource
		wantStatus  int
		wantVerdict Verdict
	}{
		{
			name: "404 Not Found",
			setupFn: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			source:      domain.FeedSource{ID: "test", URL: "http://example.com/feed", Enabled: true},
			wantStatus:  http.StatusNotFound,
			wantVerdict: VerdictFailed,
		},
		{
			name: "Not Modified",
			setupFn: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/rss+xml")
					w.WriteHeader(http.StatusNotModified)
				}))
			},
			source:      domain.FeedSource{ID: "test", URL: "http://example.com/feed", Enabled: true},
			wantStatus:  http.StatusNotModified,
			wantVerdict: VerdictFailed,
		},
		{
			name: "Empty Feed",
			setupFn: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/rss+xml")
					w.WriteHeader(http.StatusOK)
					//nolint:errcheck
					w.Write([]byte("<?xml version=\"1.0\"?><rss version=\"2.0\"><channel></channel></rss>"))
				}))
			},
			source:      domain.FeedSource{ID: "test", URL: "http://example.com/feed", Enabled: true},
			wantVerdict: VerdictFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupFn()
			defer server.Close()

			client := server.Client()
			validator := NewValidator(client, SourceCheckOptions{})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tt.source.URL = server.URL
			result, err := validator.Check(ctx, tt.source)
			assert.NoError(t, err)

			if tt.wantStatus != 0 {
				assert.Equal(t, tt.wantStatus, result.HTTPStatus)
			}
			assert.Equal(t, tt.wantVerdict, result.Verdict)
		})
	}
}
