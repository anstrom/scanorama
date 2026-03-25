package logging

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// Test helpers
// ----------------------------------------------------------------------------

func makeEntry(level, message, component string) LogEntry {
	return LogEntry{
		Time:      time.Now(),
		Level:     level,
		Message:   message,
		Component: component,
	}
}

// mockHandler is a minimal slog.Handler used to drive TeeHandler tests.
type mockHandler struct {
	mu        sync.Mutex
	enabled   bool
	handled   []slog.Record
	attrs     []slog.Attr
	group     string
	handleErr error
}

func (m *mockHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return m.enabled
}

func (m *mockHandler) Handle(_ context.Context, r slog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handled = append(m.handled, r)
	return m.handleErr
}

func (m *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	m.mu.Lock()
	combined := make([]slog.Attr, len(m.attrs)+len(attrs))
	copy(combined, m.attrs)
	copy(combined[len(m.attrs):], attrs)
	m.mu.Unlock()
	return &mockHandler{
		enabled: m.enabled,
		attrs:   combined,
		group:   m.group,
	}
}

func (m *mockHandler) WithGroup(name string) slog.Handler {
	return &mockHandler{
		enabled: m.enabled,
		attrs:   m.attrs,
		group:   name,
	}
}

func (m *mockHandler) recordCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.handled)
}

// ----------------------------------------------------------------------------
// NewRingBuffer
// ----------------------------------------------------------------------------

func TestNewRingBuffer(t *testing.T) {
	t.Run("zero capacity uses defaultCapacity", func(t *testing.T) {
		rb := NewRingBuffer(0)
		assert.Equal(t, defaultCapacity, rb.capacity)
		assert.Len(t, rb.entries, defaultCapacity)
	})

	t.Run("negative capacity uses defaultCapacity", func(t *testing.T) {
		rb := NewRingBuffer(-100)
		assert.Equal(t, defaultCapacity, rb.capacity)
		assert.Len(t, rb.entries, defaultCapacity)
	})

	t.Run("explicit positive capacity stored correctly", func(t *testing.T) {
		rb := NewRingBuffer(42)
		assert.Equal(t, 42, rb.capacity)
		assert.Len(t, rb.entries, 42)
	})
}

// ----------------------------------------------------------------------------
// Append
// ----------------------------------------------------------------------------

func TestRingBuffer_Append(t *testing.T) {
	t.Run("single append is visible in Recent(1)", func(t *testing.T) {
		rb := NewRingBuffer(10)
		entry := makeEntry("info", "hello", "")
		rb.Append(entry)

		recent := rb.Recent(1)
		require.Len(t, recent, 1)
		assert.Equal(t, entry.Message, recent[0].Message)
		assert.Equal(t, entry.Level, recent[0].Level)
	})

	t.Run("wrap-around: count stays at capacity, first entry gone", func(t *testing.T) {
		const bufCap = 5
		rb := NewRingBuffer(bufCap)
		for i := 0; i < bufCap+3; i++ {
			rb.Append(makeEntry("info", fmt.Sprintf("msg%d", i), ""))
		}

		// Count must not exceed capacity.
		assert.Equal(t, bufCap, rb.count)

		all := rb.Recent(bufCap)
		require.Len(t, all, bufCap)

		// Newest entry is the last one appended (msg7 when bufCap=5).
		assert.Equal(t, fmt.Sprintf("msg%d", bufCap+2), all[bufCap-1].Message)

		// The very first entry (msg0) must no longer be present.
		for _, e := range all {
			assert.NotEqual(t, "msg0", e.Message)
		}
	})

	t.Run("concurrent: 20 goroutines x 10 appends, no race, count never exceeds capacity", func(t *testing.T) {
		t.Parallel()
		const capacity = 50
		const goroutines = 20
		const perGoroutine = 10
		rb := NewRingBuffer(capacity)

		var wg sync.WaitGroup
		wg.Add(goroutines)
		for g := 0; g < goroutines; g++ {
			go func(g int) {
				defer wg.Done()
				for i := 0; i < perGoroutine; i++ {
					rb.Append(makeEntry("info", fmt.Sprintf("g%d-i%d", g, i), ""))
				}
			}(g)
		}
		wg.Wait()

		rb.mu.RLock()
		count := rb.count
		rb.mu.RUnlock()
		assert.LessOrEqual(t, count, capacity)
	})
}

