package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vance1852/community-family-doctor/internal/domain"
	"github.com/vance1852/community-family-doctor/internal/incident"
)

func TestClaimedIncidentAdvancesWithIssuedLease(t *testing.T) {
	f := newFixture(t)
	graph := f.createSourceGraph(t)
	reported, err := f.incidents.Report(context.Background(), f.field, incident.ReportCommand{SourceID: graph.source.ID, Title: "Clinic alert", Description: "Patients require follow-up", Severity: domain.SeveritySignificant, RequestID: "incident-lease-report"})
	if err != nil {
		t.Fatalf("report incident: %v", err)
	}
	claimed, err := f.incidents.Claim(context.Background(), f.supervisor, reported.ID)
	if err != nil {
		t.Fatalf("claim incident: %v", err)
	}
	if claimed.LeaseToken == "" {
		t.Fatal("claim returned an empty lease")
	}
	err = f.incidents.Advance(context.Background(), f.supervisor, reported.ID, claimed.LeaseToken, domain.IncidentAssessing, "incident-lease-advance")
	if errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("issued lease was rejected: %v", err)
	}
	if err != nil {
		t.Fatalf("advance incident: %v", err)
	}
	var status string
	if err := f.store.DB().QueryRowContext(context.Background(), `SELECT status FROM incidents WHERE id = ?`, reported.ID).Scan(&status); err != nil {
		t.Fatalf("reload incident: %v", err)
	}
	if status != string(domain.IncidentAssessing) {
		t.Fatalf("incident status = %s", status)
	}
}
