# M17 Web Client

This project provides a Go backend for accessing the [M17](https://m17project.org/) voice network. The backend exposes a WebSocket interface for audio and control traffic and serves a small HTTP API for use with a web UI.

## Features

- WebSocket gateway for streaming and controlling M17 traffic
- Reflector and module discovery from a JSON host file
- Prometheus metrics and health check endpoints
- Configurable CORS rules, timeouts and session limits

## Requirements

To compile the backend:

- Go 1.23 or later
- A C compiler with cgo support (for example, `gcc`)
- The `libcodec2` development library and headers

## Building

```bash
go build -o m17-webclient ./cmd/server
```

The binary listens on `:8090` by default. Change the address with `LISTEN_ADDR` and `LISTEN_PORT`.

## Configuration

### Network and Server
- `SERVER_NAME` – name advertised to clients in the WebSocket welcome message (no default)
- `LISTEN_ADDR`, `LISTEN_PORT` – bind address and port (default `:8090` on all interfaces)
- `MAX_SESSIONS` – limit concurrent WebSocket sessions (default `0`, unlimited)

### Logging
- `LOG_LEVEL` – `debug`, `info`, `warn`, `error` (default `info`)
- `LOG_FORMAT` – `text` or `json` (default `text`)

### Reflector Host File
- `M17_HOSTFILE` – path to a JSON host file used to populate the reflector list (no default; if unset, the reflector list is empty). The file is reloaded every minute.

The host file must contain a JSON object with a `reflectors` array. Each entry provides details about a reflector that can be offered to clients. A minimal example:

```json
{
  "reflectors": [
    {
      "designator": "M17-AWV",
      "domain": "m17-awv.kc1awv.net",
      "ipv4": "",
      "ipv6": "",
      "legacy": true,
      "modules": "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
      "name": "KC1AWV Test Reflector",
      "port": 17000,
      "source": "dvref.com",
      "special_modules": "",
      "url": "",
      "version": ""
    }
  ]
}
```

Important fields in each reflector object:

- `designator` – short identifier such as `M17-XXX`
- `name` – human-readable name
- `ipv4`, `ipv6`, or `domain` – address of the reflector
- `modules` – string listing supported modules (e.g. `ABCD`)
- `port` – UDP port for M17 traffic
- `legacy` – whether the reflector uses the legacy protocol

### CORS
- `ALLOWED_ORIGINS` – comma separated list of allowed origins (default none; only same‑origin requests allowed)
- `ALLOWED_HEADERS` – extra headers appended to `Access-Control-Allow-Headers` (default `Content-Type` only)
- `ALLOWED_METHODS` – extra methods appended to `Access-Control-Allow-Methods` (default `GET`, `POST`, `OPTIONS`)

### HTTP Server Timeouts
- `SERVER_READ_TIMEOUT` – max duration for reading a request (default `15s`)
- `SERVER_WRITE_TIMEOUT` – max duration before timing out writes (default `15s`)
- `SERVER_IDLE_TIMEOUT` – wait time for the next request when keep-alives are enabled (default `60s`)

### WebSocket
- `WS_PING_INTERVAL` – how often ping frames are sent (default `30s`)
- `WS_PONG_WAIT` – time to wait for a pong before closing the connection (default `60s`)

## HTTP API

| Endpoint | Description |
|----------|-------------|
| `GET /api/health` | Health probe returning `{ "status": "ok" }` |
| `GET /api/reflectors` | List of reflectors loaded from the host file |
| `GET /api/reflectors/modules?slug=<slug>` | Available modules for a reflector |
| `GET /metrics` | Prometheus metrics in text format |
| `GET /ws` | WebSocket entry point for the client |

### Metrics

The following Prometheus metrics are exported:

- `m17_sessions_started_total`
- `m17_sessions_ended_total`
- `m17_ptt_events_total`
- `m17_heartbeat_total`
- `m17_sessions_active`
- `m17_audio_frames_dropped_total`

## Deployment

### systemd
A sample service file `m17-webclient.service` is provided:

```bash
sudo cp m17-webclient.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now m17-webclient
```

### Reverse Proxy
The provided `nginx.conf` demonstrates proxying `/api` and `/ws` to the backend on `localhost:8090`. Any reverse proxy that supports WebSockets can be used.

### Developing a Web UI
To build a web (browser-based) interface:

1. Use the HTTP API under `/api` to discover reflectors and modules.
2. Open a WebSocket to `/ws`. The server replies with a `welcome` message containing a `session_id` and server name.
3. Exchange JSON control messages with a `type` field:
  - `join` – `{ "type": "join", "data": { "callsign": "N0CALL", "reflector": "M17-TEST", "module": "A" } }`
  - `ptt` – `{ "type": "ptt", "data": { "active": true } }` to start or stop transmission.
  - `format` – `{ "type": "format", "data": { "audio": "pcm" | "g711" } }` to choose the audio encoding.
  - `disconnect` – close the session when finished.

Audio is sent and received as binary WebSocket frames using the configured format.

Server responses such as `joined`, `rx`, `ptt`, `format`, `error`, and `disconnected` inform the client of state changes. Clients should also handle standard WebSocket ping/pong frames.

## Allowed Origins

Requests are checked against the `Origin` header.  Only same‑origin requests are allowed unless `ALLOWED_ORIGINS` is set. Wildcards may be used with a leading `*` (e.g. `https://*.example.com`). A single `*` permits any origin.

## Allowed Headers

By default only the `Content-Type` header is allowed in CORS requests. Set `ALLOWED_HEADERS` to a comma separated list to permit additional headers, for example `ALLOWED_HEADERS="Authorization,X-Custom"`.

## Allowed Methods

Cross-origin requests are limited to `GET`, `POST`, and `OPTIONS` unless more are specified. Use `ALLOWED_METHODS` to append additional HTTP methods, such as `ALLOWED_METHODS="PUT,DELETE"`.

## WebSocket Keep Alive

Ping frames are sent to each client at the configured interval. If a pong is not received within `WS_PONG_WAIT`, the connection is closed.

## License
This project is licensed under the terms of the MIT license.