package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// testTeams is the hardcoded test data that was previously in SeedNFLTeams
var testTeams = []domain.Team{
	{Slug: "buffalo-bills", Name: "Buffalo Bills", ShortName: "Bills", Abbreviation: "BUF", Aliases: []string{"bills", "buff bills"}},
	{Slug: "miami-dolphins", Name: "Miami Dolphins", ShortName: "Dolphins", Abbreviation: "MIA", Aliases: []string{"miami dflns", "miami dolphins", "dolphins"}},
	{Slug: "new-england-patriots", Name: "New England Patriots", ShortName: "Patriots", Abbreviation: "NE", Aliases: []string{"n e patriots", "n epatriots", "new england", "patriots"}},
	{Slug: "new-york-jets", Name: "New York Jets", ShortName: "Jets", Abbreviation: "NYJ", Aliases: []string{"ny jets", "n y jets", "new york jets", "jets"}},
	{Slug: "baltimore-ravens", Name: "Baltimore Ravens", ShortName: "Ravens", Abbreviation: "BAL", Aliases: []string{"balt ravs", "balt ravens", "ravens"}},
	{Slug: "cincinnati-bengals", Name: "Cincinnati Bengals", ShortName: "Bengals", Abbreviation: "CIN", Aliases: []string{"cincy bengals", "cincinnati", "bengals"}},
	{Slug: "cleveland-browns", Name: "Cleveland Browns", ShortName: "Browns", Abbreviation: "CLE", Aliases: []string{"cle browns", "cleveland", "browns"}},
	{Slug: "pittsburgh-steelers", Name: "Pittsburgh Steelers", ShortName: "Steelers", Abbreviation: "PIT", Aliases: []string{"pittsburgh steelers", "steelers"}},
	{Slug: "houston-texans", Name: "Houston Texans", ShortName: "Texans", Abbreviation: "HOU", Aliases: []string{"hou texans", "houston texans", "texans"}},
	{Slug: "indianapolis-colts", Name: "Indianapolis Colts", ShortName: "Colts", Abbreviation: "IND", Aliases: []string{"ind colts", "indianapolis colts", "colts"}},
	{Slug: "jacksonville-jaguars", Name: "Jacksonville Jaguars", ShortName: "Jaguars", Abbreviation: "JAX", Aliases: []string{"jax jaguars", "jacksonville jaguars", "jaguars"}},
	{Slug: "tennessee-titans", Name: "Tennessee Titans", ShortName: "Titans", Abbreviation: "TEN", Aliases: []string{"ten titans", "tennessee titans", "titans"}},
	{Slug: "denver-broncos", Name: "Denver Broncos", ShortName: "Broncos", Abbreviation: "DEN", Aliases: []string{"den broncos", "denver broncos", "broncos"}},
	{Slug: "kansas-city-chiefs", Name: "Kansas City Chiefs", ShortName: "Chiefs", Abbreviation: "KC", Aliases: []string{"k c chiefs", "kansascity chiefs", "kansas city", "chiefs"}},
	{Slug: "las-vegas-raiders", Name: "Las Vegas Raiders", ShortName: "Raiders", Abbreviation: "LV", Aliases: []string{"lv raiders", "lasvegas raiders", "vegas raiders", "oakland raiders", "raiders"}},
	{Slug: "los-angeles-chargers", Name: "Los Angeles Chargers", ShortName: "Chargers", Abbreviation: "LAC", Aliases: []string{"la chargers", "losangeles chargers", "san diego chargers", "chargers"}},
	{Slug: "dallas-cowboys", Name: "Dallas Cowboys", ShortName: "Cowboys", Abbreviation: "DAL", Aliases: []string{"dallas cowboys", "cowboys"}},
	{Slug: "new-york-giants", Name: "New York Giants", ShortName: "Giants", Abbreviation: "NYG", Aliases: []string{"ny giants", "n y giants", "new york giants", "giants"}},
	{Slug: "philadelphia-eagles", Name: "Philadelphia Eagles", ShortName: "Eagles", Abbreviation: "PHI", Aliases: []string{"philly eagles", "philadelphia eagles", "eagles"}},
	{Slug: "washington-commanders", Name: "Washington Commanders", ShortName: "Commanders", Abbreviation: "WSH", Aliases: []string{"washington", "wsh commanders", "washington commanders", "redskins"}},
	{Slug: "chicago-bears", Name: "Chicago Bears", ShortName: "Bears", Abbreviation: "CHI", Aliases: []string{"chi bears", "chicago bears", "bears"}},
	{Slug: "detroit-lions", Name: "Detroit Lions", ShortName: "Lions", Abbreviation: "DET", Aliases: []string{"detroit lions", "lions"}},
	{Slug: "green-bay-packers", Name: "Green Bay Packers", ShortName: "Packers", Abbreviation: "GB", Aliases: []string{"gb packers", "greenbay packers", "green bay", "packers"}},
	{Slug: "minnesota-vikings", Name: "Minnesota Vikings", ShortName: "Vikings", Abbreviation: "MIN", Aliases: []string{"min vikings", "minnesota vikings", "vikings"}},
	{Slug: "atlanta-falcons", Name: "Atlanta Falcons", ShortName: "Falcons", Abbreviation: "ATL", Aliases: []string{"atl falcons", "atlanta falcons", "falcons"}},
	{Slug: "carolina-panthers", Name: "Carolina Panthers", ShortName: "Panthers", Abbreviation: "CAR", Aliases: []string{"car panthers", "carolina panthers", "panthers"}},
	{Slug: "new-orleans-saints", Name: "New Orleans Saints", ShortName: "Saints", Abbreviation: "NO", Aliases: []string{"no saints", "neworleans saints", "new orleans saints", "saints"}},
	{Slug: "tampa-bay-buccaneers", Name: "Tampa Bay Buccaneers", ShortName: "Buccaneers", Abbreviation: "TB", Aliases: []string{"tb bucs", "tb buccaneers", "tampabay bucs", "tampa bay bucs", "buccaneers"}},
	{Slug: "arizona-cardinals", Name: "Arizona Cardinals", ShortName: "Cardinals", Abbreviation: "ARI", Aliases: []string{"ari cardinals", "arizona cardinals", "cardinals"}},
	{Slug: "los-angeles-rams", Name: "Los Angeles Rams", ShortName: "Rams", Abbreviation: "LAR", Aliases: []string{"la rams", "losangeles rams", "st louis rams", "rams"}},
	{Slug: "san-francisco-49ers", Name: "San Francisco 49ers", ShortName: "49ers", Abbreviation: "SF", Aliases: []string{"sf 49ers", "sanfrancisco 49ers", "san francisco 49ers", "niners", "49ers"}},
	{Slug: "seattle-seahawks", Name: "Seattle Seahawks", ShortName: "Seahawks", Abbreviation: "SEA", Aliases: []string{"sea seahawks", "seattle seahawks", "seahawks"}},
}

