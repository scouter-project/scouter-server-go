package plugin

import (
	"fmt"

	pluginpb "github.com/scouter-project/scouter-server-go/internal/plugin/proto"
	"github.com/scouter-project/scouter-server-go/internal/protocol/pack"
	"github.com/scouter-project/scouter-server-go/internal/protocol/value"
)

// alertToProto flattens an AlertPack into the proto representation the
// plugin contract uses. Tags with complex values are stringified; scalar
// values are rendered with fmt.Sprint.
func alertToProto(ap *pack.AlertPack) *pluginpb.AlertPayload {
	payload := &pluginpb.AlertPayload{
		Time:    ap.Time,
		Level:   uint32(ap.Level),
		ObjType: ap.ObjType,
		ObjHash: ap.ObjHash,
		Title:   ap.Title,
		Message: ap.Message,
		Tags:    map[string]string{},
	}
	if ap.Tags != nil {
		for _, entry := range ap.Tags.Entries {
			payload.Tags[entry.Key] = valueToString(entry.Value)
		}
	}
	return payload
}

// counterToProto flattens a PerfCounterPack. Non-numeric counters are
// skipped — plugins that care about string-valued counters should use a
// richer hook (not yet implemented).
func counterToProto(cp *pack.PerfCounterPack) *pluginpb.CounterPayload {
	payload := &pluginpb.CounterPayload{
		Time:     cp.Time,
		ObjName:  cp.ObjName,
		TimeType: uint32(cp.TimeType),
		Counters: map[string]float64{},
	}
	if cp.Data != nil {
		for _, entry := range cp.Data.Entries {
			if f, ok := valueToFloat(entry.Value); ok {
				payload.Counters[entry.Key] = f
			}
		}
	}
	return payload
}

func valueToFloat(v value.Value) (float64, bool) {
	switch x := v.(type) {
	case *value.DecimalValue:
		return float64(x.Value), true
	case *value.DoubleValue:
		return x.Value, true
	case *value.FloatValue:
		return float64(x.Value), true
	case *value.BooleanValue:
		if x.Value {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func valueToString(v value.Value) string {
	switch x := v.(type) {
	case *value.TextValue:
		return x.Value
	case *value.DecimalValue:
		return fmt.Sprint(x.Value)
	case *value.DoubleValue:
		return fmt.Sprint(x.Value)
	case *value.FloatValue:
		return fmt.Sprint(x.Value)
	case *value.BooleanValue:
		return fmt.Sprint(x.Value)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}