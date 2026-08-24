package integration_test

import (
	"context"
	"testing"
)

func TestLabAnalystCanReceiveCustodyHandoff(t *testing.T) {
	f := newFixture(t)
	sample := f.createReceivedSample(t, f.createSourceGraph(t))
	if sample.CustodianUserID != f.analyst.UserID {
		t.Fatalf("handoff to analyst was rejected, custodian=%s", sample.CustodianUserID)
	}
	_ = context.Background()
}
