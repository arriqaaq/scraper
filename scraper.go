package scraper

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/arriqaaq/boomerang"
	"github.com/pkg/errors"
)

// TargetHealth describes the health state of a target.
type TargetHealth float64

// The possible health states of a target based on the last performed scrape.
const (
	HealthUnknown TargetHealth = -1
	HealthGood    TargetHealth = 1
	HealthBad     TargetHealth = 0
)

// A scraper retrieves samples and accepts a status report at the end.
type scraper interface {
	scrape(ctx context.Context) error
	report(start time.Time, dur time.Duration, err error)
	offset(interval time.Duration, jitterSeed uint64) time.Duration
	url() *url.URL
}

// Store provides appends against a storage.
type Store interface {
	// Add adds a target response for the given target.
	Add(url *url.URL, health TargetHealth, duration time.Duration) error
	// Commit commits the entries and clears the store. This should be called when all the entries are committed/reported.
	Commit() []TargetResponse
}

// targetScraper implements the scraper interface for a target.
type targetScraper struct {
	*Target

	client  *boomerang.HttpClient
	req     *http.Request
	timeout time.Duration
}

// URL returns the target's URL.
func (s *targetScraper) url() *url.URL {
	return s.URL()
}

func (s *targetScraper) scrape(ctx context.Context) error {
	if s.req == nil {
		req, err := http.NewRequest("GET", s.URL().String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Scrape-Timeout-Seconds", fmt.Sprintf("%f", s.timeout.Seconds()))

		s.req = req
	}

	resp, err := s.client.Do(s.req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("server returned HTTP status %s", resp.Status)
	}

	return nil
}

// NewStorage creates a storage for storing target responses.
func NewStorage(chSize int) Store {
	n := new(Storage)
	n.chSize = chSize
	n.rws = make([]TargetResponse, 0, chSize)
	return n
}

// Storage represents all the remote read and write endpoints
type Storage struct {
	mtx    sync.Mutex
	rws    []TargetResponse
	chSize int
}

// Add implements Store.
func (t *Storage) Add(url *url.URL, health TargetHealth, duration time.Duration) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	report := TargetResponse{
		URL:          url,
		Status:       health,
		ResponseTime: duration,
	}

	log.Println("report: ", report)
	t.rws = append(t.rws, report)

	return nil
}

// Commit implements Store.
func (t *Storage) Commit() []TargetResponse {
	t.mtx.Lock()
	resp := t.rws
	t.rws = make([]TargetResponse, 0, t.chSize)
	t.mtx.Unlock()
	return resp
}

// TargetResponse refers to the query response from the target
type TargetResponse struct {
	URL          *url.URL      `json:"url"`
	Status       TargetHealth  `json:"status"`
	ResponseTime time.Duration `json:"response_time"`
}

// Target refers to a singular HTTP or HTTPS endpoint.
type Target struct {
	lastError          error
	lastScrape         time.Time
	lastScrapeDuration time.Duration
	health             TargetHealth
	url                *url.URL
}

// NewTarget creates a target for querying.
func NewTarget(url *url.URL) *Target {
	return &Target{
		health: HealthUnknown,
		url:    url,
	}
}

// URL returns the target's URL.
func (t *Target) URL() *url.URL {
	return t.url
}

// hash returns an identifying hash for the target.
func (t *Target) hash() uint64 {
	h := fnv.New64a()
	//nolint: errcheck
	h.Write([]byte(t.URL().String()))

	return h.Sum64()
}

// offset returns the time until the next scrape cycle for the target.
func (t *Target) offset(interval time.Duration, jitterSeed uint64) time.Duration {
	now := time.Now().UnixNano()

	// Base is a pinned to absolute time, no matter how often offset is called.
	var (
		base   = int64(interval) - now%int64(interval)
		offset = (t.hash() ^ jitterSeed) % uint64(interval)
		next   = base + int64(offset)
	)

	if next > int64(interval) {
		next -= int64(interval)
	}
	return time.Duration(next)
}

// report sets target data about the last scrape.
func (t *Target) report(start time.Time, dur time.Duration, err error) {

	if err == nil {
		t.health = HealthGood
	} else {
		t.health = HealthBad
	}

	t.lastError = err
	t.lastScrape = start
	t.lastScrapeDuration = dur
}
