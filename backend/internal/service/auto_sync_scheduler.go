package service

import (
	"log"
	"sync"
	"time"
)

type AutoSyncScheduler struct {
	name          string
	settings      *SettingsService
	isRelevantKey func(key string) bool
	isConfigured  func() bool
	getConfig     func() (bool, time.Duration)
	runSync       func() error
	retryInterval time.Duration
	resetCh       chan struct{}
	stopCh        chan struct{}
	startOnce     sync.Once
	stopOnce      sync.Once
	runMu         sync.Mutex
	stateMu       sync.Mutex
	lastSuccessAt time.Time
	retryAfter    time.Time
	lastError     string
	running       int
	// warnedUnconfigured keeps the "enabled but not configured" warning to one
	// line per episode instead of one per timer tick.
	warnedUnconfigured bool
}

// warnUnconfigured logs that an enabled auto-sync has nothing it can sync,
// once per episode — it re-arms via noteConfigured once the user fixes it.
// Both places that skip a run call this, so the silent 24h fallback that hid
// issue #54 for a release always leaves a trace in the log.
func (s *AutoSyncScheduler) warnUnconfigured() {
	s.stateMu.Lock()
	already := s.warnedUnconfigured
	s.warnedUnconfigured = true
	s.stateMu.Unlock()
	if !already {
		log.Printf("%s auto-sync is enabled but not configured "+
			"(no album selected?); skipping until that changes", s.name)
	}
}

// noteConfigured re-arms the warning so a later misconfiguration is reported.
func (s *AutoSyncScheduler) noteConfigured() {
	s.stateMu.Lock()
	s.warnedUnconfigured = false
	s.stateMu.Unlock()
}

// LastError returns the failure message of the most recent completed sync run,
// or "" if it succeeded (or none has run yet). Lets the sync-status endpoint
// surface failures the dashboard would otherwise never see (issue #44).
func (s *AutoSyncScheduler) LastError() string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.lastError
}

// IsRunning reports whether any sync is currently in flight (queued or
// executing). running is a counter so overlapping triggers each track their own
// in-flight sync; the indicator only clears once every sync has finished.
func (s *AutoSyncScheduler) IsRunning() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.running > 0
}

func (s *AutoSyncScheduler) incRunning() {
	s.stateMu.Lock()
	s.running++
	s.stateMu.Unlock()
}

func (s *AutoSyncScheduler) decRunning() {
	s.stateMu.Lock()
	s.running--
	s.stateMu.Unlock()
}

func NewAutoSyncScheduler(opts AutoSyncSchedulerOptions) *AutoSyncScheduler {
	retry := opts.RetryInterval
	if retry <= 0 {
		retry = 5 * time.Minute
	}
	return &AutoSyncScheduler{
		name:          opts.Name,
		settings:      opts.Settings,
		isRelevantKey: opts.IsRelevantKey,
		isConfigured:  opts.IsConfigured,
		getConfig:     opts.GetConfig,
		runSync:       opts.RunSync,
		retryInterval: retry,
		resetCh:       make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
	}
}

type AutoSyncSchedulerOptions struct {
	Name          string
	Settings      *SettingsService
	IsRelevantKey func(key string) bool
	IsConfigured  func() bool
	GetConfig     func() (bool, time.Duration)
	RunSync       func() error
	RetryInterval time.Duration
}

func (s *AutoSyncScheduler) Start() {
	s.startOnce.Do(func() {
		// Sources configured through settings keys re-arm on a key change;
		// connection-backed sources are re-armed by their manager instead.
		if s.settings != nil && s.isRelevantKey != nil {
			s.settings.RegisterOnChange(func(key, _ string) {
				if s.isRelevantKey(key) {
					s.TriggerReset()
				}
			})
		}
		go s.loop()
	})
}

// Stop ends the loop for good; used when a connection is deleted. A sync in
// flight finishes, nothing new is scheduled. Safe to call more than once.
func (s *AutoSyncScheduler) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

func (s *AutoSyncScheduler) TriggerReset() {
	select {
	case s.resetCh <- struct{}{}:
	default:
	}
}

func (s *AutoSyncScheduler) SyncNow() error {
	s.incRunning()
	defer s.decRunning()
	return s.runSyncNow()
}

// runSyncNow performs a single sync run without touching the in-flight counter.
// Callers are responsible for incrementing/decrementing running exactly once
// around their logical sync so it isn't double-counted.
func (s *AutoSyncScheduler) runSyncNow() error {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	if err := s.runSync(); err != nil {
		s.stateMu.Lock()
		s.retryAfter = time.Now().Add(s.retryInterval)
		s.lastError = err.Error()
		s.stateMu.Unlock()
		return err
	}

	s.stateMu.Lock()
	s.lastSuccessAt = time.Now()
	s.retryAfter = time.Time{}
	s.lastError = ""
	s.stateMu.Unlock()
	s.TriggerReset()
	return nil
}

// SyncNowAsync runs a sync in the background and returns immediately. Runs are
// serialized by SyncNow's mutex; errors are logged (callers that need to block
// on the result should use SyncNow instead). Used by endpoints that shouldn't
// hold the HTTP request open for a full clear+resync.
func (s *AutoSyncScheduler) SyncNowAsync() {
	// Mark running synchronously so the UI can observe an in-flight sync right
	// away, even before the goroutine acquires the run lock and starts. The
	// goroutine calls runSyncNow (which does not touch the counter) so this
	// logical sync is counted exactly once.
	s.incRunning()
	go func() {
		defer s.decRunning()
		if err := s.runSyncNow(); err != nil {
			log.Printf("[%s] async sync failed: %v", s.name, err)
		}
	}()
}

func (s *AutoSyncScheduler) loop() {
	timer := time.NewTimer(s.nextDelay())
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			s.tryRunDue()
		case <-s.resetCh:
		case <-s.stopCh:
			return
		}

		delay := s.nextDelay()
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(delay)
	}
}

func (s *AutoSyncScheduler) tryRunDue() {
	enabled, _ := s.getConfig()
	if !enabled {
		return
	}
	if !s.isConfigured() {
		s.warnUnconfigured()
		return
	}
	s.noteConfigured()

	if err := s.SyncNow(); err != nil {
		log.Printf("%s auto-sync failed: %v", s.name, err)
		return
	}

	log.Printf("%s auto-sync completed", s.name)
}

func (s *AutoSyncScheduler) nextDelay() time.Duration {
	enabled, interval := s.getConfig()
	if !enabled {
		return 24 * time.Hour
	}
	if !s.isConfigured() {
		// Reached at startup and on every reset, so this is where a
		// misconfigured source actually gets reported; tryRunDue would only
		// notice once the 24h fallback timer finally expired.
		s.warnUnconfigured()
		return 24 * time.Hour
	}
	s.noteConfigured()

	now := time.Now()
	s.stateMu.Lock()
	lastSuccessAt := s.lastSuccessAt
	retryAfter := s.retryAfter
	s.stateMu.Unlock()

	if !retryAfter.IsZero() && now.Before(retryAfter) {
		return time.Until(retryAfter)
	}

	if lastSuccessAt.IsZero() {
		return 0
	}

	nextRun := lastSuccessAt.Add(interval)
	if !now.Before(nextRun) {
		return 0
	}

	return time.Until(nextRun)
}
