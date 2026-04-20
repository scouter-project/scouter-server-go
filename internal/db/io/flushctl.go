package io

import (
	"sync"
	"time"
)

// IFlushable represents an object that can be periodically flushed to disk.
type IFlushable interface {
	Flush()
	IsDirty() bool
	Interval() time.Duration
}

// FlushController manages periodic flushing of registered IFlushable instances.
var flushCtl = &flushController{
	items: make(map[IFlushable]time.Time),
}

type flushController struct {
	mu      sync.Mutex
	items   map[IFlushable]time.Time // value = last flush time
	started bool
}

func GetFlushController() *flushController {
	return flushCtl
}

func (fc *flushController) Register(f IFlushable) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.items[f] = time.Time{}
	if !fc.started {
		fc.started = true
		go fc.run()
	}
}

func (fc *flushController) Unregister(f IFlushable) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	delete(fc.items, f)
}

func (fc *flushController) run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		fc.mu.Lock()
		now := time.Now()
		items := make([]IFlushable, 0, len(fc.items))
		for f, lastFlush := range fc.items {
			if f.IsDirty() && now.Sub(lastFlush) >= f.Interval() {
				items = append(items, f)
			}
		}
		fc.mu.Unlock()

		for _, f := range items {
			f.Flush()
			fc.mu.Lock()
			fc.items[f] = time.Now()
			fc.mu.Unlock()
		}
	}
}