# alert-logger-py

Example scouter-server plugin written in Python. Logs every `AlertPack`
and `PerfCounterPack` to stderr.

## Setup

```bash
cd examples/plugins/alert-logger-py
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt

# Generate gRPC stubs from the canonical proto.
python3 -m grpc_tools.protoc \
  -I ../../../internal/plugin/proto \
  --python_out=. --grpc_python_out=. \
  plugin.proto
```

## Install into scouter

```bash
chmod +x plugin.py
cp plugin.py plugin_pb2.py plugin_pb2_grpc.py $SCOUTER_HOME/plugin/
```

Make sure `plugin_enabled = true` and `plugin_dir = ./plugin` (or wherever
you copied the files) in `conf/scouter.conf`, then restart scouter.

## Notes

- The plugin reads the magic cookie from the `SCOUTER_PLUGIN_COOKIE`
  environment variable, so you cannot run it directly — scouter sets it.
- Any exception raised in `Process` becomes an error on the scouter side
  and is logged but does **not** stop alert/counter processing.
- Keep RPC handlers fast: scouter enforces a 2 s per-call timeout.
