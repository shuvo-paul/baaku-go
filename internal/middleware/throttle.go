// Login throttling: max login attempts per client per window, in memory.
//
// Laravel parity: Fortify's login limiter, 5 attempts/min (Wave-2 plan H1).
// The reference config ships limiters.login = null, so this is the port's own
// defensive default, not a copy of a configured limiter.
package middleware

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// ThrottleMessage mirrors lang/en/auth.php 'throttle'.
const ThrottleMessage = "Too many login attempts. Please try again in %d seconds."

// LoginThrottle limits each client IP to max requests per window on the
// wrapped route (mount on POST /login).
//
// ponytail: in-memory fixed windows per process — resets on restart and is
// per-instance behind multiple replicas; move to a shared store (Redis) when
// that ceiling matters. Counts every attempt, successes included; a
// failure-only limiter needs handler cooperation. The key map holds one entry
// per distinct IP until restart — fine at dev scale.
// LoginThrottle limits each client IP to max requests per window on the
// wrapped route (mount on POST /login).
//
// ponytail: in-memory fixed windows per process — resets on restart and is
// per-instance behind multiple replicas; move to a shared store (Redis) when
// that ceiling matters. Counts every attempt, successes included; a
// failure-only limiter needs handler cooperation. The key map holds one entry
// per distinct IP until restart — fine at dev scale.
func LoginThrottle(max int, window time.Duration) func(http.Handler) http.Handler {
	return throttle(max, window, ThrottleMessage)
}

// Throttle is the generic fixed-window per-IP limiter behind Fortify's
// throttled routes (verification.verify and verification.send ship
// throttle:6,1). Same in-memory caveats as LoginThrottle.
func Throttle(max int, window time.Duration) func(http.Handler) http.Handler {
	return throttle(max, window, TooManyAttemptsMessage)
}

// TooManyAttemptsMessage is Laravel's generic throttle failure message.
const TooManyAttemptsMessage = "Too many attempts. Please try again in %d seconds."

func throttle(max int, window time.Duration, message string) func(http.Handler) http.Handler {
	var (
		mu      sync.Mutex
		buckets = map[string]*bucket{}
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientIP(r)
			now := time.Now()
			mu.Lock()
			b := buckets[key]
			if b == nil || now.After(b.resetAt) {
				b = &bucket{resetAt: now.Add(window)}
				buckets[key] = b
			}
			b.n++
			n, resetAt := b.n, b.resetAt
			mu.Unlock()
			if n > max {
				secs := int(time.Until(resetAt).Seconds()) + 1
				http.Error(w, fmt.Sprintf(message, secs), http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type bucket struct {
	n       int
	resetAt time.Time
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