// ----------------------------------------------------------------------------
// Subscribe / Unsubscribe
// ----------------------------------------------------------------------------

func TestRingBuffer_Subscribe(t *testing.T) {
	t.Run("subscriber receives entry appended after subscribe", func(t *testing.T) {
		rb := NewRingBuffer(10)
		ch := rb.Subscribe()
		defer rb.Unsubscribe(ch)

		entry := makeEntry("info", "streamed-msg", "comp1")
		rb.Append(entry)

		select {
		case got := <-ch:
			assert.Equal(t, entry.Message, got.Message)
			assert.Equal(t, entry.Component, got.Component)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for subscriber message")
		}
	})

	t.Run("Unsubscribe closes channel so range terminates immediately", func(t *testing.T) {
		rb := NewRingBuffer(10)
		ch := rb.Subscribe()
		rb.Unsubscribe(ch)

		// Ranging over a closed empty channel must terminate without blocking.
		done := make(chan struct{})
		go func() {
			for range ch {
			}
			close(done)
		}()

		select {
		case <-done:
			// success
		case <-time.After(200 * time.Millisecond):
			t.Fatal("range over closed channel did not terminate promptly")
		}
	})

	t.Run("slow subscriber (full channel): extra Appends complete within 100ms", func(t *testing.T) {
		rb := NewRingBuffer(subscriberChanSize + 20)
		ch := rb.Subscribe()
		defer rb.Unsubscribe(ch)

		// Fill the subscriber channel to its exact capacity without draining it.
		for i := 0; i < subscriberChanSize; i++ {
			rb.Append(makeEntry("info", fmt.Sprintf("fill%d", i), ""))
		}

		// Channel is now full. Additional appends must take the default branch
		// (drop) and must not block the caller.
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < 10; i++ {
				rb.Append(makeEntry("warn", fmt.Sprintf("overflow%d", i), ""))
			}
		}()

		select {
		case <-done:
			// success – Append never blocked
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Append blocked on a full subscriber channel")
		}
	})
}

// ----------------------------------------------------------------------------
// Recent
// ----------------------------------------------------------------------------

func TestRingBuffer_Recent(t *testing.T) {
	t.Run("empty buffer returns empty slice", func(t *testing.T) {
		rb := NewRingBuffer(10)
		assert.Equal(t, []LogEntry{}, rb.Recent(5))
	})

	t.Run("n=0 returns empty slice", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("info", "msg", ""))
		assert.Equal(t, []LogEntry{}, rb.Recent(0))
	})

	t.Run("n greater than count returns all stored entries", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("info", "a", ""))
		rb.Append(makeEntry("info", "b", ""))
		recent := rb.Recent(100)
		require.Len(t, recent, 2)
		assert.Equal(t, "a", recent[0].Message)
		assert.Equal(t, "b", recent[1].Message)
	})

	t.Run("n less than count returns last n in oldest-first order", func(t *testing.T) {
		rb := NewRingBuffer(10)
		for i := 0; i < 5; i++ {
			rb.Append(makeEntry("info", fmt.Sprintf("msg%d", i), ""))
		}

		recent := rb.Recent(3)
		require.Len(t, recent, 3)
		assert.Equal(t, "msg2", recent[0].Message)
		assert.Equal(t, "msg3", recent[1].Message)
		assert.Equal(t, "msg4", recent[2].Message)
	})

	t.Run("after wrap-around Recent(2) returns two newest", func(t *testing.T) {
		const bufCap = 5
		rb := NewRingBuffer(bufCap)
		// Append bufCap+3 items so the oldest 3 are evicted.
		for i := 0; i < bufCap+3; i++ {
			rb.Append(makeEntry("info", fmt.Sprintf("msg%d", i), ""))
		}

		recent := rb.Recent(2)
		require.Len(t, recent, 2)
		// With bufCap=5 and 8 items appended, the two newest are msg6 and msg7.
		assert.Equal(t, fmt.Sprintf("msg%d", bufCap+1), recent[0].Message)
		assert.Equal(t, fmt.Sprintf("msg%d", bufCap+2), recent[1].Message)
	})
}

