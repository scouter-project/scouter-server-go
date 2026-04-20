package cache

import (
	"sync"

	"github.com/scouter-project/scouter-server-go/internal/protocol/value"
)

// TimeType constants matching Java's TimeTypeEnum.
const (
	TimeTypeRealtime byte = 1
	TimeTypeOneMin   byte = 2
	TimeTypeFiveMin  byte = 3
)

// CounterKey identifies a specific counter for an object.
type CounterKey struct {
	ObjHash  int32
	Counter  string
	TimeType byte
}

// counterSubKey is a secondary index key for per-object counter lookup.
type counterSubKey struct {
	Counter  string
	TimeType byte
}

// CounterCache stores the latest counter values per object.
type CounterCache struct {
	mu    sync.RWMutex
	store map[CounterKey]value.Value
	byObj map[int32]map[counterSubKey]value.Value // secondary index for O(1) per-object lookup
}

func NewCounterCache() *CounterCache {
	return &CounterCache{
		store: make(map[CounterKey]value.Value),
		byObj: make(map[int32]map[counterSubKey]value.Value),
	}
}

func (c *CounterCache) Put(key CounterKey, v value.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = v

	sub, ok := c.byObj[key.ObjHash]
	if !ok {
		sub = make(map[counterSubKey]value.Value)
		c.byObj[key.ObjHash] = sub
	}
	sub[counterSubKey{Counter: key.Counter, TimeType: key.TimeType}] = v
}

func (c *CounterCache) Get(key CounterKey) (value.Value, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.store[key]
	return v, ok
}

// GetByObjHash returns all counter values for a given object hash.
func (c *CounterCache) GetByObjHash(objHash int32) map[string]value.Value {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sub, ok := c.byObj[objHash]
	if !ok {
		return nil
	}
	result := make(map[string]value.Value, len(sub))
	for k, v := range sub {
		result[k.Counter] = v
	}
	return result
}