package outbox

import (
	"context"
	outboxService "github.com/hosseinasadian/mini-wallet/internal/auth/service/outbox"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"sync"
	"time"
)

type WorkerConfig struct {
	BatchInterval    time.Duration `koanf:"batch_interval"`
	RecoveryInterval time.Duration `koanf:"recovery_interval"`
	LockTimeout      time.Duration `koanf:"lock_timeout"`
	BatchSize        int64         `koanf:"batch_size"`
	Concurrency      int64         `koanf:"concurrency"`
}

type Worker struct {
	outboxSvc *outboxService.Service
	logger    *pkgLogger.Logger
	config    WorkerConfig
}

func NewWorker(
	outboxSvc *outboxService.Service,
	logger *pkgLogger.Logger,
	config WorkerConfig,
) *Worker {
	return &Worker{
		outboxSvc: outboxSvc,
		logger:    logger,
		config:    config,
	}
}

func (w *Worker) Start(ctx context.Context) {
	w.logger.Info("outbox worker started",
		"batch_interval", w.config.BatchInterval,
		"recovery_interval", w.config.RecoveryInterval,
	)

	go w.runRecovery(ctx)

	batchTicker := time.NewTicker(w.config.BatchInterval)
	defer batchTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("outbox worker stopped")
			return
		case <-batchTicker.C:
			w.ProcessBatch(ctx)
		}
	}
}

func (w *Worker) runRecovery(ctx context.Context) {
	ticker := time.NewTicker(w.config.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := w.outboxSvc.RecoverStaleEvents(ctx)
			if err != nil {
				w.logger.Error("stale event recovery failed", "error", err)
				continue
			}
			if count > 0 {
				w.logger.Warn("recovered stale outbox events", "count", count)
			}
		}
	}
}

func (w *Worker) ProcessBatch(ctx context.Context) {
	events, err := w.outboxSvc.ClaimOutboxEvents(ctx, w.config.BatchSize, w.config.LockTimeout)
	if err != nil {
		w.logger.Error("failed to claim outbox events", "error", err)
		return
	}

	if len(events) == 0 {
		return
	}

	w.logger.Info("processing outbox batch", "count", len(events))

	var (
		mu        sync.Mutex
		sem       = make(chan struct{}, w.config.Concurrency)
		wg        sync.WaitGroup
		processed []string
		failed    []string
	)

	for _, e := range events {
		wg.Add(1)
		sem <- struct{}{}

		go func(e outboxService.OutboxEvent) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := w.outboxSvc.ProcessEvent(ctx, &e); err != nil {
				w.logger.Error("failed to process event", "event_id", e.EventID, "error", err)

				mu.Lock()
				failed = append(failed, e.EventID)
				mu.Unlock()
				return
			}

			mu.Lock()
			processed = append(processed, e.EventID)
			mu.Unlock()
		}(e)

	}

	wg.Wait()
	if err := w.outboxSvc.MarkEventsProcessed(ctx, processed); err != nil {
		w.logger.Error("failed to mark events as processed", "error", err)
	}
	if err := w.outboxSvc.MarkEventsFailed(ctx, failed); err != nil {
		w.logger.Error("failed to mark events as failed", "error", err)
	}
}
