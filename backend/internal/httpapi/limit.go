package httpapi

import (
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// Limits on the routes that work without a token, per client and per server
// instance (backend plan T08). A sign-in takes one start, one form, and one
// callback; a device polls every 5 seconds for at most 10 minutes.
const (
	limitWindow = 10 * time.Minute
	signInLimit = 30
	pollLimit   = 300
)

// limiter counts each client's requests in fixed windows. It keeps the counts
// in memory, so each server instance limits on its own.
type limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	started time.Time
	counts  map[string]int
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{limit: limit, window: window, now: time.Now, counts: map[string]int{}}
}

// allow counts a request from client. It reports whether the request is
// within the limit, and how long until the window ends.
func (l *limiter) allow(client string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.started) >= l.window {
		l.started = now
		clear(l.counts)
	}
	l.counts[client]++
	return l.counts[client] <= l.limit, l.started.Add(l.window).Sub(now)
}

// limited runs next unless the client is over l's limit; then refuse answers
// instead, and Retry-After says when the window ends.
func (s *server) limited(l *limiter, refuse func(http.ResponseWriter), next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ok, wait := l.allow(clientAddress(r, s.proxyHops)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			refuse(w)
			return
		}
		next(w, r)
	}
}

func refuseSignIn(w http.ResponseWriter) {
	writeError(w, &api.Error{Code: api.CodeRateLimited, Message: "too many sign-in attempts; wait a few minutes and try again"})
}

// refusePoll answers as RFC 8628 does a device that polls too often, so the
// apps wait longer without knowing about the limit.
func refusePoll(w http.ResponseWriter) {
	writeError(w, &api.Error{Code: api.CodeSlowDown, Message: "polling too often; wait longer between requests"})
}

func refusePage(w http.ResponseWriter) {
	render(w, http.StatusTooManyRequests, resultPageTemplate,
		resultPageData{"Too many attempts", "Wait a few minutes, then open the link from your device again."})
}

// clientAddress returns who sent r: the entry hops places from the end of
// X-Forwarded-For, which the trusted proxies in front of the server append,
// or the connection's address when hops is 0 or the header has too few
// entries. Earlier entries come from the client and may be forged. An IPv6
// address stands for its /64, which one client usually holds whole.
func clientAddress(r *http.Request, hops int) string {
	address := ""
	if hops > 0 {
		var entries []string
		for _, header := range r.Header.Values("X-Forwarded-For") {
			for _, entry := range strings.Split(header, ",") {
				entries = append(entries, strings.TrimSpace(entry))
			}
		}
		if len(entries) >= hops {
			address = entries[len(entries)-hops]
		}
	}
	if address == "" {
		address = r.RemoteAddr
		if host, _, err := net.SplitHostPort(address); err == nil {
			address = host
		}
	}
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return address
	}
	if ip = ip.Unmap(); ip.Is6() {
		prefix, _ := ip.Prefix(64)
		return prefix.String()
	}
	return ip.String()
}
