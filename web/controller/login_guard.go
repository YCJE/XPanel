package controller

import (
	"sync"
	"time"
)

// loginGuard throttles brute-force attempts against the single admin account.
// Two keys are tracked independently:
//   - TCP peer address (cannot be spoofed by headers; several attempts from
//     one host share a bucket)
//   - username (protects the account even from distributed attempts)
//
// Both are in-memory on purpose: the panel has a single administrator and the
// guard only needs to make online guessing impractical.
type loginGuard struct {
	mu     sync.Mutex
	byIP   map[string]*failSlot
	byUser map[string]*failSlot
}

type failSlot struct {
	failures    int
	firstFail   time.Time
	lockedUntil time.Time
}

const (
	loginMaxFailures = 5
	loginWindow      = 10 * time.Minute
	loginLockout     = 15 * time.Minute
)

var loginThrottler = &loginGuard{
	byIP:   make(map[string]*failSlot),
	byUser: make(map[string]*failSlot),
}

// allow reports whether a login attempt from ip for username may proceed.
func (g *loginGuard) allow(ip, username string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pruneLocked()
	now := time.Now()
	for _, slot := range []*failSlot{g.byIP[ip], g.byUser[username]} {
		if slot != nil && now.Before(slot.lockedUntil) {
			return false
		}
	}
	return true
}

// fail records a failed attempt; once a slot accumulates loginMaxFailures
// inside loginWindow it is locked out for loginLockout.
func (g *loginGuard) fail(ip, username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for key, m := range map[string]map[string]*failSlot{ip: g.byIP, username: g.byUser} {
		slot := m[key]
		if slot == nil {
			slot = &failSlot{firstFail: now}
			m[key] = slot
		}
		if now.Sub(slot.firstFail) > loginWindow {
			slot.firstFail = now
			slot.failures = 0
		}
		slot.failures++
		if slot.failures >= loginMaxFailures {
			slot.lockedUntil = now.Add(loginLockout)
			slot.failures = 0
			slot.firstFail = now
		}
	}
}

// success clears the records for a successful authentication.
func (g *loginGuard) success(ip, username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.byIP, ip)
	delete(g.byUser, username)
}

// pruneLocked drops stale slots so the maps cannot grow without bound.
// Caller must hold g.mu.
func (g *loginGuard) pruneLocked() {
	now := time.Now()
	for key, slot := range g.byIP {
		if now.Sub(slot.firstFail) > loginWindow && now.After(slot.lockedUntil) {
			delete(g.byIP, key)
		}
	}
	for key, slot := range g.byUser {
		if now.Sub(slot.firstFail) > loginWindow && now.After(slot.lockedUntil) {
			delete(g.byUser, key)
		}
	}
}