// ----------------------------------------------------------------------------
// Query
// ----------------------------------------------------------------------------

func TestRingBuffer_Query(t *testing.T) {
	t.Run("empty buffer returns empty slice and total=0", func(t *testing.T) {
		rb := NewRingBuffer(10)
		entries, total := rb.Query(LogFilter{})
		assert.Equal(t, []LogEntry{}, entries)
		assert.Equal(t, 0, total)
	})

	t.Run("no filter returns all entries newest-first with correct total", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("info", "first", ""))
		rb.Append(makeEntry("info", "second", ""))
		rb.Append(makeEntry("info", "third", ""))

		entries, total := rb.Query(LogFilter{PageSize: 10})
		assert.Equal(t, 3, total)
		require.Len(t, entries, 3)
		assert.Equal(t, "third", entries[0].Message)
		assert.Equal(t, "second", entries[1].Message)
		assert.Equal(t, "first", entries[2].Message)
	})

	t.Run("level=info: debug excluded, info/warn/error included", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("debug", "dbg", ""))
		rb.Append(makeEntry("info", "inf", ""))
		rb.Append(makeEntry("warn", "wrn", ""))
		rb.Append(makeEntry("error", "err", ""))

		entries, total := rb.Query(LogFilter{Level: "info", PageSize: 10})
		assert.Equal(t, 3, total)
		require.Len(t, entries, 3)
		for _, e := range entries {
			assert.NotEqual(t, "debug", e.Level)
		}
	})

	t.Run("level=error: only error entries returned", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("debug", "dbg", ""))
		rb.Append(makeEntry("info", "inf", ""))
		rb.Append(makeEntry("warn", "wrn", ""))
		rb.Append(makeEntry("error", "err", ""))

		entries, total := rb.Query(LogFilter{Level: "error", PageSize: 10})
		assert.Equal(t, 1, total)
		require.Len(t, entries, 1)
		assert.Equal(t, "error", entries[0].Level)
	})

	t.Run("empty Level string matches all levels (same as no filter)", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("debug", "dbg", ""))
		rb.Append(makeEntry("info", "inf", ""))

		_, totalEmpty := rb.Query(LogFilter{Level: "", PageSize: 10})
		_, totalDebug := rb.Query(LogFilter{Level: "debug", PageSize: 10})
		assert.Equal(t, totalDebug, totalEmpty)
	})

	t.Run("component: exact match only, partial prefix excluded", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("info", "msg1", "auth"))
		rb.Append(makeEntry("info", "msg2", "auth-service"))
		rb.Append(makeEntry("info", "msg3", "db"))

		entries, total := rb.Query(LogFilter{Component: "auth", PageSize: 10})
		assert.Equal(t, 1, total)
		require.Len(t, entries, 1)
		assert.Equal(t, "auth", entries[0].Component)
	})

	t.Run("search: case-insensitive substring match", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("info", "Hello World", ""))
		rb.Append(makeEntry("info", "goodbye", ""))

		entries, total := rb.Query(LogFilter{Search: "HELLO", PageSize: 10})
		assert.Equal(t, 1, total)
		require.Len(t, entries, 1)
		assert.Equal(t, "Hello World", entries[0].Message)
	})

	t.Run("search: non-matching term returns empty", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("info", "hello", ""))

		entries, total := rb.Query(LogFilter{Search: "xyz", PageSize: 10})
		assert.Equal(t, 0, total)
		assert.Equal(t, []LogEntry{}, entries)
	})

	t.Run("since: excludes entries before the time", func(t *testing.T) {
		rb := NewRingBuffer(10)
		base := time.Now()
		rb.Append(LogEntry{Time: base.Add(-2 * time.Hour), Level: "info", Message: "old"})
		rb.Append(LogEntry{Time: base.Add(1 * time.Hour), Level: "info", Message: "new"})

		entries, total := rb.Query(LogFilter{Since: base, PageSize: 10})
		assert.Equal(t, 1, total)
		require.Len(t, entries, 1)
		assert.Equal(t, "new", entries[0].Message)
	})

	t.Run("until: excludes entries after the time", func(t *testing.T) {
		rb := NewRingBuffer(10)
		base := time.Now()
		rb.Append(LogEntry{Time: base.Add(-2 * time.Hour), Level: "info", Message: "old"})
		rb.Append(LogEntry{Time: base.Add(2 * time.Hour), Level: "info", Message: "future"})

		entries, total := rb.Query(LogFilter{Until: base, PageSize: 10})
		assert.Equal(t, 1, total)
		require.Len(t, entries, 1)
		assert.Equal(t, "old", entries[0].Message)
	})

	t.Run("pagination: page 1 vs page 2 have correct non-overlapping entries", func(t *testing.T) {
		rb := NewRingBuffer(20)
		for i := 0; i < 10; i++ {
			rb.Append(makeEntry("info", fmt.Sprintf("msg%d", i), ""))
		}

		page1, total1 := rb.Query(LogFilter{Page: 1, PageSize: 3})
		page2, total2 := rb.Query(LogFilter{Page: 2, PageSize: 3})

		assert.Equal(t, 10, total1)
		assert.Equal(t, 10, total2)
		require.Len(t, page1, 3)
		require.Len(t, page2, 3)

		// Newest-first: page 1 = msg9, msg8, msg7.
		assert.Equal(t, "msg9", page1[0].Message)
		assert.Equal(t, "msg8", page1[1].Message)
		assert.Equal(t, "msg7", page1[2].Message)
		// Page 2: msg6, msg5, msg4.
		assert.Equal(t, "msg6", page2[0].Message)
		assert.Equal(t, "msg5", page2[1].Message)
		assert.Equal(t, "msg4", page2[2].Message)

		// Pages must not overlap.
		page1Set := make(map[string]bool)
		for _, e := range page1 {
			page1Set[e.Message] = true
		}
		for _, e := range page2 {
			assert.False(t, page1Set[e.Message], "page 2 entry %q also found in page 1", e.Message)
		}
	})

	t.Run("pagination: page beyond total returns empty slice, total unchanged", func(t *testing.T) {
		rb := NewRingBuffer(10)
		rb.Append(makeEntry("info", "only", ""))

		entries, total := rb.Query(LogFilter{Page: 99, PageSize: 10})
		assert.Equal(t, 1, total)
		assert.Equal(t, []LogEntry{}, entries)
	})

	t.Run("combined filters: level + search + component all applied", func(t *testing.T) {
		rb := NewRingBuffer(20)
		rb.Append(makeEntry("debug", "db connected", "db"))    // excluded: below info
		rb.Append(makeEntry("info", "db query ok", "db"))      // included
		rb.Append(makeEntry("info", "auth login", "auth"))     // excluded: wrong component
		rb.Append(makeEntry("error", "db query failed", "db")) // included

		entries, total := rb.Query(LogFilter{
			Level:     "info",
			Search:    "db",
			Component: "db",
			PageSize:  10,
		})
		assert.Equal(t, 2, total)
		require.Len(t, entries, 2)
		// Newest-first: error entry was appended last.
		assert.Equal(t, "db query failed", entries[0].Message)
		assert.Equal(t, "db query ok", entries[1].Message)
	})
}

