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

## Build
- `make build` to build
- `make test` to run tests
- Output directory: `dist`
