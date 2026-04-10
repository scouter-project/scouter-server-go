# Scouter Server Go — Plugin System

Scouter Server Go supports **external plugins** implemented in any language
that can speak gRPC. Plugins run in their own OS process, isolating scouter
from crashes, panics, and memory leaks in third-party code.

## Hooks

| Hook | Called on | Use case |
|------|-----------|----------|
| `IAlert.Process(AlertPayload)`   | every `AlertPack` scouter processes   | forward alerts to Slack / PagerDuty / custom sinks |
| `ICounter.Process(CounterPayload)` | every `PerfCounterPack` scouter processes | route counters to Prometheus / InfluxDB / custom TSDB |

A single plugin binary may implement either or both services. Missing
services are silently ignored.

> **XLog / XLogProfile hooks are intentionally not exposed yet.** Those
> code paths run at full agent throughput and the gRPC overhead would be
> material. If you need to process XLogs, talk to us first.

## Configuration

```properties
# conf/scouter.conf
plugin_enabled = true
plugin_dir     = ./plugin
```

`plugin_dir` is scanned on startup. Every executable file directly inside
it (no subdirectories) is launched. Non-executable files and files
ending in `.alert`, `.conf`, `.md`, `.txt` are skipped — those are reserved
for alert scripts and documentation.

## Wire contract

Plugins speak the proto defined in
[`internal/plugin/proto/plugin.proto`](../internal/plugin/proto/plugin.proto)
on top of [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin).

Handshake:

| Field             | Value                          |
|-------------------|--------------------------------|
| ProtocolVersion   | `1`                            |
| MagicCookieKey    | `SCOUTER_PLUGIN_COOKIE`        |
| MagicCookieValue  | `scouter-server-go-plugin-v1`  |
| Transport         | gRPC only                       |

## Writing a plugin in Go

```go
package main

import (
    "context"
    "fmt"
    scouterplugin "github.com/scouter-project/scouter-server-go/internal/plugin"
    pluginpb "github.com/scouter-project/scouter-server-go/internal/plugin/proto"
)

type myAlert struct{}

func (myAlert) Process(_ context.Context, p *pluginpb.AlertPayload) error {
    fmt.Printf("alert: %s / %s\n", p.Title, p.Message)
    return nil
}

func main() {
    scouterplugin.Serve(myAlert{}, nil) // only IAlert
}
```

Build and install:

```bash
go build -o my-alert-plugin ./cmd/my-alert-plugin
cp my-alert-plugin $SCOUTER_HOME/plugin/
```

Working example: [`examples/plugins/alert-logger-go`](../examples/plugins/alert-logger-go).

## Writing a plugin in Python

See [`examples/plugins/alert-logger-py`](../examples/plugins/alert-logger-py)
for a minimal end-to-end example including `protoc` codegen invocation.

In short, a Python plugin must:

1. Refuse to start unless `SCOUTER_PLUGIN_COOKIE=scouter-server-go-plugin-v1`.
2. Bind a gRPC server to a loopback port.
3. Register `IAlertServicer` and/or `ICounterServicer` (and the standard
   grpc-health servicer — hashicorp/go-plugin health-checks the channel).
4. Print exactly one handshake line on stdout:
   `1|1|tcp|127.0.0.1:<port>|grpc`
5. `server.wait_for_termination()`.

## Error isolation

- Plugin RPC errors are logged (`plugin: alert hook error`) and swallowed.
- Plugin panics on the host side are caught and logged.
- A plugin crash kills only that plugin's process; scouter continues.
  **The manager does not currently auto-restart crashed plugins** — if
  a plugin dies, it stays dead until scouter restarts. Plan accordingly.
- Every `Process` call is bounded by a **2 second timeout**. Slow plugins
  will see `context deadline exceeded` errors and drop events.

## Value coercion

The proto contract uses simple types (`string`, `double`) rather than
scouter's full `Value` hierarchy. At the boundary:

| Scouter value       | → AlertPayload.tags (string) | → CounterPayload.counters (double) |
|---------------------|------------------------------|------------------------------------|
| `TextValue`         | as-is                        | skipped                            |
| `DecimalValue`/`*Value` numeric | `fmt.Sprint`                 | cast to float64                    |
| `BooleanValue`      | `"true"` / `"false"`         | `1.0` / `0.0`                      |
| everything else     | `fmt.Sprintf("%v", …)`       | skipped                            |

If you need full fidelity (lists, nested maps, blobs), ask for a richer
payload and we will version-bump the proto.
