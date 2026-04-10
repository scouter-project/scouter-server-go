// send-test-udp is a tiny harness that fires a sample AlertPack and
// PerfCounterPack at a running scouter-server over UDP. Use it to verify
// the plugin system end-to-end:
//
//	go run ./examples/plugins/send-test-udp -port 6100
//
// Flags:
//
//	-host   scouter UDP host (default 127.0.0.1)
//	-port   scouter UDP port (default 6100)
//	-which  alert|counter|both (default both)
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/scouter-project/scouter-server-go/internal/protocol"
	"github.com/scouter-project/scouter-server-go/internal/protocol/pack"
	"github.com/scouter-project/scouter-server-go/internal/protocol/value"
)

func main() {
	host := flag.String("host", "127.0.0.1", "scouter UDP host")
	port := flag.Int("port", 6100, "scouter UDP port")
	which := flag.String("which", "both", "alert | counter | both")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *host, *port))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()

	if *which == "alert" || *which == "both" {
		sendAlert(conn)
	}
	if *which == "counter" || *which == "both" {
		sendCounter(conn)
	}
	fmt.Println("ok — sent to", addr)
}

func sendAlert(conn *net.UDPConn) {
	tags := value.NewMapValue()
	tags.Put("service", value.NewTextValue("demo-api"))
	tags.Put("region", value.NewTextValue("kr-central"))

	ap := &pack.AlertPack{
		Time:    time.Now().UnixMilli(),
		Level:   2, // ERROR
		ObjType: "tomcat",
		ObjHash: 42,
		Title:   "plugin-smoke-test",
		Message: "if you see this in the plugin log, it works",
		Tags:    tags,
	}
	writeOne(conn, ap)
}

func sendCounter(conn *net.UDPConn) {
	data := value.NewMapValue()
	data.Put("cpu", &value.DoubleValue{Value: 87.5})
	data.Put("mem", value.NewDecimalValue(2048))
	data.Put("tps", &value.FloatValue{Value: 123.4})

	cp := &pack.PerfCounterPack{
		Time:     time.Now().UnixMilli(),
		ObjName:  "demo-api",
		TimeType: 1, // Realtime
		Data:     data,
	}
	writeOne(conn, cp)
}

// writeOne frames a single pack in the CAFE single-frame UDP format.
func writeOne(conn *net.UDPConn, p pack.Pack) {
	o := protocol.NewDataOutputX()
	o.WriteInt32(protocol.UDP_CAFE)
	pack.WritePack(o, p)
	if _, err := conn.Write(o.ToByteArray()); err != nil {
		fmt.Fprintln(os.Stderr, "send error:", err)
		os.Exit(1)
	}
}
