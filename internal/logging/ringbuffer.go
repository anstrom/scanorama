// Package logging - in-memory ring buffer for capturing and querying log entries.
package logging

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const defaultCapacity = 2000

const subscriberChanSize = 128

const (
	levelDebug = 0
	levelInfo  = 1
	levelWarn  = 2
	levelError = 3
)

// LogEntry is a single captured log record stored in the ring buffer.
type LogEntry struct {
	Time      time.Time         `json:"time"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Component string            `json:"component,omitempty"`
	Attrs     map[string]string `json:"attrs,omitempty"`
}

// LogFilter holds the parameters for querying the ring buffer.
// Zero values mean "no filter" for each field.
type LogFilter struct {
	Level     string
	Component string
	Search    string
	Since     time.Time
	Until     time.Time
	Page      int
	PageSize  int
}

// RingBuffer is a thread-safe circular buffer of LogEntry values.
// It also maintains a set of subscriber channels for real-time streaming.
type RingBuffer struct {
	entries  []LogEntry
	capacity int
	head     int // index of the next write slot
	count    int // number of valid entries (≤ capacity)
	mu       sync.RWMutex

	subsMu      sync.Mutex
	subscribers map[chan LogEntry]struct{}
}

// NewRingBuffer creates a new RingBuffer with the given capacity.
// A capacity ≤ 0 uses the default of 2000 entries.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	return &RingBuffer{
		entries:     make([]LogEntry, capacity),
		capacity:    capacity,
		subscribers: make(map[chan LogEntry]struct{}),
	}
}

// Append writes entry into the ring buffer and notifies all subscribers
// with a non-blocking send (slow subscribers simply miss the entry).
func (rb *RingBuffer) Append(entry LogEntry) {
	rb.mu.Lock()
	rb.entries[rb.head] = entry
	rb.head = (rb.head + 1) % rb.capacity
	if rb.count < rb.capacity {
		rb.count++
	}
	rb.mu.Unlock()

	// Fan-out to subscribers — non-blocking so a slow reader never stalls writers.
	rb.subsMu.Lock()
	for ch := range rb.subscribers {
		select {
		case ch <- entry:
		default: // subscriber channel full; drop
		}
	}
	rb.subsMu.Unlock()
}

// Subscribe returns a buffered channel that receives every future log entry.
// Call Unsubscribe when done to avoid goroutine / channel leaks.
func (rb *RingBuffer) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, subscriberChanSize)
	rb.subsMu.Lock()
	rb.subscribers[ch] = struct{}{}
	rb.subsMu.Unlock()
	return ch
}

// Unsubscribe removes ch from the subscriber set and closes it so that a
// range loop over the channel terminates cleanly.
func (rb *RingBuffer) Unsubscribe(ch chan LogEntry) {
	rb.subsMu.Lock()
	delete(rb.subscribers, ch)
	rb.subsMu.Unlock()
	close(ch)
}

// Recent returns up to n entries in chronological order (oldest first).
// If n exceeds the number of stored entries, all entries are returned.
func (rb *RingBuffer) Recent(n int) []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n <= 0 || rb.count == 0 {
		return []LogEntry{}
	}
	if n > rb.count {
		n = rb.count
	}

	result := make([]LogEntry, n)
	// The oldest of the last n entries sits at (head - n) mod capacity.
	start := (rb.head - n + rb.capacity) % rb.capacity
	for i := 0; i < n; i++ {
		result[i] = rb.entries[(start+i)%rb.capacity]
	}
	return result
}

// Query filters the buffer contents and returns a newest-first paginated slice
// together with the total count before pagination.
//
// Level filtering is minimum-level: "debug" shows everything, "info" hides
// debug, "warn" hides debug+info, "error" shows only errors.
// An empty Level string matches all levels.
//
// total is the number of entries that matched the filter (before pagination).
func (rb *RingBuffer) Query(f LogFilter) (entries []LogEntry, total int) {
	rb.mu.RLock()
	all := rb.chronological()
	rb.mu.RUnlock()

	minLevel := parseLevel(f.Level)

	filtered := make([]LogEntry, 0, len(all))
	for _, e := range all {
		if matchesFilter(e, f, minLevel) {
			filtered = append(filtered, e)
		}
	}

	total = len(filtered)

	// Reverse to newest-first before pagination.
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	// Pagination defaults.
	page := f.Page
	pageSize := f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize
	if offset >= len(filtered) {
		return []LogEntry{}, total
	}
	end := offset + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total
}

// Handler returns a slog.Handler that appends every handled record to rb.
func (rb *RingBuffer) Handler() slog.Handler {
	return &ringBufferHandler{rb: rb}
}

// chronological returns all stored entries in oldest-first order.
// Caller must hold rb.mu (at least RLock) before calling.
func (rb *RingBuffer) chronological() []LogEntry {
	if rb.count == 0 {
		return []LogEntry{}
	}
	result := make([]LogEntry, rb.count)
	start := (rb.head - rb.count + rb.capacity) % rb.capacity
	for i := 0; i < rb.count; i++ {
		result[i] = rb.entries[(start+i)%rb.capacity]
	}
	return result
}

// ---------------------------------------------------------------------------
// ringBufferHandler — implements slog.Handler
// ---------------------------------------------------------------------------

type ringBufferHandler struct {
	rb    *RingBuffer
	attrs []slog.Attr // attrs attached via WithAttrs
	group string      // group set via WithGroup
}

// Enabled always returns true so that every log record is captured regardless
// of the level set on the primary (console) handler.
func (h *ringBufferHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// Handle converts the slog.Record into a LogEntry and appends it to the ring
// buffer. The "component" (or "handler") attribute is promoted to a dedicated
// field; all other attributes are collected into Attrs.
func (h *ringBufferHandler) Handle(_ context.Context, record slog.Record) error {
	entry := LogEntry{
		Time:    record.Time,
		Level:   strings.ToLower(record.Level.String()),
		Message: record.Message,
		Attrs:   make(map[string]string),
	}

	// Pre-attached attrs (from logger.With(...)).
	for _, a := range h.attrs {
		switch a.Key {
		case "component", "handler":
			entry.Component = a.Value.String()
		default:
			entry.Attrs[a.Key] = a.Value.String()
		}
	}

	// Per-record attrs.
	record.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "component", "handler":
			entry.Component = a.Value.String()
		default:
			entry.Attrs[a.Key] = a.Value.String()
		}
		return true
	})

	// Omit empty attrs map to keep JSON tidy.
	if len(entry.Attrs) == 0 {
		entry.Attrs = nil
	}

	h.rb.Append(entry)
	return nil
}

// WithAttrs returns a new handler with the given attrs pre-attached.
func (h *ringBufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(combined, h.attrs)
	copy(combined[len(h.attrs):], attrs)
	return &ringBufferHandler{
		rb:    h.rb,
		attrs: combined,
		group: h.group,
	}
}

// WithGroup returns a new handler with the group name set.
func (h *ringBufferHandler) WithGroup(name string) slog.Handler {
	return &ringBufferHandler{
		rb:    h.rb,
		attrs: h.attrs,
		group: name,
	}
}

// ---------------------------------------------------------------------------
// TeeHandler — fan-out to two slog.Handler values
// ---------------------------------------------------------------------------

// teeHandler writes every record to both primary and secondary.
type teeHandler struct {
	primary   slog.Handler
	secondary slog.Handler
}

// TeeHandler returns a slog.Handler that forwards every record to both
// primary and secondary. Enabled returns true when either sub-handler is
// enabled. WithAttrs and WithGroup are propagated to both sub-handlers.
func TeeHandler(primary, secondary slog.Handler) slog.Handler {
	return &teeHandler{primary: primary, secondary: secondary}
}

func (h *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.primary.Enabled(ctx, level) || h.secondary.Enabled(ctx, level)
}

func (h *teeHandler) Handle(ctx context.Context, record slog.Record) error {
	// Clone the record so each handler gets an independent copy.
	r1 := record.Clone()
	r2 := record.Clone()

	firstErr := h.primary.Handle(ctx, r1)
	if err := h.secondary.Handle(ctx, r2); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &teeHandler{
		primary:   h.primary.WithAttrs(attrs),
		secondary: h.secondary.WithAttrs(attrs),
	}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{
		primary:   h.primary.WithGroup(name),
		secondary: h.secondary.WithGroup(name),
	}
}

// ---------------------------------------------------------------------------
// Level helpers
// ---------------------------------------------------------------------------

// parseLevel converts a level string to a numeric value for minimum-level
// comparisons. Unknown strings are treated as debug (0 = show everything).
func parseLevel(level string) int {
	switch strings.ToLower(level) {
	case "debug":
		return levelDebug
	case "info":
		return levelInfo
	case "warn", "warning":
		return levelWarn
	case "error":
		return levelError
	default:
		return levelDebug
	}
}

// matchesFilter reports whether entry e passes all criteria in f with the given minimum level.
func matchesFilter(e LogEntry, f LogFilter, minLevel int) bool {
	if f.Level != "" && parseLevel(e.Level) < minLevel {
		return false
	}
	if f.Component != "" && e.Component != f.Component {
		return false
	}
	if f.Search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(f.Search)) {
		return false
	}
	if !f.Since.IsZero() && e.Time.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && e.Time.After(f.Until) {
		return false
	}
	return true
}
