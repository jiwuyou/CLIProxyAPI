package auth

import "testing"

func TestQoderAuthenticatorDisablesScheduledRefresh(t *testing.T) {
	if lead := NewQoderAuthenticator().RefreshLead(); lead != nil {
		t.Fatalf("RefreshLead() = %v, want nil without a working refresh flow", *lead)
	}
}