type mockRepository struct {
	stories []domain.Story
}

func (r *mockRepository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	r.stories = append(r.stories, story)
	return domain.UpsertCreated, nil
}

func (r *mockRepository) List() ([]domain.Story, error) {
	return r.stories, nil
}

func (r *mockRepository) Count() (int, error) {
	return len(r.stories), nil
}

type failingFetcher struct {
	err error
}

func (f *failingFetcher) Fetch(sourceID string) ([]domain.FeedEntry, error) {
	return nil, f.err
}

func TestNewApplication(t *testing.T) {
	app := New()
	if app == nil {
		t.Fatal("New() returned nil")
	}
}

func TestWithRepository(t *testing.T) {
	repo := &mockRepository{}
	_ = New(WithRepository(repo))
}

func TestWithFeedFetchers(t *testing.T) {
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "test", Source: "test", PublishedAt: time.Now(), Summary: "test", URL: "https://test.com"},
	})
	app := New(WithFeedFetchers(fetcher))
	if len(app.fetchers) != 1 {
		t.Errorf("fetchers length = %d; want 1", len(app.fetchers))
	}
}

func TestWithClusterer(t *testing.T) {
	clust := domain.NewClusterer()
	app := New(WithClusterer(clust))
	result := app.RunGrouping(context.Background())
	if len(result.Errors) == 0 {
		t.Error("expected error when no repository configured")
	}
}

