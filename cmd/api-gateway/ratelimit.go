package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"tea-platform/pkg/response"
)

// rateLimiter — потокобезопасный лимитер с фиксированным окном по ключу (IP).
// Используется на чувствительных auth-маршрутах как первый барьер против
// брутфорса на уровне шлюза (в дополнение к блокировке по e-mail в account-svc).
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string]*window
	limit  int
	window time.Duration
	lastGC time.Time
}

type window struct {
	count int
	reset time.Time
}

// newRateLimiter создаёт лимитер: не более limit запросов за period с одного IP.
func newRateLimiter(limit int, period time.Duration) *rateLimiter {
	return &rateLimiter{hits: make(map[string]*window), limit: limit, window: period, lastGC: time.Now()}
}

// allow регистрирует запрос от key и сообщает, не превышен ли лимит.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Периодическая очистка истёкших окон, чтобы карта не росла бесконечно.
	if now.Sub(rl.lastGC) > rl.window {
		for k, w := range rl.hits {
			if now.After(w.reset) {
				delete(rl.hits, k)
			}
		}
		rl.lastGC = now
	}

	w, ok := rl.hits[key]
	if !ok || now.After(w.reset) {
		rl.hits[key] = &window{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}

// middleware оборачивает обработчик проверкой лимита по IP клиента.
func (rl *rateLimiter) middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			response.Error(w, http.StatusTooManyRequests, "Слишком много запросов, попробуйте позже")
			return
		}
		next(w, r)
	}
}

// clientIP извлекает IP клиента, учитывая X-Forwarded-For от nginx.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Первый адрес в списке — исходный клиент.
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
