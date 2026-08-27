package worker

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/vance1852/community-family-doctor/internal/domain"
	repository "github.com/vance1852/community-family-doctor/internal/repository/sqlite"
)

func openWorkerStore(t *testing.T) *repository.Store {
	t.Helper()
	store, err := repository.Open(context.Background(), filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedWorkerGraph(t *testing.T, store *repository.Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	if err := store.CreateOrganization(ctx, "org-1", "River Authority", now); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	source := domain.WaterSource{ID: "source-1", OrganizationID: "org-1", Name: "North Reservoir", Kind: domain.SourceReservoir, Timezone: "UTC", Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := repository.InsertWaterSource(ctx, store.DB(), source); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	zone := domain.ProtectionZone{ID: "zone-1", SourceID: source.ID, OrganizationID: "org-1", Name: "Primary", Level: domain.ZonePrimary, AreaSquareMeters: 1000, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := repository.InsertProtectionZone(ctx, store.DB(), zone); err != nil {
		t.Fatalf("insert zone: %v", err)
	}
	station := domain.MonitoringStation{ID: "station-1", SourceID: source.ID, ZoneID: zone.ID, OrganizationID: "org-1", Code: "N1", Name: "North", Latitude: 31, Longitude: 121, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := repository.InsertMonitoringStation(ctx, store.DB(), station); err != nil {
		t.Fatalf("insert station: %v", err)
	}
}

// TestProcessOneReleasesJobOnCancellation verifies that when ProcessAlert is
// interrupted by context cancellation, the claimed alert job is released back
// to pending without consuming a retry attempt, and the cancellation error is
// returned so the loop and Run can surface it.
func TestProcessOneReleasesJobOnCancellation(t *testing.T) {
	store := openWorkerStore(t)
	seedWorkerGraph(t, store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Now().UTC()
	reading := domain.TelemetryReading{ID: "reading-cancel", OrganizationID: "org-1", StationID: "station-1", ExternalID: "external-cancel", Parameter: "turbidity", Value: 12, Unit: "NTU", Threshold: 5, ObservedAt: now, ReceivedAt: now}
	if created, err := repository.InsertTelemetryReading(ctx, store.DB(), reading); err != nil || !created {
		t.Fatalf("reading created=%v err=%v", created, err)
	}
	job := domain.AlertJob{ID: "job-cancel", OrganizationID: "org-1", ReadingID: reading.ID, Status: domain.JobPending, MaxAttempts: 2, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := repository.InsertAlertJob(ctx, store.DB(), job); err != nil {
		t.Fatal(err)
	}
	processor := &cancelingAlerts{cancel: cancel}
	r := New(store, processor, nil, slog.Default(), "owner", time.Hour, time.Hour, 1)
	err := r.processOne(ctx, "owner")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("processOne error = %v, want context.Canceled", err)
	}
	var status string
	var attempt int
	if err := store.DB().QueryRow(`SELECT status, attempt_count FROM alert_jobs WHERE id = 'job-cancel'`).Scan(&status, &attempt); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempt != 0 {
		t.Fatalf("after cancel status=%s attempt=%d, want pending/0", status, attempt)
	}
}

type cancelingAlerts struct{ cancel context.CancelFunc }

func (a *cancelingAlerts) ProcessAlert(ctx context.Context, job domain.AlertJob) error {
	a.cancel()
	<-ctx.Done()
	return ctx.Err()
}

// TestRunSurfacesShutdownErrorWithCause verifies Run returns an ErrShutdown
// error that wraps the cancellation cause rather than a silent nil.
func TestRunSurfacesShutdownErrorWithCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	r := New(nil, nil, nil, slog.Default(), "owner", time.Hour, time.Hour, 1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel(context.Canceled)
	}()
	err := r.Run(ctx)
	if err == nil {
		t.Fatal("Run returned nil for canceled context")
	}
	if !errors.Is(err, domain.ErrShutdown) {
		t.Fatalf("Run error = %v, want ErrShutdown", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want cause context.Canceled", err)
	}
}
