// Package arrapi is a client for the /api/v3 HTTP API shared by Sonarr and
// Radarr.
package arrapi

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/drzippit/arr-cli/internal/release"
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
	resp, err := c.do(ctx, http.MethodGet, path, query, nil)
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

// post sends body as JSON to path, expects 201 Created and decodes the
// answer into into. When the App rejects the body, the error gives the App's
// own reasons.
func (c *Client) post(ctx context.Context, path string, body, into any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPost, path, nil, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		if err := json.Unmarshal(respBody, into); err != nil {
			return fmt.Errorf("POST %s: decode response: %w", endpoint(path), err)
		}
		return nil
	case http.StatusBadRequest:
		if reasons := validationErrors(respBody); len(reasons) > 0 {
			return fmt.Errorf("POST %s: %s", endpoint(path), strings.Join(reasons, "; "))
		}
	}
	return &StatusError{Method: http.MethodPost, Path: path, Code: resp.StatusCode}
}

// do sends one authenticated request to path. A body is sent as JSON.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Response, error) {
	u := c.baseURL + endpoint(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "arr-cli/"+release.Current().Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpc.Do(req)
}

// validationErrors reads the reasons an App gives for rejecting a request
// body, if it gave any.
func validationErrors(body []byte) []string {
	var failures []struct {
		ErrorMessage string `json:"errorMessage"`
	}
	if json.Unmarshal(body, &failures) != nil {
		return nil
	}
	var reasons []string
	for _, f := range failures {
		if f.ErrorMessage != "" {
			reasons = append(reasons, f.ErrorMessage)
		}
	}
	return reasons
}

// StatusError means the Instance answered, but not with the status the
// request expects. Method is empty for GET.
type StatusError struct {
	Method string
	Path   string
	Code   int
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("%s %s: %d %s", cmp.Or(e.Method, http.MethodGet), endpoint(e.Path), e.Code, http.StatusText(e.Code))
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
	TitleSlug  string `json:"titleSlug"`
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
	TitleSlug  string           `json:"titleSlug"`
	Year       int              `json:"year"`
	Status     string           `json:"status"`
	Monitored  bool             `json:"monitored"`
	TvdbID     int              `json:"tvdbId"`
	Seasons    []Season         `json:"seasons"`
	Statistics SeriesStatistics `json:"statistics"`
}

// Season is one season of a Series. Season 0 holds the specials.
type Season struct {
	SeasonNumber int  `json:"seasonNumber"`
	Monitored    bool `json:"monitored"`
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

// LookupSeriesByTVDB asks a Sonarr Instance for the Candidate with TVDB ID
// id. Its ID is the Series' Library ID when the Series is in the Library. A
// TVDB ID the App does not know is an error.
func (c *Client) LookupSeriesByTVDB(ctx context.Context, id int) (Series, error) {
	series, err := c.LookupSeries(ctx, "tvdb:"+strconv.Itoa(id))
	if err != nil {
		return Series{}, err
	}
	for _, s := range series {
		if s.TvdbID == id {
			return s, nil
		}
	}
	return Series{}, fmt.Errorf("no Series with TVDB ID %d", id)
}

// LookupMovieByTMDB asks a Radarr Instance for the Candidate with TMDB ID id.
// Radarr builds it afresh from TMDB, so its ID is 0 even when the Movie is in
// the Library; LibraryMovieByTMDB tells. A TMDB ID the App does not know is
// an error.
func (c *Client) LookupMovieByTMDB(ctx context.Context, id int) (Movie, error) {
	var m Movie
	err := c.get(ctx, "movie/lookup/tmdb", url.Values{"tmdbId": {strconv.Itoa(id)}}, &m)
	var se *StatusError
	if (errors.As(err, &se) && se.Code == http.StatusNotFound) || (err == nil && m.TmdbID == 0) {
		return Movie{}, fmt.Errorf("no Movie with TMDB ID %d", id)
	}
	return m, err
}

// LibraryMovieByTMDB returns the Movie with TMDB ID id in a Radarr Instance's
// Library, and whether there is one.
func (c *Client) LibraryMovieByTMDB(ctx context.Context, id int) (Movie, bool, error) {
	var movies []Movie
	if err := c.get(ctx, "movie", url.Values{"tmdbId": {strconv.Itoa(id)}}, &movies); err != nil {
		return Movie{}, false, err
	}
	// Filter here too, in case the Instance ignores tmdbId and sends every Movie.
	for _, m := range movies {
		if m.TmdbID == id {
			return m, true, nil
		}
	}
	return Movie{}, false, nil
}

// QualityProfile is one of an Instance's quality profiles.
type QualityProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// QualityProfiles fetches the Instance's quality profiles.
func (c *Client) QualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	var ps []QualityProfile
	err := c.get(ctx, "qualityprofile", nil, &ps)
	return ps, err
}

