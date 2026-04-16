package core

import (
	"log/slog"
	"net"
	"time"

	"github.com/scouter-project/scouter-server-go/internal/core/cache"
	"github.com/scouter-project/scouter-server-go/internal/db/counter"
	"github.com/scouter-project/scouter-server-go/internal/protocol/pack"
	"github.com/scouter-project/scouter-server-go/internal/protocol/value"
	"github.com/scouter-project/scouter-server-go/internal/util"
)

// Use cache.TimeTypeRealtime instead of local constant.

// PerfCountCore processes incoming PerfCounterPack data.
type PerfCountCore struct {
	counterCache   *cache.CounterCache
	counterWR      *counter.CounterWR
	auto5mSampling *Auto5MSampling
	queue          chan *pack.PerfCounterPack
}

func NewPerfCountCore(counterCache *cache.CounterCache, counterWR *counter.CounterWR) *PerfCountCore {
	pc := &PerfCountCore{
		counterCache:   counterCache,
		counterWR:      counterWR,
		auto5mSampling: NewAuto5MSampling(counterWR),
		queue:          make(chan *pack.PerfCounterPack, 4096),
	}
	go pc.run()
	return pc
}

// Auto5MSampling returns the auto 5-minute sampling instance for starting its goroutine.
func (pc *PerfCountCore) Auto5MSampling() *Auto5MSampling {
	return pc.auto5mSampling
}

func (pc *PerfCountCore) Handler() PackHandler {
	return func(p pack.Pack, addr *net.UDPAddr) {
		cp, ok := p.(*pack.PerfCounterPack)
		if !ok {
			return
		}
		if cp.Time == 0 {
			cp.Time = time.Now().UnixMilli()
		}
		select {
		case pc.queue <- cp:
		default:
			slog.Warn("PerfCountCore queue overflow")
		}
	}
}

func (pc *PerfCountCore) run() {
	for cp := range pc.queue {
		objHash := util.HashString(cp.ObjName)

		// Cache each counter value
		for _, entry := range cp.Data.Entries {
			key := cache.CounterKey{
				ObjHash:  objHash,
				Counter:  entry.Key,
				TimeType: cp.TimeType,
			}
			pc.counterCache.Put(key, entry.Value)
		}

		slog.Debug("PerfCountCore processing",
			"objName", cp.ObjName,
			"objHash", objHash,
			"counters", cp.Data.Size())

		if pc.counterWR == nil {
			continue
		}

		if cp.TimeType == cache.TimeTypeRealtime {
			// Write to realtime storage
			counters := make(map[string]value.Value)
			for _, entry := range cp.Data.Entries {
				counters[entry.Key] = entry.Value
			}
			pc.counterWR.AddRealtimeFromPerfCounter(cp.Time, objHash, counters)

			// Feed into Auto5MSampling for periodic daily aggregation
			for _, entry := range cp.Data.Entries {
				key := cache.CounterKey{
					ObjHash:  objHash,
					Counter:  entry.Key,
					TimeType: cache.TimeTypeRealtime,
				}
				pc.auto5mSampling.Add(key, entry.Value)
			}
		} else {
			// FIVE_MIN or other non-realtime: write directly to daily storage
			t := time.UnixMilli(cp.Time)
			date := t.Format("20060102")
			hhmm := t.Hour()*100 + t.Minute()
			bucket := counter.HHMMToBucket(hhmm)

			for _, entry := range cp.Data.Entries {
				f64, ok := value.ToFloat64(entry.Value)
				if !ok {
					continue
				}
				pc.counterWR.AddDaily(&counter.DailyEntry{
					Date:        date,
					ObjHash:     objHash,
					CounterName: entry.Key,
					Bucket:      bucket,
					Value:       f64,
				})

				// Mark this key so Auto5MSampling skips it
				key := cache.CounterKey{
					ObjHash:  objHash,
					Counter:  entry.Key,
					TimeType: cp.TimeType,
				}
				pc.auto5mSampling.Add(key, entry.Value)
			}
		}
	}
}
