package node

import (
	"net/http/httptest"
	"testing"
	"time"

	"nftvault/cas"
	"nftvault/control"
)

// TestNodeEnforcesQuotaViaControl proves the end-to-end billing/space path:
// a node in control mode rejects writes that would exceed the tenant's plan.
func TestNodeEnforcesQuotaViaControl(t *testing.T) {
	// Control server with a 1000-byte plan.
	store, _ := control.NewStore("")
	svc := control.NewService(store, time.Now, control.ManualProvider{})
	ctrlSrv := httptest.NewServer(control.NewAPI(svc, "admin").Handler())
	t.Cleanup(ctrlSrv.Close)

	plan, _ := svc.CreatePlan("Tiny", 1000, 0, "EUR", control.PeriodMonthly)
	_, token, err := svc.CreateAccount("Tenant", "", plan.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Storage node in control mode.
	be, _ := cas.NewFSBackend("disk", t.TempDir())
	nodeSrv := httptest.NewServer(NewServer(be, "").WithQuota(control.NewClient(ctrlSrv.URL)).Handler())
	t.Cleanup(nodeSrv.Close)

	rb := NewRemoteBackend("point", nodeSrv.URL, token)

	// 600 bytes: within quota.
	block1 := make([]byte, 600)
	if err := rb.Put(cas.Address(block1), block1); err != nil {
		t.Fatalf("put within quota: %v", err)
	}
	// Another 600 bytes: 1200 > 1000 quota -> node must refuse.
	block2 := make([]byte, 600)
	for i := range block2 {
		block2[i] = 1 // different content -> different address
	}
	if err := rb.Put(cas.Address(block2), block2); err == nil {
		t.Fatal("node stored data that exceeds the tenant quota")
	}

	// Confirm the control server recorded only the first block's usage.
	st, err := control.NewClient(ctrlSrv.URL).Status(token)
	if err != nil {
		t.Fatal(err)
	}
	if st.BytesUsed != 600 {
		t.Fatalf("usage = %d, want 600", st.BytesUsed)
	}

	// Re-putting the SAME first block must not double-charge.
	if err := rb.Put(cas.Address(block1), block1); err != nil {
		t.Fatalf("re-put existing block: %v", err)
	}
	st, _ = control.NewClient(ctrlSrv.URL).Status(token)
	if st.BytesUsed != 600 {
		t.Fatalf("dedup double-charged: usage = %d", st.BytesUsed)
	}

	// A missing/empty token is rejected by the node.
	if err := NewRemoteBackend("x", nodeSrv.URL, "").Put(cas.Address(block1), block1); err == nil {
		t.Fatal("node accepted a write with no account token")
	}
}