// ----------------------------------------------------------------------------
// Handler / ringBufferHandler
// ----------------------------------------------------------------------------

func TestRingBuffer_Handler(t *testing.T) {
	t.Run("Enabled always returns true for any level", func(t *testing.T) {
		rb := NewRingBuffer(10)
		h := rb.Handler()
		ctx := context.Background()
		assert.True(t, h.Enabled(ctx, slog.LevelDebug))
		assert.True(t, h.Enabled(ctx, slog.LevelInfo))
		assert.True(t, h.Enabled(ctx, slog.LevelWarn))
		assert.True(t, h.Enabled(ctx, slog.LevelError))
	})

	t.Run("Handle: level stored lowercase, message and time preserved", func(t *testing.T) {
		rb := NewRingBuffer(10)
		h := rb.Handler()
		now := time.Now().Truncate(time.Millisecond)

		record := slog.NewRecord(now, slog.LevelWarn, "test message", 0)
		require.NoError(t, h.Handle(context.Background(), record))

		recent := rb.Recent(1)
		require.Len(t, recent, 1)
		assert.Equal(t, "warn", recent[0].Level)
		assert.Equal(t, "test message", recent[0].Message)
		assert.Equal(t, now, recent[0].Time)
	})

	t.Run("Handle: 'component' attr promoted to LogEntry.Component", func(t *testing.T) {
		rb := NewRingBuffer(10)
		h := rb.Handler()

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		record.AddAttrs(slog.String("component", "my-component"))
		require.NoError(t, h.Handle(context.Background(), record))

		recent := rb.Recent(1)
		require.Len(t, recent, 1)
		assert.Equal(t, "my-component", recent[0].Component)
		assert.Nil(t, recent[0].Attrs)
	})

	t.Run("Handle: 'handler' attr also promoted to LogEntry.Component", func(t *testing.T) {
		rb := NewRingBuffer(10)
		h := rb.Handler()

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		record.AddAttrs(slog.String("handler", "my-handler"))
		require.NoError(t, h.Handle(context.Background(), record))

		recent := rb.Recent(1)
		require.Len(t, recent, 1)
		assert.Equal(t, "my-handler", recent[0].Component)
		assert.Nil(t, recent[0].Attrs)
	})

	t.Run("Handle: other attrs collected into LogEntry.Attrs map", func(t *testing.T) {
		rb := NewRingBuffer(10)
		h := rb.Handler()

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		record.AddAttrs(slog.String("user", "alice"), slog.Int("code", 42))
		require.NoError(t, h.Handle(context.Background(), record))

		recent := rb.Recent(1)
		require.Len(t, recent, 1)
		require.NotNil(t, recent[0].Attrs)
		assert.Equal(t, "alice", recent[0].Attrs["user"])
		assert.Equal(t, "42", recent[0].Attrs["code"])
	})

	t.Run("Handle: Attrs is nil when no non-component attrs present", func(t *testing.T) {
		rb := NewRingBuffer(10)
		h := rb.Handler()

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		require.NoError(t, h.Handle(context.Background(), record))

		recent := rb.Recent(1)
		require.Len(t, recent, 1)
		assert.Nil(t, recent[0].Attrs)
	})

	t.Run("WithAttrs: pre-attached attrs appear in subsequent Handle calls", func(t *testing.T) {
		rb := NewRingBuffer(10)
		h := rb.Handler().WithAttrs([]slog.Attr{
			slog.String("component", "pre-comp"),
			slog.String("extra", "val"),
		})

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		require.NoError(t, h.Handle(context.Background(), record))

		recent := rb.Recent(1)
		require.Len(t, recent, 1)
		assert.Equal(t, "pre-comp", recent[0].Component)
		require.NotNil(t, recent[0].Attrs)
		assert.Equal(t, "val", recent[0].Attrs["extra"])
	})

	t.Run("WithGroup: returns new handler, does not panic, group field set", func(t *testing.T) {
		rb := NewRingBuffer(10)
		h := rb.Handler()
		h2 := h.WithGroup("mygroup")

		assert.NotNil(t, h2)
		rbh, ok := h2.(*ringBufferHandler)
		require.True(t, ok, "WithGroup must return a *ringBufferHandler")
		assert.Equal(t, "mygroup", rbh.group)

		// The new handler must still be functional.
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "after-group", 0)
		require.NoError(t, h2.Handle(context.Background(), record))
		assert.Len(t, rb.Recent(1), 1)
	})
}

