package controller

import "testing"

func TestLoginGuardLockout(t *testing.T) {
	g := &loginGuard{byIP: make(map[string]*failSlot), byUser: make(map[string]*failSlot)}

	// Below the threshold: allowed.
	for i := 0; i < loginMaxFailures-1; i++ {
		g.fail("1.2.3.4", "admin")
	}
	if !g.allow("1.2.3.4", "admin") {
		t.Fatal("expected attempts below threshold to be allowed")
	}

	// Reaching the threshold locks the IP, the username and both together.
	g.fail("1.2.3.4", "admin")
	if g.allow("1.2.3.4", "admin") {
		t.Fatal("expected lockout after max failures")
	}
	if g.allow("5.6.7.8", "admin") {
		t.Fatal("expected username lockout from another IP")
	}
	if g.allow("1.2.3.4", "root") {
		t.Fatal("expected IP lockout for another username")
	}
	// Unrelated IP + username still allowed.
	if !g.allow("5.6.7.8", "root") {
		t.Fatal("unrelated IP and username must not be locked")
	}

	// Success clears the records.
	g.success("1.2.3.4", "admin")
	if !g.allow("1.2.3.4", "admin") {
		t.Fatal("expected lockout cleared after success")
	}
}