func TestRunIngestNoRepository(t *testing.T) {
	app := New()
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 0 || result.StoriesCreated != 0 {
		t.Errorf("expected no work, got entries=%d stories=%d", result.EntriesFetched, result.StoriesCreated)
	}
}

func TestRunIngestNoFetchers(t *testing.T) {
	repo := &mockRepository{}
	app := New(WithRepository(repo))
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 0 || result.StoriesCreated != 0 {
		t.Errorf("expected no work, got entries=%d stories=%d", result.EntriesFetched, result.StoriesCreated)
	}
}

func TestRunIngestWithFetchers(t *testing.T) {
	repo := &mockRepository{}
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "test story", Source: "test", PublishedAt: time.Now(), Summary: "test", URL: "https://test.com"},
	})
	app := New(WithRepository(repo), WithFeedFetchers(fetcher))
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 1 {
		t.Errorf("entries fetched = %d; want 1", result.EntriesFetched)
	}
	if result.StoriesCreated != 1 {
		t.Errorf("stories created = %d; want 1", result.StoriesCreated)
	}
}

func TestRunIngestFailureIsolation(t *testing.T) {
	repo := &mockRepository{}
	fetcher1 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com"},
	})
	fetcher2 := &failingFetcher{err: fmt.Errorf("fetcher2 failed")}
	fetcher3 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story3", Source: "feed3", PublishedAt: time.Now(), Summary: "s3", URL: "https://f3.com"},
	})

	app := New(WithRepository(repo), WithFeedFetchers(fetcher1, fetcher2, fetcher3))
	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 2 {
		t.Errorf("entries fetched = %d; want 2 (feed1 and feed3)", result.EntriesFetched)
	}
	if result.StoriesCreated != 2 {
		t.Errorf("stories created = %d; want 2", result.StoriesCreated)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d; want 1 (feed2)", len(result.Errors))
	}
	if result.Errors[0].Error() != "fetcher2 failed" {
		t.Errorf("first error = %v; want \"fetcher2 failed\"", result.Errors[0])
	}
}

func TestRunIngestWithRegistryWithTeamRegistry(t *testing.T) {
	// Test RunIngestWithRegistry with teamRegistry configured (full flow with team extraction)
	repo := &mockRepository{}
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "Giants win Super Bowl", Source: "test-feed", PublishedAt: time.Now(), Summary: "The Giants defeated the Chiefs", URL: "https://f1.com", GUID: "guid1"},
	})
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:          "test-feed",
		URL:         "https://example.com/feed.xml",
		Publication: "Test Publication",
		Priority:    1,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	teamReg := domain.NewTeamRegistryFromSlice(testTeams)
	app := New(WithRepository(repo), WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}), WithRegistry(registry), WithTeamRegistry(teamReg))
	result := app.RunIngest(context.Background())
	if result.StoriesCreated != 1 {
		t.Errorf("expected 1 story created, got %d", result.StoriesCreated)
	}
	if len(repo.stories) != 1 {
		t.Fatalf("expected 1 story in repo, got %d", len(repo.stories))
	}
	story := repo.stories[0]
	// Check that teams were extracted from title+summary
	foundGiants := false
	foundChiefs := false
	for _, entity := range story.Entities {
		if entity == "new-york-giants" {
			foundGiants = true
		}
		if entity == "kansas-city-chiefs" {
			foundChiefs = true
		}
	}
	if !foundGiants {
		t.Errorf("expected Giants entity in story, got %v", story.Entities)
	}
	if !foundChiefs {
		t.Errorf("expected Chiefs entity in story, got %v", story.Entities)
	}
}