// ----------------------------------------------------------------------------
// TeeHandler
// ----------------------------------------------------------------------------

func TestTeeHandler(t *testing.T) {
	t.Run("Handle writes to both primary and secondary", func(t *testing.T) {
		rb1 := NewRingBuffer(10)
		rb2 := NewRingBuffer(10)
		tee := TeeHandler(rb1.Handler(), rb2.Handler())

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "tee message", 0)
		require.NoError(t, tee.Handle(context.Background(), record))

		r1 := rb1.Recent(1)
		r2 := rb2.Recent(1)
		require.Len(t, r1, 1)
		require.Len(t, r2, 1)
		assert.Equal(t, "tee message", r1[0].Message)
		assert.Equal(t, "tee message", r2[0].Message)
	})

	t.Run("Enabled true when primary enabled, secondary disabled", func(t *testing.T) {
		tee := TeeHandler(&mockHandler{enabled: true}, &mockHandler{enabled: false})
		assert.True(t, tee.Enabled(context.Background(), slog.LevelInfo))
	})

	t.Run("Enabled true when secondary enabled, primary disabled", func(t *testing.T) {
		tee := TeeHandler(&mockHandler{enabled: false}, &mockHandler{enabled: true})
		assert.True(t, tee.Enabled(context.Background(), slog.LevelInfo))
	})

	t.Run("Enabled false when both sub-handlers are disabled", func(t *testing.T) {
		tee := TeeHandler(&mockHandler{enabled: false}, &mockHandler{enabled: false})
		assert.False(t, tee.Enabled(context.Background(), slog.LevelInfo))
	})

	t.Run("WithAttrs propagated to both sub-handlers", func(t *testing.T) {
		rb1 := NewRingBuffer(10)
		rb2 := NewRingBuffer(10)
		tee := TeeHandler(rb1.Handler(), rb2.Handler()).
			WithAttrs([]slog.Attr{slog.String("component", "tee-comp")})

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		require.NoError(t, tee.Handle(context.Background(), record))

		r1 := rb1.Recent(1)
		r2 := rb2.Recent(1)
		require.Len(t, r1, 1)
		require.Len(t, r2, 1)
		assert.Equal(t, "tee-comp", r1[0].Component)
		assert.Equal(t, "tee-comp", r2[0].Component)
	})

	t.Run("WithGroup propagated to both sub-handlers", func(t *testing.T) {
		primary := &mockHandler{enabled: true}
		secondary := &mockHandler{enabled: true}
		h2 := TeeHandler(primary, secondary).WithGroup("mygroup")

		assert.NotNil(t, h2)
		th, ok := h2.(*teeHandler)
		require.True(t, ok, "WithGroup must return a *teeHandler")

		ph, ok := th.primary.(*mockHandler)
		require.True(t, ok, "primary must remain a *mockHandler after WithGroup")
		assert.Equal(t, "mygroup", ph.group)

		sh, ok := th.secondary.(*mockHandler)
		require.True(t, ok, "secondary must remain a *mockHandler after WithGroup")
		assert.Equal(t, "mygroup", sh.group)
	})

	t.Run("primary error returned; secondary error does not overwrite it", func(t *testing.T) {
		primary := &mockHandler{enabled: true, handleErr: fmt.Errorf("primary error")}
		secondary := &mockHandler{enabled: true, handleErr: fmt.Errorf("secondary error")}
		tee := TeeHandler(primary, secondary)

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		err := tee.Handle(context.Background(), record)
		assert.EqualError(t, err, "primary error")
	})

	t.Run("secondary error returned when primary succeeds", func(t *testing.T) {
		primary := &mockHandler{enabled: true}
		secondary := &mockHandler{enabled: true, handleErr: fmt.Errorf("secondary error")}
		tee := TeeHandler(primary, secondary)

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		err := tee.Handle(context.Background(), record)
		assert.EqualError(t, err, "secondary error")
	})

	t.Run("no error returned when both sub-handlers succeed", func(t *testing.T) {
		primary := &mockHandler{enabled: true}
		secondary := &mockHandler{enabled: true}
		tee := TeeHandler(primary, secondary)

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
		assert.NoError(t, tee.Handle(context.Background(), record))
		assert.Equal(t, 1, primary.recordCount())
		assert.Equal(t, 1, secondary.recordCount())
	})
}

// ----------------------------------------------------------------------------
// parseLevel (unexported, accessible from the same package)
// ----------------------------------------------------------------------------

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"debug", levelDebug},
		{"DEBUG", levelDebug},
		{"info", levelInfo},
		{"INFO", levelInfo},
		{"warn", levelWarn},
		{"WARN", levelWarn},
		{"warning", levelWarn},
		{"WARNING", levelWarn},
		{"error", levelError},
		{"ERROR", levelError},
		// Unknown / empty strings fall back to levelDebug (show everything).
		{"", levelDebug},
		{"trace", levelDebug},
		{"critical", levelDebug},
		{"UNKNOWN", levelDebug},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("input=%q", tc.input), func(t *testing.T) {
			assert.Equal(t, tc.expected, parseLevel(tc.input))
		})
	}
}
