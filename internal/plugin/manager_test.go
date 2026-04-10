package plugin

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pluginpb "github.com/scouter-project/scouter-server-go/internal/plugin/proto"
	"github.com/scouter-project/scouter-server-go/internal/protocol/pack"
	"github.com/scouter-project/scouter-server-go/internal/protocol/value"
)

// fakeAlertHook captures every AlertPayload it sees.
type fakeAlertHook struct {
	mu     sync.Mutex
	seen   []*pluginpb.AlertPayload
	err    error
	panics bool
	delay  time.Duration
}

func (f *fakeAlertHook) Process(ctx context.Context, p *pluginpb.AlertPayload) error {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.panics {
		panic("boom")
	}
	f.mu.Lock()
	f.seen = append(f.seen, p)
	f.mu.Unlock()
	return f.err
}

type fakeCounterHook struct {
	calls atomic.Int32
}

func (f *fakeCounterHook) Process(_ context.Context, _ *pluginpb.CounterPayload) error {
	f.calls.Add(1)
	return nil
}

// newTestManager builds a Manager bypassing subprocess launch so the
// dispatch logic can be exercised with in-process fake hooks.
func newTestManager(alert AlertHook, counter CounterHook) *Manager {
	m := &Manager{
		dir:     "(test)",
		enabled: true,
		timeout: 500 * time.Millisecond,
	}
	m.plugins = append(m.plugins, &loaded{
		name:    "fake",
		alert:   alert,
		counter: counter,
	})
	return m
}

func TestDispatchAlert_Happy(t *testing.T) {
	hook := &fakeAlertHook{}
	m := newTestManager(hook, nil)

	m.DispatchAlert(&pack.AlertPack{
		Time:    1,
		Title:   "t",
		Message: "m",
		Tags:    value.NewMapValue(),
	})

	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.seen) != 1 {
		t.Fatalf("expected 1 call, got %d", len(hook.seen))
	}
	if hook.seen[0].Title != "t" {
		t.Errorf("Title = %q", hook.seen[0].Title)
	}
}

func TestDispatchAlert_DisabledIsNoop(t *testing.T) {
	hook := &fakeAlertHook{}
	m := newTestManager(hook, nil)
	m.enabled = false

	m.DispatchAlert(&pack.AlertPack{Title: "t"})

	if len(hook.seen) != 0 {
		t.Error("disabled manager should not dispatch")
	}
}

func TestDispatchAlert_NilPackIsNoop(t *testing.T) {
	hook := &fakeAlertHook{}
	m := newTestManager(hook, nil)
	m.DispatchAlert(nil) // must not panic
	if len(hook.seen) != 0 {
		t.Error("nil pack should not be forwarded")
	}
}

func TestDispatchAlert_HookErrorIsSwallowed(t *testing.T) {
	hook := &fakeAlertHook{err: errors.New("sink unavailable")}
	m := newTestManager(hook, nil)
	// Must not panic, must not propagate.
	m.DispatchAlert(&pack.AlertPack{Title: "t"})
}

func TestDispatchAlert_HookPanicIsSwallowed(t *testing.T) {
	hook := &fakeAlertHook{panics: true}
	m := newTestManager(hook, nil)
	// Must not panic out of the manager.
	m.DispatchAlert(&pack.AlertPack{Title: "t"})
}

func TestDispatchAlert_TimeoutCancelsCall(t *testing.T) {
	hook := &fakeAlertHook{delay: 2 * time.Second} // longer than manager timeout
	m := newTestManager(hook, nil)
	m.timeout = 50 * time.Millisecond

	start := time.Now()
	m.DispatchAlert(&pack.AlertPack{Title: "t"})
	elapsed := time.Since(start)

	// Should return well before the hook's 2s delay elapses.
	if elapsed > 500*time.Millisecond {
		t.Errorf("DispatchAlert took %v, timeout did not fire", elapsed)
	}
}

func TestDispatchAlert_SkippedWhenNoAlertHook(t *testing.T) {
	// counter-only plugin; alert dispatch should be a silent skip.
	ch := &fakeCounterHook{}
	m := newTestManager(nil, ch)
	m.DispatchAlert(&pack.AlertPack{Title: "t"}) // must not panic
}

func TestDispatchCounter_Happy(t *testing.T) {
	ch := &fakeCounterHook{}
	m := newTestManager(nil, ch)

	data := value.NewMapValue()
	data.Put("cpu", &value.DoubleValue{Value: 1.0})

	m.DispatchCounter(&pack.PerfCounterPack{
		ObjName:  "api",
		TimeType: 1,
		Data:     data,
	})

	if ch.calls.Load() != 1 {
		t.Errorf("expected 1 counter call, got %d", ch.calls.Load())
	}
}

func TestDispatchCounter_FanOutToMultiplePlugins(t *testing.T) {
	a := &fakeCounterHook{}
	b := &fakeCounterHook{}
	m := newTestManager(nil, a)
	m.plugins = append(m.plugins, &loaded{name: "second", counter: b})

	m.DispatchCounter(&pack.PerfCounterPack{ObjName: "x", Data: value.NewMapValue()})

	if a.calls.Load() != 1 || b.calls.Load() != 1 {
		t.Errorf("fan-out failed: a=%d b=%d", a.calls.Load(), b.calls.Load())
	}
}

func TestNewManager_EmptyDirDisables(t *testing.T) {
	m := NewManager("", true)
	if m.enabled {
		t.Error("empty dir should disable manager regardless of enabled flag")
	}
}

func TestStop_Idempotent(t *testing.T) {
	m := newTestManager(&fakeAlertHook{}, nil)
	// First Stop should clear; second must not panic.
	m.Stop()
	m.Stop()
	if len(m.plugins) != 0 {
		t.Errorf("plugins not cleared: %d", len(m.plugins))
	}
}
