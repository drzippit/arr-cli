// Package arrapi is a client for the /api/v3 HTTP API shared by Sonarr and
// Radarr.
package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to one Instance.
type Client struct {
	baseURL string
	apiKey  string
	httpc   *http.Client
}

// New returns a Client for the Instance at baseURL.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
}

// SystemStatus is the subset of /api/v3/system/status arr-cli uses.
type SystemStatus struct {
	AppName string `json:"appName"`
	Version string `json:"version"`
}

// HealthCheck is one entry from /api/v3/health.
type HealthCheck struct {
	Source  string `json:"source"`
	Type    string `json:"type"`
	Message string `json:"message"`
	WikiURL string `json:"wikiUrl"`
}

// SystemStatus fetches the Instance's App name and version.
func (c *Client) SystemStatus(ctx context.Context) (SystemStatus, error) {
	var s SystemStatus
	err := c.get(ctx, "system/status", nil, &s)
	return s, err
}

// Health fetches the Instance's current health checks.
func (c *Client) Health(ctx context.Context) ([]HealthCheck, error) {
	var h []HealthCheck
	err := c.get(ctx, "health", nil, &h)
	return h, err
}

// QueueItem is one download from /api/v3/queue. Series and Episode are set by
// Sonarr, Movie by Radarr. Release is the name of the release being
// downloaded, which the API calls its title.
type QueueItem struct {
	Release                 string         `json:"title"`
	Series                  *SeriesSummary `json:"series"`
	Episode                 *QueueEpisode  `json:"episode"`
	Movie                   *QueueMovie    `json:"movie"`
	Size                    float64        `json:"size"`
	SizeLeft                float64        `json:"sizeleft"`
	TimeLeft                string         `json:"timeleft"`
	EstimatedCompletionTime string         `json:"estimatedCompletionTime"`
	Status                  string         `json:"status"`
	TrackedDownloadState    string         `json:"trackedDownloadState"`
	Protocol                string         `json:"protocol"`
	DownloadClient          string         `json:"downloadClient"`
}

// SeriesSummary is the Series a Sonarr queue or calendar item belongs to.
type SeriesSummary struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
}

// QueueEpisode is the episode a Sonarr queue item downloads.
type QueueEpisode struct {
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Title         string `json:"title"`
}

// QueueMovie is the Movie a Radarr queue item downloads.
type QueueMovie struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
}

type queuePage struct {
	TotalRecords int         `json:"totalRecords"`
	Records      []QueueItem `json:"records"`
}

// queuePageSize is how many queue items Queue asks for per request.
const queuePageSize = 1000

// Queue fetches every download in the Instance's queue, following pages. It
// asks both Sonarr and Radarr to embed their Titles and to include downloads
// they cannot match to a Title; each App ignores the parameters it does not
// know.
func (c *Client) Queue(ctx context.Context) ([]QueueItem, error) {
	var items []QueueItem
	for page := 1; ; page++ {
		var p queuePage
		q := url.Values{
			"page":           {strconv.Itoa(page)},
			"pageSize":       {strconv.Itoa(queuePageSize)},
			"includeSeries":  {"true"},
			"includeEpisode": {"true"},
			"includeMovie":   {"true"},
			// Downloads the App cannot match to a Title are still in progress.
			"includeUnknownSeriesItems": {"true"},
			"includeUnknownMovieItems":  {"true"},
		}
		if err := c.get(ctx, "queue", q, &p); err != nil {
			return nil, err
		}
		items = append(items, p.Records...)
		if len(p.Records) == 0 || len(items) >= p.TotalRecords {
			return items, nil
		}
	}
}

func (c *Client) get(ctx context.Context, path string, query url.Values, into any) error {
	u := c.baseURL + endpoint(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return &StatusError{Path: path, Code: resp.StatusCode}
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("GET %s: decode response: %w", endpoint(path), err)
	}
	return nil
}

// StatusError means the Instance answered, but not with 200 OK.
type StatusError struct {
	Path string
	Code int
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("GET %s: %d %s", endpoint(e.Path), e.Code, http.StatusText(e.Code))
	if e.Code == http.StatusUnauthorized {
		msg += " (check the API key)"
	}
	return msg
}

func endpoint(path string) string { return "/api/v3/" + path }

// CalendarEpisode is one episode from Sonarr's /api/v3/calendar.
type CalendarEpisode struct {
	SeasonNumber  int            `json:"seasonNumber"`
	EpisodeNumber int            `json:"episodeNumber"`
	Title         string         `json:"title"`
	AirDateUTC    time.Time      `json:"airDateUtc"`
	HasFile       bool           `json:"hasFile"`
	Monitored     bool           `json:"monitored"`
	Series        *SeriesSummary `json:"series"`
}