// RootFolder is one of the folders an Instance keeps its Titles in.
type RootFolder struct {
	Path string `json:"path"`
}

// RootFolders fetches the Instance's root folders.
func (c *Client) RootFolders(ctx context.Context) ([]RootFolder, error) {
	var fs []RootFolder
	err := c.get(ctx, "rootfolder", nil, &fs)
	return fs, err
}

// MinimumAvailabilities are the values Radarr takes for when a Movie counts
// as available to download.
var MinimumAvailabilities = []string{"announced", "inCinemas", "released"}

// NewMovie is a Candidate to add to a Radarr Instance's Library, with where
// and how to keep it. MinimumAvailability is one of MinimumAvailabilities.
// Search starts a search for the Movie once it is added.
type NewMovie struct {
	Candidate           Movie
	QualityProfileID    int
	RootFolderPath      string
	MinimumAvailability string
	Search              bool
}

// AddMovie adds a Movie to a Radarr Instance's Library, monitored, so that
// it is downloaded once it is available, and returns it as it is now in the
// Library.
func (c *Client) AddMovie(ctx context.Context, m NewMovie) (Movie, error) {
	type addOptions struct {
		Monitor        string `json:"monitor"`
		SearchForMovie bool   `json:"searchForMovie"`
	}
	var added Movie
	err := c.post(ctx, "movie", struct {
		Title               string     `json:"title"`
		TitleSlug           string     `json:"titleSlug,omitempty"`
		Year                int        `json:"year"`
		TmdbID              int        `json:"tmdbId"`
		QualityProfileID    int        `json:"qualityProfileId"`
		RootFolderPath      string     `json:"rootFolderPath"`
		Monitored           bool       `json:"monitored"`
		MinimumAvailability string     `json:"minimumAvailability"`
		AddOptions          addOptions `json:"addOptions"`
	}{
		Title:               m.Candidate.Title,
		TitleSlug:           m.Candidate.TitleSlug,
		Year:                m.Candidate.Year,
		TmdbID:              m.Candidate.TmdbID,
		QualityProfileID:    m.QualityProfileID,
		RootFolderPath:      m.RootFolderPath,
		Monitored:           true,
		MinimumAvailability: m.MinimumAvailability,
		AddOptions:          addOptions{Monitor: "movieOnly", SearchForMovie: m.Search},
	}, &added)
	return added, err
}

// MonitorOptions are the values Sonarr takes for which episodes of a Series
// to monitor when it is added.
var MonitorOptions = []string{"all", "future", "missing", "existing", "firstSeason", "latestSeason", "none"}

// NewSeries is a Candidate to add to a Sonarr Instance's Library, with where
// and how to keep it. Monitor is one of MonitorOptions. Search starts a search
// for the monitored episodes missing once it is added.
type NewSeries struct {
	Candidate        Series
	QualityProfileID int
	RootFolderPath   string
	Monitor          string
	Search           bool
}

// AddSeries adds a Series to a Sonarr Instance's Library, monitored and kept
// in season folders, and returns it as it is now in the Library. Which
// episodes are monitored follows s.Monitor.
func (c *Client) AddSeries(ctx context.Context, s NewSeries) (Series, error) {
	type addOptions struct {
		Monitor                  string `json:"monitor"`
		SearchForMissingEpisodes bool   `json:"searchForMissingEpisodes"`
	}
	seasons := s.Candidate.Seasons
	if seasons == nil {
		seasons = []Season{}
	}
	var added Series
	err := c.post(ctx, "series", struct {
		Title            string     `json:"title"`
		TitleSlug        string     `json:"titleSlug,omitempty"`
		Year             int        `json:"year"`
		TvdbID           int        `json:"tvdbId"`
		QualityProfileID int        `json:"qualityProfileId"`
		RootFolderPath   string     `json:"rootFolderPath"`
		Monitored        bool       `json:"monitored"`
		SeasonFolder     bool       `json:"seasonFolder"`
		Seasons          []Season   `json:"seasons"`
		AddOptions       addOptions `json:"addOptions"`
	}{
		Title:            s.Candidate.Title,
		TitleSlug:        s.Candidate.TitleSlug,
		Year:             s.Candidate.Year,
		TvdbID:           s.Candidate.TvdbID,
		QualityProfileID: s.QualityProfileID,
		RootFolderPath:   s.RootFolderPath,
		Monitored:        true,
		SeasonFolder:     true,
		Seasons:          seasons,
		AddOptions:       addOptions{Monitor: s.Monitor, SearchForMissingEpisodes: s.Search},
	}, &added)
	return added, err
}
