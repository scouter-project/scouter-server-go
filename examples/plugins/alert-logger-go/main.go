// alert-logger-go is an example scouter-server plugin written in Go.
//
// It prints every AlertPack and PerfCounterPack to stderr. Build with:
//
//	go build -o alert-logger ./examples/plugins/alert-logger-go
//
// and drop the resulting binary into scouter-server's plugin_dir.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	scouterplugin "github.com/scouter-project/scouter-server-go/internal/plugin"
	pluginpb "github.com/scouter-project/scouter-server-go/internal/plugin/proto"
)

type alertHook struct{}

func (alertHook) Process(_ context.Context, p *pluginpb.AlertPayload) error {
	fmt.Fprintf(os.Stderr, "[alert] t=%s level=%d obj=%s title=%q msg=%q tags=%v\n",
		time.UnixMilli(p.Time).Format(time.RFC3339),
		p.Level, p.ObjType, p.Title, p.Message, p.Tags)
	return nil
}

type counterHook struct{}

func (counterHook) Process(_ context.Context, p *pluginpb.CounterPayload) error {
	fmt.Fprintf(os.Stderr, "[counter] t=%s obj=%s timeType=%d counters=%v\n",
		time.UnixMilli(p.Time).Format(time.RFC3339),
		p.ObjName, p.TimeType, p.Counters)
	return nil
}

func main() {
	scouterplugin.Serve(alertHook{}, counterHook{})
}
