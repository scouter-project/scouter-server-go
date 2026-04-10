#!/usr/bin/env python3
"""
alert-logger-py — example scouter-server plugin written in Python.

This is a hashicorp/go-plugin compatible gRPC server. It speaks the
handshake protocol (line 1 of stdout = "1|1|tcp|host:port|grpc") then
serves the IAlert and ICounter services defined in plugin.proto.

Prerequisites:
    python3 -m pip install grpcio grpcio-tools protobuf

Generate the gRPC stubs once (re-run if plugin.proto changes):
    python3 -m grpc_tools.protoc \\
        -I ../../../internal/plugin/proto \\
        --python_out=. \\
        --grpc_python_out=. \\
        plugin.proto

Run as a scouter plugin:
    chmod +x plugin.py
    cp plugin.py $SCOUTER/plugin/
"""
from __future__ import annotations

import os
import socket
import sys
import time
from concurrent import futures

import grpc

import plugin_pb2          # generated from plugin.proto
import plugin_pb2_grpc     # generated from plugin.proto

# Handshake constants — MUST match internal/plugin/shared.go.
COOKIE_KEY   = "SCOUTER_PLUGIN_COOKIE"
COOKIE_VALUE = "scouter-server-go-plugin-v1"
CORE_PROTOCOL = 1
APP_PROTOCOL  = 1


class AlertService(plugin_pb2_grpc.IAlertServicer):
    def Process(self, req, _ctx):
        sys.stderr.write(
            f"[py-alert] t={time.strftime('%Y-%m-%dT%H:%M:%S', time.localtime(req.time/1000))}"
            f" level={req.level} obj={req.obj_type}"
            f" title={req.title!r} msg={req.message!r} tags={dict(req.tags)}\n"
        )
        sys.stderr.flush()
        return plugin_pb2.Ack()


class CounterService(plugin_pb2_grpc.ICounterServicer):
    def Process(self, req, _ctx):
        sys.stderr.write(
            f"[py-counter] t={time.strftime('%Y-%m-%dT%H:%M:%S', time.localtime(req.time/1000))}"
            f" obj={req.obj_name} timeType={req.time_type}"
            f" counters={dict(req.counters)}\n"
        )
        sys.stderr.flush()
        return plugin_pb2.Ack()


def main() -> None:
    # 1) Refuse to launch without the magic cookie — standard hashicorp
    #    go-plugin protection against accidental direct execution.
    if os.environ.get(COOKIE_KEY) != COOKIE_VALUE:
        sys.stderr.write(
            "This binary is a scouter-server plugin and is not meant to be "
            "invoked directly.\n"
        )
        sys.exit(1)

    # 2) Bind to an ephemeral loopback port.
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    host, port = sock.getsockname()
    sock.close()

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    plugin_pb2_grpc.add_IAlertServicer_to_server(AlertService(), server)
    plugin_pb2_grpc.add_ICounterServicer_to_server(CounterService(), server)

    # Register go-plugin's health and broker services so the host's
    # handshake succeeds.
    from grpc_health.v1 import health, health_pb2, health_pb2_grpc
    svc = health.HealthServicer()
    svc.set("plugin", health_pb2.HealthCheckResponse.SERVING)
    health_pb2_grpc.add_HealthServicer_to_server(svc, server)

    server.add_insecure_port(f"127.0.0.1:{port}")
    server.start()

    # 3) Emit the handshake line on stdout exactly once, then let the
    #    host read the rest from stderr. Format:
    #      CORE|APP|network|address|protocol
    sys.stdout.write(f"{CORE_PROTOCOL}|{APP_PROTOCOL}|tcp|127.0.0.1:{port}|grpc\n")
    sys.stdout.flush()

    # 4) Block until the host kills us.
    server.wait_for_termination()


if __name__ == "__main__":
    main()
