package scraper

import (
	"context"
	"sync"
	"time"

	"github.com/arriqaaq/boomerang"
)

// ScrapeConfig describes the config for the scraper pool.
type ScrapeConfig struct {
	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval time.Duration
	// The timeout for scraping targets of this config.
	ScrapeTimeout time.Duration
	// The channel size for the storage.
	StoreSize int
	// Jitter seed
	JitterSeed uint64
}

func NewScrapePool(
	cfg *ScrapeConfig,
) (*ScrapePool, error) {

	client := boomerang.NewHttpClient(&boomerang.ClientConfig{
		Transport:  boomerang.DefaultTransport(),
		Timeout:    cfg.ScrapeTimeout,
		MaxRetries: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	sp := &ScrapePool{
		ctx:    ctx,
		cancel: cancel,
		config: cfg,
		client: client,
		loops:  map[uint64]loop{},
		quitCh: make(chan struct{}, 1),
	}

	// store is a common storage to which multiple scrapers will push
	sp.store = NewStorage(sp.config.StoreSize)

	// Setup prometheus metrics exporter
	sp.Exporter = NewExporter(NewMetrics(), sp.config.StoreSize)

	sp.newLoop = func(opts scrapeLoopOptions) loop {

		return newScrapeLoop(
			ctx,
			opts.scraper,
			func(ctx context.Context) Store { return sp.store },
			sp.config.JitterSeed,
		)
	}
	return sp, nil
}

// ScrapePool manages scrapes for sets of targets.
type ScrapePool struct {
	mtx    sync.Mutex
	ctx    context.Context
	client *boomerang.HttpClient
	loops  map[uint64]loop
	config *ScrapeConfig
	cancel context.CancelFunc
	store  Store

	*Exporter

	quitCh chan struct{}

	// Constructor for new scrape loops.
	newLoop func(scrapeLoopOptions) loop
}

// Stop terminates all scrape loops and returns after they all terminated.
func (sp *ScrapePool) Stop() {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()
	sp.cancel()
	close(sp.quitCh)

	var wg sync.WaitGroup

	for fp, l := range sp.loops {
		wg.Add(1)

		go func(l loop) {
			l.stop()
			wg.Done()
		}(l)

		delete(sp.loops, fp)
	}

	wg.Wait()
}

// Start starts scrape loops for new targets.
func (sp *ScrapePool) Start(targets []*Target) {

	var (
		interval = time.Duration(sp.config.ScrapeInterval)
		timeout  = time.Duration(sp.config.ScrapeTimeout)
	)

	sp.mtx.Lock()

	for _, t := range targets {
		hash := t.hash()
		ts := &targetScraper{Target: t, client: sp.client, timeout: timeout}
		newLoop := sp.newLoop(scrapeLoopOptions{
			target:  t,
			scraper: ts,
		})
		sp.loops[hash] = newLoop

	}

	sp.mtx.Unlock()

	for _, l := range sp.loops {
		if l != nil {
			go l.run(interval, timeout, nil)
		}
	}

	ticker := time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-ticker.C:
				entries := sp.store.Commit()
				sp.Exporter.entries = entries
				break
			case <-sp.quitCh:
				return
			}
		}
	}()
}
