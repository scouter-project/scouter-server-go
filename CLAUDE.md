# Scouter Server Go - Project Rules

## Reference Java Project
- **Original Java Scouter project location**: `/Users/nhn/IdeaProjects/scouter`
- This Go project is a port/rewrite of the Java Scouter server
- When investigating bugs, implementing features, or comparing behavior, always reference the original Java source code at the above path
- Java server core code is typically under `scouter.server/src/main/java/scouter/server/`
- Java protocol/lang code is under `scouter.common/src/main/java/scouter/lang/` and `scouter.common/src/main/java/scouter/util/`

## Key Mapping: Java Classes → Go Files
- `scouter.server.core.XLogGroupPerf` → `internal/core/xlog_group_perf.go`
- `scouter.server.core.XLogGroupUtil` → `internal/core/xlog_group_util.go`
- `scouter.server.core.cache.XLogLoopCache` → `internal/core/cache/xlog_cache.go`
- `scouter.server.core.XLogCore` → `internal/core/xlog_core.go`

## Code Review Rules

### Map fields in structs must have bounded growth
When adding a `map` as a struct field, always ensure it cannot grow indefinitely:
- **Require one of**: max size cap with eviction, TTL-based cleanup, or periodic reset
- **Check**: Is there a `delete()` call or size check that prevents unbounded accumulation?
- **If no bound exists**: Add a `maxSize` constant and eviction logic before inserting new entries
- **Reference**: `TextWR.dupCheck` was a memory leak (35MB/hour) because it lacked a size cap. Fixed by adding `maxDupCheckSize = 100000` with 10% batch eviction.

### Time semantics on the wire: always epoch milliseconds
The Java Scouter protocol uses **epoch milliseconds (Long)** for every time field on the wire — `stime`, `etime`, `time`, `COMMON_TIME`, all xlog/counter timestamps. The Eclipse client also plots time axes in epoch millis (`xyGraph.primaryXAxis.setRange(stime, etime)`).
- **When reading** `stime`/`etime` from a `MapPack`: use `param.GetLong(...)`, never `GetInt(...)` (truncates the upper 32 bits and returns garbage).
- **When emitting** a `time` field in a response: it must be **epoch millis**, not seconds, not seconds-since-midnight. Storage may use a compact internal form (e.g., `RealtimeCounterData` keys by `seconds-since-midnight`), but the handler must convert back to epoch millis before responding.
- **Helper**: `handler_counter_read.go:toSecOfDayRange` converts epoch-millis → seconds-of-day for realtime counter reads. Mirror this pattern when adding new handlers that touch realtime storage.
- **Reference**: `COUNTER_PAST_TIME_GROUP` (and 3 sibling handlers) returned no data because they passed `int32(stime/1000)` (epoch seconds, ~1.7e9) into `ReadRealtimeRange`, which iterates against keys stored as seconds-since-midnight (0..86399). Even when matches existed, the returned `timeSec` was emitted as-is (seconds), so the client charted nothing because points fell outside `[stime_ms, etime_ms]` on the X axis.

### Reader/writer index staleness for today's data
`CounterWR` and `CounterRD` (and similar pairs) hold **separate** `MemHashBlock` instances backed by the same `.hfile`. The reader loads the index once when first opening the day; subsequent writes by the writer are not visible until the reader reloads.
- **Symptom**: "past data shows fine, new data doesn't appear" for today's date.
- **Pattern**: For today's date, periodically call `Reload()` on the reader's index (already implemented for `RealtimeCounterData` at 4s and `DailyCounterData` at 10s in `counter_rd.go`). Do **not** Close+Reopen — Close flushes the reader's stale buffer to disk and overwrites the writer's data.
- **When adding a new WR/RD pair** that shares a `MemHashBlock`-backed index file, mirror this Reload pattern.

## Build
- `make build` to build
- `make test` to run tests
- Output directory: `dist`
