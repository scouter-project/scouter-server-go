package core

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/scouter-project/scouter-server-go/internal/core/cache"
	"github.com/scouter-project/scouter-server-go/internal/db/counter"
	"github.com/scouter-project/scouter-server-go/internal/protocol/value"
	"github.com/scouter-project/scouter-server-go/internal/util"
)

// Auto5MSampling peaks one sample from realtime counter data every 5 minutes
// and writes it to daily (5-minute bucket) storage.
// This matches Java's scouter.server.core.Auto5MSampling.
type Auto5MSampling struct {
	mu          sync.Mutex
	counterMap  map[cache.CounterKey]value.Value
	fiveMinKeys map[cache.CounterKey]struct{} // keys already sent as FIVE_MIN by agent
	counterWR   *counter.CounterWR
}

func NewAuto5MSampling(counterWR *counter.CounterWR) *Auto5MSampling {
	return &Auto5MSampling{
		counterMap:  make(map[cache.CounterKey]value.Value),
		fiveMinKeys: make(map[cache.CounterKey]struct{}),
		counterWR:   counterWR,
	}
}

// Add accumulates a counter value for the next 5-minute flush.
// For REALTIME type, it stores the latest value for sampling.
// For FIVE_MIN type, it marks the key so Auto5MSampling skips it (agent already sends 5m data).
func (a *Auto5MSampling) Add(key cache.CounterKey, v value.Value) {
	switch key.TimeType {
	case cache.TimeTypeRealtime:
		// Only sample numeric types (matching Java: BOOLEAN, FLOAT, DOUBLE, DECIMAL)
		switch v.ValueType() {
		case value.TYPE_BOOLEAN, value.TYPE_FLOAT, value.TYPE_DOUBLE, value.TYPE_DECIMAL:
			a.mu.Lock()
			a.counterMap[key] = v
			a.mu.Unlock()
		}
	case cache.TimeTypeFiveMin:
		a.mu.Lock()
		fmKey := cache.CounterKey{
			ObjHash:  key.ObjHash,
			Counter:  key.Counter,
			TimeType: cache.TimeTypeFiveMin,
		}
		a.fiveMinKeys[fmKey] = struct{}{}
		a.mu.Unlock()
	}
}

// Start begins the 5-minute periodic flush goroutine.
func (a *Auto5MSampling) Start(ctx context.Context) {
	go a.run(ctx)
}

func (a *Auto5MSampling) run(ctx context.Context) {
	ticker := time.NewTicker(util.MillisPerFiveMinute * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.flush()
		}
	}
}

func (a *Auto5MSampling) flush() {
	// Swap the map atomically (like Java's counterMap swap)
	a.mu.Lock()
	workMap := a.counterMap
	a.counterMap = make(map[cache.CounterKey]value.Value)
	fiveMinKeys := a.fiveMinKeys
	a.mu.Unlock()

	now := time.Now()
	date := now.Format("20060102")
	hhmm := now.Hour()*100 + now.Minute()
	bucket := counter.HHMMToBucket(hhmm)

	flushed := 0
	for key, v := range workMap {
		// Skip if agent already sends FIVE_MIN data for this counter
		fmKey := cache.CounterKey{
			ObjHash:  key.ObjHash,
			Counter:  key.Counter,
			TimeType: cache.TimeTypeFiveMin,
		}
		if _, exists := fiveMinKeys[fmKey]; exists {
			continue
		}

		f64, ok := value.ToFloat64(v)
		if !ok {
			continue
		}

		a.counterWR.AddDaily(&counter.DailyEntry{
			Date:        date,
			ObjHash:     key.ObjHash,
			CounterName: key.Counter,
			Bucket:      bucket,
			Value:       f64,
		})
		flushed++
	}

	if flushed > 0 {
		slog.Debug("Auto5MSampling flushed", "count", flushed, "date", date, "bucket", bucket)
	}
}
