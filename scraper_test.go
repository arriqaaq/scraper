package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/arriqaaq/boomerang"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestTargetScraperScrapeOK(t *testing.T) {
	const (
		configTimeout   = 1500 * time.Millisecond
		expectedTimeout = "1.500000"
	)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			timeout := r.Header.Get("X-Scrape-Timeout-Seconds")
			if timeout != expectedTimeout {
				t.Errorf("Expected scrape timeout header %q, got %q", expectedTimeout, timeout)
			}
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: NewTarget(serverURL),
		client: boomerang.NewHttpClient(&boomerang.ClientConfig{
			Transport:  boomerang.DefaultTransport(),
			MaxRetries: 1,
		}),
		timeout: configTimeout,
	}

	err = ts.scrape(context.Background())
	require.NoError(t, err)
}

func TestTargetScrapeScrapeCancel(t *testing.T) {
	block := make(chan struct{})

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-block
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: NewTarget(serverURL),
		client: boomerang.NewHttpClient(&boomerang.ClientConfig{
			Transport:  boomerang.DefaultTransport(),
			MaxRetries: 1,
		}),
	}
	ctx, cancel := context.WithCancel(context.Background())

	errc := make(chan error, 1)

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	go func() {
		err := ts.scrape(ctx)
		if err == nil {
			errc <- errors.New("Expected error but got nil")
		} else if ctx.Err() != context.Canceled {
			errc <- errors.Errorf("Expected context cancellation error but got: %s", ctx.Err())
		} else {
			close(errc)
		}
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape function did not return unexpectedly")
	case err := <-errc:
		require.NoError(t, err)
	}
	close(block)
}

func TestTargetScrapeScrapeNotFound(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: NewTarget(serverURL),
		client: boomerang.NewHttpClient(&boomerang.ClientConfig{
			Transport:  boomerang.DefaultTransport(),
			MaxRetries: 1,
		}),
	}

	err = ts.scrape(context.Background())
	require.Contains(t, err.Error(), "404", "Expected \"404 NotFound\" error but got: %s", err)
}

func TestStorageAdd(t *testing.T) {
	s := NewStorage(2)

	serverURL, err := url.Parse("http://foobar.com")
	if err != nil {
		panic(err)
	}

	err = s.Add(serverURL, HealthGood, time.Duration(1*time.Second))
	require.NoError(t, err)

	resp := s.Commit()[0]
	require.Contains(t, resp.URL.String(), "http://foobar.com")
}