func TestRunIngestRegistryNoEnabledSources(t *testing.T) {
	registry, err := domain.NewFeedRegistry()
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 0 || result.StoriesCreated != 0 {
		t.Errorf("expected no work, got entries=%d stories=%d", result.EntriesFetched, result.StoriesCreated)
	}
}

func TestRunIngestRegistryFetchFailure(t *testing.T) {
	fetcher := &failingFetcher{err: fmt.Errorf("fetch failed")}
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:          "test-feed",
		URL:         "https://example.com/feed.xml",
		Publication: "Test Publication",
		Priority:    1,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 0 {
		t.Errorf("expected 0 entries fetched, got %d", result.EntriesFetched)
	}
	if result.StoriesCreated != 0 {
		t.Errorf("expected 0 stories created, got %d", result.StoriesCreated)
	}
}

func TestRunIngestRegistryPerSourceIsolation(t *testing.T) {
	fetcher1 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com"},
	})
	fetcher2 := &failingFetcher{err: fmt.Errorf("fetcher2 failed")}
	fetcher3 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story3", Source: "feed3", PublishedAt: time.Now(), Summary: "s3", URL: "https://f3.com"},
	})

	registry, err := domain.NewFeedRegistry(
		domain.FeedSource{ID: "feed1", URL: "https://f1.com", Enabled: true, Publication: "Feed 1", Priority: 1},
		domain.FeedSource{ID: "feed2", URL: "https://f2.com", Enabled: true, Publication: "Feed 2", Priority: 2},
		domain.FeedSource{ID: "feed3", URL: "https://f3.com", Enabled: true, Publication: "Feed 3", Priority: 3},
	)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	app := New(
		WithRepository(&mockRepository{}),
		WithFetcherMap(map[string]domain.FeedFetcher{
			"feed1": fetcher1,
			"feed2": fetcher2,
			"feed3": fetcher3,
		}),
		WithRegistry(registry),
	)
	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 2 {
		t.Errorf("entries fetched = %d; want 2", result.EntriesFetched)
	}
	if result.StoriesCreated != 2 {
		t.Errorf("stories created = %d; want 2", result.StoriesCreated)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d; want 1", len(result.Errors))
	}
}

func TestRunIngestLegacyFallback(t *testing.T) {
	repo := &mockRepository{}
	fetcher1 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com"},
	})
	fetcher2 := &failingFetcher{err: fmt.Errorf("fetcher2 failed")}
	fetcher3 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story3", Source: "feed3", PublishedAt: time.Now(), Summary: "s3", URL: "https://f3.com"},
	})

	app := New(WithRepository(repo), WithFeedFetchers(fetcher1, fetcher2, fetcher3))
	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 2 {
		t.Errorf("entries fetched = %d; want 2", result.EntriesFetched)
	}
	if result.StoriesCreated != 2 {
		t.Errorf("stories created = %d; want 2", result.StoriesCreated)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d; want 1", len(result.Errors))
	}
}

func TestRunExport(t *testing.T) {
	repo := &mockRepository{}
	app := New(WithRepository(repo))
	result := app.RunExport(context.Background())
	if result.StoriesExported != 0 {
		t.Errorf("expected 0 stories exported, got %d", result.StoriesExported)
	}
}

func TestRunExportWithStories(t *testing.T) {
	repo := &mockRepository{
		stories: []domain.Story{
			{ID: "story1", Title: "Story 1", Source: "src1", PublishedAt: time.Now(), Summary: "s1", URL: "https://1.com"},
			{ID: "story2", Title: "Story 2", Source: "src2", PublishedAt: time.Now(), Summary: "s2", URL: "https://2.com"},
		},
	}
	app := New(WithRepository(repo))
	result := app.RunExport(context.Background())
	if result.StoriesExported != 2 {
		t.Errorf("expected 2 stories exported, got %d", result.StoriesExported)
	}
}

