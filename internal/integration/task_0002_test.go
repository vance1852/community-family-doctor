package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vance1852/community-family-doctor/internal/domain"
	"github.com/vance1852/community-family-doctor/internal/incident"
	"github.com/vance1852/community-family-doctor/internal/remediation"
)

func TestDraftRemediationCannotCompleteBeforeApproval(t *testing.T) {
	f := newFixture(t)
	graph := f.createSourceGraph(t)
	incident, err := f.incidents.Report(context.Background(), f.field, incident.ReportCommand{SourceID: graph.source.ID, Title: "Draft response", Description: "Draft response", Severity: domain.SeverityAdvisory, RequestID: "draft-action"})
	if err != nil {
		t.Fatalf("report incident: %v", err)
	}
	plan, err := f.remediation.CreatePlan(context.Background(), f.supervisor, remediation.CreatePlanCommand{
		IncidentID: incident.ID, Title: "Draft response", Objective: "Keep approval gate intact", BudgetCents: 1000,
		Actions: []remediation.CreateAction{{IdempotencyKey: "draft-action-1", Description: "Inspect the affected clinic"}}, RequestID: "draft-plan",
	})
	if err != nil {
		t.Fatalf("create draft plan: %v", err)
	}
	var actionID string
	if err := f.store.DB().QueryRowContext(context.Background(), `SELECT id FROM remediation_actions WHERE plan_id = ?`, plan.ID).Scan(&actionID); err != nil {
		t.Fatalf("load draft action: %v", err)
	}
	err = f.remediation.CompleteAction(context.Background(), f.field, remediation.CompleteActionCommand{
		PlanID: plan.ID, ActionID: actionID, ExpectedVersion: 1, Success: true, RequestID: "draft-complete",
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("draft completion error = %v", err)
	}
	var status string
	var version int64
	if err := f.store.DB().QueryRowContext(context.Background(), `SELECT status, version FROM remediation_actions WHERE id = ?`, actionID).Scan(&status, &version); err != nil {
		t.Fatalf("reload draft action: %v", err)
	}
	if status != string(domain.ActionPending) || version != 1 {
		t.Fatalf("draft action mutated status=%s version=%d", status, version)
	}
}
