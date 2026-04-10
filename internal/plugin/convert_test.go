package plugin

import (
	"testing"

	"github.com/scouter-project/scouter-server-go/internal/protocol/pack"
	"github.com/scouter-project/scouter-server-go/internal/protocol/value"
)

func TestAlertToProto_FullFields(t *testing.T) {
	tags := value.NewMapValue()
	tags.Put("service", value.NewTextValue("api"))
	tags.Put("count", value.NewDecimalValue(42))
	tags.Put("ratio", &value.DoubleValue{Value: 0.75})

	ap := &pack.AlertPack{
		Time:    1700000000000,
		Level:   2,
		ObjType: "tomcat",
		ObjHash: 123,
		Title:   "CPU High",
		Message: "cpu > 90%",
		Tags:    tags,
	}

	got := alertToProto(ap)

	if got.Time != 1700000000000 {
		t.Errorf("Time = %d", got.Time)
	}
	if got.Level != 2 {
		t.Errorf("Level = %d", got.Level)
	}
	if got.ObjType != "tomcat" {
		t.Errorf("ObjType = %q", got.ObjType)
	}
	if got.ObjHash != 123 {
		t.Errorf("ObjHash = %d", got.ObjHash)
	}
	if got.Title != "CPU High" || got.Message != "cpu > 90%" {
		t.Errorf("Title/Message mismatch: %+v", got)
	}
	if got.Tags["service"] != "api" {
		t.Errorf("tags[service] = %q", got.Tags["service"])
	}
	if got.Tags["count"] != "42" {
		t.Errorf("tags[count] = %q", got.Tags["count"])
	}
	if got.Tags["ratio"] != "0.75" {
		t.Errorf("tags[ratio] = %q", got.Tags["ratio"])
	}
}

func TestAlertToProto_NilTags(t *testing.T) {
	ap := &pack.AlertPack{Time: 1, Title: "t", Message: "m"}
	got := alertToProto(ap)
	if got.Tags == nil {
		t.Fatal("Tags should be non-nil (empty map)")
	}
	if len(got.Tags) != 0 {
		t.Errorf("Tags = %v, want empty", got.Tags)
	}
}

func TestCounterToProto_NumericOnly(t *testing.T) {
	data := value.NewMapValue()
	data.Put("cpu", &value.DoubleValue{Value: 87.5})
	data.Put("mem", value.NewDecimalValue(2048))
	data.Put("flt", &value.FloatValue{Value: 1.5})
	data.Put("alive", &value.BooleanValue{Value: true})
	data.Put("name", value.NewTextValue("skipped"))

	cp := &pack.PerfCounterPack{
		Time:     999,
		ObjName:  "api-1",
		TimeType: 1,
		Data:     data,
	}

	got := counterToProto(cp)

	if got.ObjName != "api-1" || got.Time != 999 || got.TimeType != 1 {
		t.Errorf("header mismatch: %+v", got)
	}
	if got.Counters["cpu"] != 87.5 {
		t.Errorf("cpu = %v", got.Counters["cpu"])
	}
	if got.Counters["mem"] != 2048 {
		t.Errorf("mem = %v", got.Counters["mem"])
	}
	if got.Counters["flt"] != 1.5 {
		t.Errorf("flt = %v", got.Counters["flt"])
	}
	if got.Counters["alive"] != 1 {
		t.Errorf("alive = %v", got.Counters["alive"])
	}
	if _, ok := got.Counters["name"]; ok {
		t.Error("text-valued counter should have been skipped")
	}
}

func TestCounterToProto_NilData(t *testing.T) {
	cp := &pack.PerfCounterPack{ObjName: "x"}
	got := counterToProto(cp)
	if got.Counters == nil {
		t.Fatal("Counters should be non-nil")
	}
	if len(got.Counters) != 0 {
		t.Errorf("Counters = %v", got.Counters)
	}
}
