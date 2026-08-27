package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vance1852/community-family-doctor/internal/domain"
	repository "github.com/vance1852/community-family-doctor/internal/repository/sqlite"
)

type AlertProcessor interface {
	ProcessAlert(context.Context, domain.AlertJob) error
}
type Notifier interface {
	Deliver(context.Context, string, string, []byte) error
}

type Runtime struct {
	store       *repository.Store
	alerts      AlertProcessor
	notifier    Notifier
	logger      *slog.Logger
	owner       string
	poll        time.Duration
	lease       time.Duration
	concurrency int
}

func New(store *repository.Store, alerts AlertProcessor, notifier Notifier, logger *slog.Logger, owner string, poll, lease time.Duration, concurrency int) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	if concurrency < 1 {
		concurrency = 1
	}
	return &Runtime{store: store, alerts: alerts, notifier: notifier, logger: logger, owner: owner, poll: poll, lease: lease, concurrency: concurrency}
}

// Run blocks until ctx is canceled or a worker goroutine fails unexpectedly.
// On cancellation it returns a *domain.ShutdownError wrapping the context
// cause so the caller can distinguish an expected shutdown from a real
// failure instead of receiving a silent nil.
func (r *Runtime) Run(ctx context.Context) error {
	var workers sync.WaitGroup
	workerCtx, stop := context.WithCancelCause(ctx)
	for index := 0; index < r.concurrency; index++ {
		workers.Add(1)
		go func(workerID int) {
			defer workers.Done()
			r.loop(workerCtx, fmt.Sprintf("%s-%d", r.owner, workerID))
		}(index)
	}
	<-workerCtx.Done()
	// Stop any remaining workers promptly and wait for in-flight jobs to
	// finish releasing their leases before reporting termination.
	stop(context.Cause(workerCtx))
	workers.Wait()
	return terminationError(workerCtx)
}

// terminationError reports the cancellation cause as a shutdown error. A nil
// cause (context canceled without a cause) is still surfaced as ErrShutdown
// so the caller never mistakes cancellation for a clean, work-complete exit.
func terminationError(ctx context.Context) error {
	cause := context.Cause(ctx)
	if cause == nil {
		cause = ctx.Err()
	}
	if cause == nil {
		return nil
	}
	return &domain.ShutdownError{Cause: cause}
}

func (r *Runtime) loop(ctx context.Context, owner string) {
	ticker := time.NewTicker(r.poll)
	defer ticker.Stop()
	for {
		if err := r.processOne(ctx, owner); err != nil {
			switch {
			case errors.Is(err, domain.ErrNotFound):
				// No claimable work this iteration; poll again.
			case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
				// Context cancellation is expected during shutdown; the
				// cause is surfaced by Run, so do not log it as an error.
			default:
				r.logger.ErrorContext(ctx, "worker iteration failed", "owner", owner, "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runtime) processOne(ctx context.Context, owner string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.alerts != nil {
		token, err := leaseToken()
		if err != nil {
			return err
		}
		job, err := r.store.ClaimAlertJob(ctx, owner, token, time.Now().UTC(), r.lease)
		if err == nil {
			processErr := r.alerts.ProcessAlert(ctx, job)
			if processErr != nil && errors.Is(processErr, context.Canceled) {
				// Interrupted by shutdown, not by job failure: release the
				// lease without consuming a retry attempt so the job is
				// reclaimed on the next start. A stale-release (lease
				// already expired or stolen) is tolerated.
				releaseErr := r.store.ReleaseAlertJob(context.Background(), job, time.Now().UTC())
				if releaseErr != nil && !errors.Is(releaseErr, domain.ErrLeaseLost) {
					r.logger.ErrorContext(ctx, "release alert job after cancellation failed", "owner", owner, "job_id", job.ID, "error", releaseErr)
				}
				return processErr
			}
			if finishErr := r.store.FinishAlertJob(ctx, job, processErr, time.Now().UTC()); finishErr != nil {
				return errors.Join(processErr, finishErr)
			}
			return processErr
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
	}
	if r.notifier != nil {
		token, err := leaseToken()
		if err != nil {
			return err
		}
		event, err := r.store.ClaimOutboxEvent(ctx, owner, token, time.Now().UTC(), r.lease)
		if err != nil {
			return err
		}
		deliverErr := r.notifier.Deliver(ctx, event.Topic, event.IdempotencyKey, event.Payload)
		if deliverErr != nil && errors.Is(deliverErr, context.Canceled) {
			releaseErr := r.store.ReleaseOutboxEvent(context.Background(), event, time.Now().UTC())
			if releaseErr != nil && !errors.Is(releaseErr, domain.ErrLeaseLost) {
				r.logger.ErrorContext(ctx, "release outbox event after cancellation failed", "owner", owner, "event_id", event.ID, "error", releaseErr)
			}
			return deliverErr
		}
		if deliverErr != nil {
			return errors.Join(deliverErr, r.store.FailOutboxEvent(ctx, event, deliverErr, time.Now().UTC()))
		}
		return r.store.CompleteOutboxEvent(ctx, event, time.Now().UTC())
	}
	return domain.ErrNotFound
}

func leaseToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate worker lease token: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