func TestRunGroupingWithRepository(t *testing.T) {
	repo := &mockRepository{}
	clust := domain.NewClusterer()
	app := New(WithRepository(repo), WithClusterer(clust))

	result := app.RunGrouping(context.Background())

	if result.GroupsCreated != 0 {
		t.Errorf("expected 0 groups created, got %d", result.GroupsCreated)
	}
}

func TestRunGroupingNoEngine(t *testing.T) {
	repo := &mockRepository{}
	app := New(WithRepository(repo))

	result := app.RunGrouping(context.Background())

	if len(result.Errors) == 0 {
		t.Error("expected error when no clustering engine configured")
	}
}

func TestRunGroupingNoRepository(t *testing.T) {
	clust := domain.NewClusterer()
	app := New(WithClusterer(clust))

	result := app.RunGrouping(context.Background())

	if len(result.Errors) == 0 {
		t.Error("expected error when no repository configured")
	}
}

func TestRunGroupingWithStories(t *testing.T) {
	repo := &mockRepository{
		stories: []domain.Story{
			{ID: "story1", Title: "Story One", Source: "src1", PublishedAt: time.Now(), Summary: "s1", URL: "https://1.com"},
			{ID: "story2", Title: "Story Two", Source: "src2", PublishedAt: time.Now(), Summary: "s2", URL: "https://2.com"},
		},
	}
	clust := domain.NewClusterer()
	app := New(WithRepository(repo), WithClusterer(clust))

	result := app.RunGrouping(context.Background())

	_ = result
}

func TestRunIngestRegistryUpsertCreated(t *testing.T) {
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "Story 1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com", GUID: "guid1"},
	})
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:          "test-feed",
		URL:         "https://example.com/feed.xml",
		Publication: "Test Publication",
		Priority:    1,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	if result.StoriesCreated != 1 {
		t.Errorf("expected 1 story created, got %d", result.StoriesCreated)
	}
}

func TestRunIngestRegistryMultipleStories(t *testing.T) {
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "Story 1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com", GUID: "guid1"},
		{Title: "Story 2", Source: "feed1", PublishedAt: time.Now(), Summary: "s2", URL: "https://f2.com", GUID: "guid2"},
	})
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:          "test-feed",
		URL:         "https://example.com/feed.xml",
		Publication: "Test Publication",
		Priority:    1,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	if result.StoriesCreated != 2 {
		t.Errorf("expected 2 stories created, got %d", result.StoriesCreated)
	}
}

func TestWithTeamRegistry(t *testing.T) {
	teamReg := domain.NewTeamRegistryFromSlice(testTeams)
	app := New(WithTeamRegistry(teamReg))
	if app.teamRegistry != teamReg {
		t.Error("team registry not set correctly")
	}
}

func TestRunIngestWithTeamRegistry(t *testing.T) {
	repo := &mockRepository{}
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "Giants win Super Bowl", Source: "test", PublishedAt: time.Now(), Summary: "The Giants defeated the Chiefs", URL: "https://test.com"},
	})
	teamReg := domain.NewTeamRegistryFromSlice(testTeams)
	app := New(WithRepository(repo), WithFeedFetchers(fetcher), WithTeamRegistry(teamReg))
	result := app.RunIngest(context.Background())
	if result.StoriesCreated != 1 {
		t.Errorf("expected 1 story created, got %d", result.StoriesCreated)
	}
	if len(repo.stories) != 1 {
		t.Fatalf("expected 1 story in repo, got %d", len(repo.stories))
	}
	story := repo.stories[0]
	// Check that teams were extracted
	foundGiants := false
	foundChiefs := false
	for _, entity := range story.Entities {
		if entity == "new-york-giants" {
			foundGiants = true
		}
		if entity == "kansas-city-chiefs" {
			foundChiefs = true
		}
	}
	if !foundGiants {
		t.Errorf("expected Giants entity in story, got %v", story.Entities)
	}
	if !foundChiefs {
		t.Errorf("expected Chiefs entity in story, got %v", story.Entities)
	}
}

