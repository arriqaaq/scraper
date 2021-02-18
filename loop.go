package scraper

import (
	"context"
	"log"
	"time"
)

// A loop can run and be stopped again.
type loop interface {
	run(interval, timeout time.Duration, errc chan<- error)
	stop()
}

type scrapeLoopOptions struct {
	target  *Target
	scraper scraper
}

func newScrapeLoop(
	ctx context.Context,
	sc scraper,
	store func(ctx context.Context) Store,
	jitterSeed uint64,
) *scrapeLoop {
	sl := &scrapeLoop{
		ctx:        ctx,
		scraper:    sc,
		store:      store,
		stopped:    make(chan struct{}),
		jitterSeed: jitterSeed,
	}
	sl.ctx, sl.cancel = context.WithCancel(ctx)

	return sl
}

type scrapeLoop struct {
	scraper    scraper
	jitterSeed uint64

	store func(ctx context.Context) Store

	exporter *Exporter

	ctx     context.Context
	cancel  func()
	stopped chan struct{}
}

func (sl *scrapeLoop) run(interval, timeout time.Duration, errc chan<- error) {
	select {
	case <-time.After(sl.scraper.offset(interval, sl.jitterSeed)):
		// Continue after a scraping offset.
	case <-sl.ctx.Done():
		close(sl.stopped)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-sl.ctx.Done():
			close(sl.stopped)
			return
		default:
		}

		scrapeTime := time.Now()

		sl.scrapeAndReport(interval, timeout, scrapeTime, errc)

		select {
		case <-sl.ctx.Done():
			close(sl.stopped)
			return
		case <-ticker.C:
		}
	}

	close(sl.stopped)

}

// scrapeAndReport performs a scrape and then appends the result to the store
// together with reporting metrics.
func (sl *scrapeLoop) scrapeAndReport(interval, timeout time.Duration, appendTime time.Time, errc chan<- error) time.Time {
	start := time.Now()

	var scrapeErr error

	app := sl.store(sl.ctx)

	defer func() {
		sl.scraper.report(start, time.Since(start), scrapeErr)
	}()

	var health TargetHealth
	scrapeCtx, cancel := context.WithTimeout(sl.ctx, timeout)
	scrapeErr = sl.scraper.scrape(scrapeCtx)
	cancel()

	if scrapeErr != nil {
		health = HealthBad
		log.Println("msg", "Scrape failed", "err", scrapeErr)
		if errc != nil {
			errc <- scrapeErr
		}
	} else {
		health = HealthGood
	}

	// appending the stats to the store to make it available to the exporter.
	app.Add(sl.scraper.url(), health, time.Since(start))

	return start
}

// Stop the scraping.
func (sl *scrapeLoop) stop() {
	sl.cancel()
	<-sl.stopped
}
