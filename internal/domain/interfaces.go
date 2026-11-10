// Package domain contains core domain models and interfaces.
package domain

// IngestionResult represents the result of an ingestion run.
//type IngestionResult struct {
//	EntriesFetched int
//	StoriesCreated int
//	StoriesUpdated int
//	StoriesSkipped int
//	Errors         []error
//}

// ExportResult represents the result of an export run.
//type ExportResult struct {
//	StoriesExported int
//	Errors          []error
//}

// Repository interface for story storage.
//type Repository interface {
//	Upsert(story Story) error
//	List() ([]Story, error)
//	Count() (int, error)
//}

// FeedFetcher interface for fetching entries from feeds.
//type FeedFetcher interface {
//	Fetch(sourceID string) ([]FeedEntry, error)
//}

// NOTE: Types and interfaces are now defined in source.go and story.go.
// This file is kept for backward compatibility of the package declaration.
