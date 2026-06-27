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
	startOnce     sync.Once
	runMu         sync.Mutex
	stateMu       sync.Mutex
	lastSuccessAt time.Time
	retryAfter    time.Time
	running       bool
}

// IsRunning reports whether a sync is currently executing.
func (s *AutoSyncScheduler) IsRunning() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.running
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
		s.settings.RegisterOnChange(func(key, _ string) {
			if s.isRelevantKey(key) {
				s.TriggerReset()
			}
		})
		go s.loop()
	})
}

func (s *AutoSyncScheduler) TriggerReset() {
	select {
	case s.resetCh <- struct{}{}:
	default:
	}
}

func (s *AutoSyncScheduler) SyncNow() error {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	s.stateMu.Lock()
	s.running = true
	s.stateMu.Unlock()
	defer func() {
		s.stateMu.Lock()
		s.running = false
		s.stateMu.Unlock()
	}()

	if err := s.runSync(); err != nil {
		s.stateMu.Lock()
		s.retryAfter = time.Now().Add(s.retryInterval)
		s.stateMu.Unlock()
		return err
	}

	s.stateMu.Lock()
	s.lastSuccessAt = time.Now()
	s.retryAfter = time.Time{}
	s.stateMu.Unlock()
	s.TriggerReset()
	return nil
}

// SyncNowAsync runs a sync in the background and returns immediately. Runs are
// serialized by SyncNow's mutex; errors are logged (callers that need to block
// on the result should use SyncNow instead). Used by endpoints that shouldn't
// hold the HTTP request open for a full clear+resync.
func (s *AutoSyncScheduler) SyncNowAsync() {
	go func() {
		if err := s.SyncNow(); err != nil {
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
	if !enabled || !s.isConfigured() {
		return
	}

	if err := s.SyncNow(); err != nil {
		log.Printf("%s auto-sync failed: %v", s.name, err)
		return
	}

	log.Printf("%s auto-sync completed", s.name)
}

func (s *AutoSyncScheduler) nextDelay() time.Duration {
	enabled, interval := s.getConfig()
	if !enabled || !s.isConfigured() {
		return 24 * time.Hour
	}

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
