package counter

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/scouter-project/scouter-server-go/internal/protocol/value"
)

// CounterRD reads both realtime and daily counter data.
type CounterRD struct {
	mu             sync.Mutex
	baseDir        string
	realtimeDays   map[string]*RealtimeCounterData
	dailyDays      map[string]*DailyCounterData
	realtimeOpenAt map[string]time.Time // tracks when each realtime data was opened
	dailyOpenAt    map[string]time.Time // tracks when each daily data was opened
}

const (
	dailyRefreshInterval    = 10 * time.Second
	realtimeRefreshInterval = 4 * time.Second
)

func NewCounterRD(baseDir string) *CounterRD {
	return &CounterRD{
		baseDir:        baseDir,
		realtimeDays:   make(map[string]*RealtimeCounterData),
		dailyDays:      make(map[string]*DailyCounterData),
		realtimeOpenAt: make(map[string]time.Time),
		dailyOpenAt:    make(map[string]time.Time),
	}
}

// ReadRealtime retrieves counter values for an object at a specific second.
func (r *CounterRD) ReadRealtime(date string, objHash int32, timeSec int32) (map[string]value.Value, error) {
	data, err := r.getRealtimeData(date)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return data.Read(objHash, timeSec)
}

// ReadRealtimeRange reads all realtime entries for an object in a time range.
func (r *CounterRD) ReadRealtimeRange(date string, objHash int32, startSec, endSec int32,
	handler func(timeSec int32, counters map[string]value.Value)) error {
	data, err := r.getRealtimeData(date)
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}
	return data.ReadRange(objHash, startSec, endSec, handler)
}

// ReadDaily retrieves the value at a specific 5-minute bucket.
func (r *CounterRD) ReadDaily(date string, objHash int32, counterName string, bucket int) (float64, bool, error) {
	data, err := r.getDailyData(date)
	if err != nil {
		return 0, false, err
	}
	if data == nil {
		return 0, false, nil
	}
	return data.Read(objHash, counterName, bucket)
}

// ReadDailyAll retrieves all 288 bucket values for a counter key.
func (r *CounterRD) ReadDailyAll(date string, objHash int32, counterName string) ([]float64, error) {
	data, err := r.getDailyData(date)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return data.ReadAll(objHash, counterName)
}

func (r *CounterRD) getRealtimeData(date string) (*RealtimeCounterData, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	today := now.Format("20060102")

	if d, ok := r.realtimeDays[date]; ok {
		// For today's date, periodically reload the in-memory index from disk
		// because CounterWR has its own MemHashBlock that receives new writes
		// and flushes to the same .hfile. Without reloading, the RD's index
		// becomes stale and never sees newly written per-second entries —
		// matching the "past data shows but new data doesn't" symptom.
		if date == today && now.Sub(r.realtimeOpenAt[date]) > realtimeRefreshInterval {
			d.Reload()
			r.realtimeOpenAt[date] = now
		}
		return d, nil
	}

	dir := filepath.Join(r.baseDir, date, "counter")
	if _, err := os.Stat(filepath.Join(dir, "real.data")); os.IsNotExist(err) {
		return nil, nil
	}

	d, err := NewRealtimeCounterData(dir)
	if err != nil {
		return nil, err
	}
	r.realtimeDays[date] = d
	r.realtimeOpenAt[date] = now
	return d, nil
}

func (r *CounterRD) getDailyData(date string) (*DailyCounterData, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	today := now.Format("20060102")

	if d, ok := r.dailyDays[date]; ok {
		// For today's date, periodically reload the in-memory index from disk
		// because CounterWR has its own MemHashBlock that receives new writes
		// and flushes to the same .hfile. Without reloading, the RD's index
		// becomes stale and can't find newly written entries.
		// Note: we must NOT close+reopen because Close() would flush the RD's
		// stale buffer to disk, overwriting the WR's current data.
		if date == today && now.Sub(r.dailyOpenAt[date]) > dailyRefreshInterval {
			d.Reload()
			r.dailyOpenAt[date] = now
		}
		return d, nil
	}

	dir := filepath.Join(r.baseDir, date, "counter")
	if _, err := os.Stat(filepath.Join(dir, "5m.data")); os.IsNotExist(err) {
		return nil, nil
	}

	d, err := NewDailyCounterData(dir)
	if err != nil {
		return nil, err
	}
	r.dailyDays[date] = d
	r.dailyOpenAt[date] = now
	return d, nil
}

// PurgeOldDays closes day containers not in the keepDates set.
func (r *CounterRD) PurgeOldDays(keepDates map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for date, d := range r.realtimeDays {
		if keepDates[date] {
			continue
		}
		d.Close()
		delete(r.realtimeDays, date)
		delete(r.realtimeOpenAt, date)
	}
	for date, d := range r.dailyDays {
		if keepDates[date] {
			continue
		}
		d.Close()
		delete(r.dailyDays, date)
		delete(r.dailyOpenAt, date)
	}
}

// Close closes all open data files.
func (r *CounterRD) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.realtimeDays {
		d.Close()
	}
	for _, d := range r.dailyDays {
		d.Close()
	}
}
