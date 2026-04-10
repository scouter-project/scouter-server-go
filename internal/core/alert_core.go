package core

import (
	"log/slog"
	"net"
	"time"

	"github.com/scouter-project/scouter-server-go/internal/core/cache"
	"github.com/scouter-project/scouter-server-go/internal/db/alert"
	"github.com/scouter-project/scouter-server-go/internal/protocol"
	"github.com/scouter-project/scouter-server-go/internal/protocol/pack"
)

// AlertPluginDispatcher is the minimal surface AlertCore needs from the
// plugin subsystem. Kept as an interface so the core package does not
// depend on internal/plugin (which drags in grpc + hashicorp/go-plugin).
type AlertPluginDispatcher interface {
	DispatchAlert(ap *pack.AlertPack)
}

// AlertCore processes incoming AlertPack data.
type AlertCore struct {
	queue      chan *pack.AlertPack
	alertWR    *alert.AlertWR
	alertCache *cache.AlertCache
	plugins    AlertPluginDispatcher
}

func NewAlertCore(alertWR *alert.AlertWR, alertCache *cache.AlertCache) *AlertCore {
	ac := &AlertCore{
		queue:      make(chan *pack.AlertPack, 1024),
		alertWR:    alertWR,
		alertCache: alertCache,
	}
	go ac.run()
	return ac
}

// SetPluginDispatcher attaches a plugin dispatcher. Safe to call once at
// startup before agents connect. Passing nil disables plugin fan-out.
func (ac *AlertCore) SetPluginDispatcher(d AlertPluginDispatcher) {
	ac.plugins = d
}

func (ac *AlertCore) Handler() PackHandler {
	return func(p pack.Pack, addr *net.UDPAddr) {
		ap, ok := p.(*pack.AlertPack)
		if !ok {
			return
		}
		if ap.Time == 0 {
			ap.Time = time.Now().UnixMilli()
		}
		ac.Add(ap)
	}
}

// Add enqueues an AlertPack for processing (usable by AgentManager too).
func (ac *AlertCore) Add(ap *pack.AlertPack) {
	select {
	case ac.queue <- ap:
	default:
		slog.Warn("AlertCore queue overflow")
	}
}

func (ac *AlertCore) run() {
	for ap := range ac.queue {
		slog.Debug("AlertCore processing",
			"objHash", ap.ObjHash,
			"title", ap.Title)

		o := protocol.NewDataOutputX()
		pack.WritePack(o, ap)
		data := o.ToByteArray()

		// Add to real-time cache for ALERT_REAL_TIME delivery
		if ac.alertCache != nil {
			ac.alertCache.Add(data)
		}

		// Persist to disk
		if ac.alertWR != nil {
			ac.alertWR.Add(&alert.AlertEntry{
				TimeMs: ap.Time,
				Data:   data,
			})
		}

		// Fan out to external plugins (IAlert hooks).
		if ac.plugins != nil {
			ac.plugins.DispatchAlert(ap)
		}
	}
}