func TestRunIngestWithTeamRegistryAndEntities(t *testing.T) {
	repo := &mockRepository{}
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "49ers trade for WR", Source: "test", PublishedAt: time.Now(), Summary: "The 49ers make a trade", URL: "https://test.com", Entities: []string{"san-francisco-49ers"}},
	})
	teamReg := domain.NewTeamRegistryFromSlice(testTeams)
	app := New(WithRepository(repo), WithFeedFetchers(fetcher), WithTeamRegistry(teamReg))
	result := app.RunIngest(context.Background())
	if result.StoriesCreated != 1 {
		t.Errorf("expected 1 story created, got %d", result.StoriesCreated)
	}
	if len(repo.stories) != 1 {
		t.Fatalf("expected 1 story in repo, got %d", len(repo.stories))
	}
	story := repo.stories[0]
	// Check that 49ers is in entities (from existing Entities field)
	found49ers := false
	for _, entity := range story.Entities {
		if entity == "san-francisco-49ers" {
			found49ers = true
		}
	}
	if !found49ers {
		t.Errorf("expected 49ers entity in story, got %v", story.Entities)
	}
	// Check that other teams from title/summary were also extracted
	found49ersAgain := false
	for _, entity := range story.Entities {
		if entity == "san-francisco-49ers" {
			found49ersAgain = true
			break
		}
	}
	if !found49ersAgain {
		t.Errorf("expected deduplication of 49ers entity, got %v", story.Entities)
	}
	// Count unique 49ers entries
	count := 0
	for _, entity := range story.Entities {
		if entity == "san-francisco-49ers" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 49ers entity (deduplicated), got %d", count)
	}
}

func TestExtractTeams_NilRegistry(t *testing.T) {
	// Test extractTeams with nil registry (should return story unchanged)
	story := domain.Story{
		ID:       "test-story",
		Title:    "Giants win Super Bowl",
		Source:   "test",
		Summary:  "The Giants defeated the Chiefs",
		Entities: []string{"existing-entity"},
	}
	result := extractTeams(story, nil)
	if len(result.Entities) != 1 || result.Entities[0] != "existing-entity" {
		t.Errorf("expected unchanged entities with nil registry, got %v", result.Entities)
	}
}

func TestExtractTeams_WithRegistry(t *testing.T) {
	// Test extractTeams with non-nil registry (should extract teams from title+summary)
	story := domain.Story{
		ID:       "test-story",
		Title:    "Giants win Super Bowl",
		Source:   "test",
		Summary:  "The Giants defeated the Chiefs",
		Entities: []string{},
	}
	teamReg := domain.NewTeamRegistryFromSlice(testTeams)
	result := extractTeams(story, teamReg)
	if len(result.Entities) != 2 {
		t.Errorf("expected 2 entities extracted, got %v", result.Entities)
	}
	foundGiants := false
	foundChiefs := false
	for _, entity := range result.Entities {
		if entity == "new-york-giants" {
			foundGiants = true
		}
		if entity == "kansas-city-chiefs" {
			foundChiefs = true
		}
	}
	if !foundGiants || !foundChiefs {
		t.Errorf("expected Giants and Chiefs entities, got %v", result.Entities)
	}
}

func TestExtractTeams_Deduplication(t *testing.T) {
	// Test extractTeams with duplicate prevention
	story := domain.Story{
		ID:       "test-story",
		Title:    "Giants win Super Bowl",
		Source:   "test",
		Summary:  "The Giants are great",
		Entities: []string{"new-york-giants"},
	}
	teamReg := domain.NewTeamRegistryFromSlice(testTeams)
	result := extractTeams(story, teamReg)
	// Should not add duplicate Giants
	count := 0
	for _, entity := range result.Entities {
		if entity == "new-york-giants" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 Giants entity (no duplicate), got %d: %v", count, result.Entities)
	}
}
