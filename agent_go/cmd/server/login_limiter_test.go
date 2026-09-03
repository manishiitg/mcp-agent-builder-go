package server

import (
	"testing"
	"time"
)

func TestLoginLimiterLocksAfterRepeatedFailures(t *testing.T) {
	clock := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	l := &loginRateLimiter{failures: map[string]*loginFailures{}, limit: 3, window: time.Minute, lockout: 5 * time.Minute, now: func() time.Time { return clock }}
	for i := 0; i < 2; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("attempt %d should be allowed", i)
		}
		l.fail("1.2.3.4")
	}
	l.fail("1.2.3.4")
	if l.allow("1.2.3.4") {
		t.Fatal("third failure should lock the address")
	}
	if !l.allow("5.6.7.8") {
		t.Fatal("other addresses are unaffected")
	}
	clock = clock.Add(6 * time.Minute)
	if !l.allow("1.2.3.4") {
		t.Fatal("lockout should expire")
	}
	l.fail("1.2.3.4")
	l.reset("1.2.3.4")
	if !l.allow("1.2.3.4") {
		t.Fatal("a successful login clears the slate")
	}
}