// CalendarMovie is one Movie from Radarr's /api/v3/calendar. Each release
// date is a calendar day, given as midnight UTC, and is nil when unknown.
type CalendarMovie struct {
	Title           string     `json:"title"`
	Year            int        `json:"year"`
	InCinemas       *time.Time `json:"inCinemas"`
	DigitalRelease  *time.Time `json:"digitalRelease"`
	PhysicalRelease *time.Time `json:"physicalRelease"`
	HasFile         bool       `json:"hasFile"`
	Monitored       bool       `json:"monitored"`
}

// EpisodeCalendar fetches the monitored episodes a Sonarr Instance expects to
// air between start and end, and the unmonitored ones too if unmonitored is
// set.
func (c *Client) EpisodeCalendar(ctx context.Context, start, end time.Time, unmonitored bool) ([]CalendarEpisode, error) {
	var eps []CalendarEpisode
	q := calendarQuery(start, end, unmonitored)
	q.Set("includeSeries", "true")
	err := c.get(ctx, "calendar", q, &eps)
	return eps, err
}

// MovieCalendar fetches the monitored Movies a Radarr Instance expects to
// release between start and end, and the unmonitored ones too if unmonitored
// is set.
func (c *Client) MovieCalendar(ctx context.Context, start, end time.Time, unmonitored bool) ([]CalendarMovie, error) {
	var movies []CalendarMovie
	err := c.get(ctx, "calendar", calendarQuery(start, end, unmonitored), &movies)
	return movies, err
}

func calendarQuery(start, end time.Time, unmonitored bool) url.Values {
	return url.Values{
		"start":       {start.UTC().Format(time.RFC3339)},
		"end":         {end.UTC().Format(time.RFC3339)},
		"unmonitored": {strconv.FormatBool(unmonitored)},
	}
}

// Movie is one Movie from Radarr's /api/v3/movie or /api/v3/movie/lookup. ID
// is the Movie's ID in the Instance's Library, or 0 for a Candidate not in it.
type Movie struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	SortTitle  string `json:"sortTitle"`
	Year       int    `json:"year"`
	Status     string `json:"status"`
	Monitored  bool   `json:"monitored"`
	HasFile    bool   `json:"hasFile"`
	SizeOnDisk int64  `json:"sizeOnDisk"`
	TmdbID     int    `json:"tmdbId"`
}

// Movies fetches every Movie in a Radarr Instance's Library.
func (c *Client) Movies(ctx context.Context) ([]Movie, error) {
	var movies []Movie
	err := c.get(ctx, "movie", nil, &movies)
	return movies, err
}

// Series is one Series from Sonarr's /api/v3/series or /api/v3/series/lookup.
// ID is the Series' ID in the Instance's Library, or 0 for a Candidate not in
// it.
type Series struct {
	ID         int              `json:"id"`
	Title      string           `json:"title"`
	SortTitle  string           `json:"sortTitle"`
	Year       int              `json:"year"`
	Status     string           `json:"status"`
	Monitored  bool             `json:"monitored"`
	TvdbID     int              `json:"tvdbId"`
	Statistics SeriesStatistics `json:"statistics"`
}

// SeriesStatistics counts a Series' seasons and the episodes on disk.
// EpisodeCount counts only the monitored or downloaded episodes.
type SeriesStatistics struct {
	SeasonCount      int   `json:"seasonCount"`
	EpisodeFileCount int   `json:"episodeFileCount"`
	EpisodeCount     int   `json:"episodeCount"`
	SizeOnDisk       int64 `json:"sizeOnDisk"`
}

// AllSeries fetches every Series in a Sonarr Instance's Library.
func (c *Client) AllSeries(ctx context.Context) ([]Series, error) {
	var series []Series
	err := c.get(ctx, "series", nil, &series)
	return series, err
}

// LookupMovies asks a Radarr Instance for the Candidates matching term.
func (c *Client) LookupMovies(ctx context.Context, term string) ([]Movie, error) {
	var movies []Movie
	err := c.get(ctx, "movie/lookup", url.Values{"term": {term}}, &movies)
	return movies, err
}

// LookupSeries asks a Sonarr Instance for the Candidates matching term.
func (c *Client) LookupSeries(ctx context.Context, term string) ([]Series, error) {
	var series []Series
	err := c.get(ctx, "series/lookup", url.Values{"term": {term}}, &series)
	return series, err
}
