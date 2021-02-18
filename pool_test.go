package scraper

import (
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/arriqaaq/boomerang"
	"github.com/stretchr/testify/require"
)

type testLoop struct {
	startFunc func(interval, timeout time.Duration, errc chan<- error)
	stopFunc  func()
	runOnce   bool
}

func (l *testLoop) run(interval, timeout time.Duration, errc chan<- error) {
	if l.runOnce {
		panic("loop must be started only once")
	}
	l.runOnce = true
	l.startFunc(interval, timeout, errc)
}

func (l *testLoop) stop() {
	l.stopFunc()
}

func TestScrapePoolScrapeLoopsStarted(t *testing.T) {
	var wg sync.WaitGroup
	newLoop := func(opts scrapeLoopOptions) loop {
		wg.Add(1)
		l := &testLoop{
			startFunc: func(interval, timeout time.Duration, errc chan<- error) {
				wg.Done()
			},
			stopFunc: func() {},
		}
		return l
	}
	sp, err := NewScrapePool(&ScrapeConfig{
		ScrapeInterval: time.Duration(3 * time.Second),
		ScrapeTimeout:  time.Duration(2 * time.Second),
	})
	require.NoError(t, err)

	sp.client = boomerang.NewHttpClient(&boomerang.ClientConfig{
		Transport:  boomerang.DefaultTransport(),
		MaxRetries: 1,
	})
	sp.newLoop = newLoop

	serverURL1, _ := url.Parse("http://foo.com")
	serverURL2, _ := url.Parse("http://bar.com")

	tgs := []*Target{
		{
			url: serverURL1,
		},
		{
			url: serverURL2,
		},
	}

	sp.Start(tgs)

	require.Equal(t, 2, len(sp.loops))

	wg.Wait()
	for _, l := range sp.loops {
		require.True(t, l.(*testLoop).runOnce, "loop should be running")
	}
}
