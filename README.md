# Loxone2Velux Gateway

Bridge between [Loxone](https://www.loxone.com) home automation and the Velux
KLF-200 gateway. It exposes the KLF-200 (io-homecontrol) devices as simple
HTTP endpoints for Loxone Virtual Outputs, pushes state changes to the
Miniserver via UDP and ships a small web dashboard.

Available as a **Home Assistant add-on** (directory `loxone2velux/`) or as a
standalone Docker container.

## Features

- Control windows, roller shutters and blinds (open / close / stop / position)
- Loxone-friendly GET endpoints returning `OK` / `ERROR` / plain numbers
- UDP feedback to the Miniserver (position, state, rain, wind)
- Rain and wind sensor status derived from KLF-200 limitation notifications
- Automatic reconnect and keep-alive towards the KLF-200
- Web dashboard (served by the gateway, works behind Home Assistant Ingress)
- Optional API token authentication

## Quick start (Docker)

```bash
mkdir -p data
cp config.example.yaml data/config.yaml   # edit klf200.host / klf200.password / server.api_token
# On Linux, give the container user ownership of its writable data directory:
sudo chown -R 1000:1000 data
docker compose up -d
```

Without a config file the gateway starts with defaults and reads
`KLF200_HOST` and `KLF200_PASSWORD` from the environment.

The container stores its complete configuration in `/data/config.yaml`.
Mount the entire `/data` directory read-write: saving uses an atomic file
replacement, which cannot work with a single-file bind mount. Existing Docker
installations must move `./config.yaml` to `./data/config.yaml` and, if present,
`./loxone.yaml` to `./data/loxone.yaml`. Back up both files first. On first startup
the standalone gateway merges the legacy sidecar into the complete config and
retains it as `loxone.yaml.legacy`. Subsequent updates use only `config.yaml`.

The dashboard is then available on `http://localhost:8080/`, the API on
`/api/...` and the Loxone endpoints on `/loxone/...`.

## Home Assistant add-on

Add this repository as an add-on repository in Home Assistant and install
"Loxone2Velux Gateway". Configuration, endpoints and the UDP feedback are
described in [`loxone2velux/DOCS.md`](loxone2velux/DOCS.md).

## API overview

| Method | Path                                | Description                     |
| ------ | ----------------------------------- | ------------------------------- |
| GET    | `/health`                           | Status, no auth                 |
| GET    | `/api/nodes`                        | All devices                     |
| POST   | `/api/nodes/{id}/position`          | `{"position": 0-100}`           |
| POST   | `/api/nodes/{id}/open|close|stop`   | Commands                        |
| GET    | `/api/sensors`                      | Rain / wind status              |
| POST   | `/api/sensors/refresh`              | Query KLF-200 limitation status |
| GET    | `/api/mappings`                     | Node → Loxone-ID mappings       |
| POST/PUT/DELETE | `/api/mappings[/{id}]`     | Manage mappings                 |
| GET/PUT| `/api/loxone/config[/udp]`          | UDP feedback settings           |
| GET/POST | `/api/config`                     | Gateway configuration (no secrets returned) |
| GET    | `/loxone/node/{id}/open|close|stop` | Loxone Virtual Output targets   |
| GET    | `/loxone/node/{id}/set/{pct}`       | Set position                    |
| GET    | `/loxone/node/{id}/position`        | Read position (0-100)           |
| GET    | `/loxone/sensors[/rain|/wind]`      | `rain;wind` or `0`/`1`          |

With `server.api_token` set, pass the token as `Authorization: Bearer <token>`
or `?token=<token>`. Position semantics: 0 = open, 100 = closed.

Unknown raw positions use `position_percent: -1`; node responses also include
`stale`. Sensor responses include `valid`, which is true only after a complete
refresh on a live connection and expires after twice the refresh interval.
Loxone status endpoints return HTTP 503 and `ERROR` for unknown or stale data;
UDP skips unknown position/target values. Complete refreshes resend mapped
positions and valid sensor states to repair lost UDP feedback.

In the HA add-on, `GET /api/config` reports `managed: true`. Change connection
settings, API token and log level through HA add-on options. API attempts to
change these fields are rejected with HTTP 400. Mappings and UDP settings remain
editable and are stored atomically in `loxone.yaml`; standalone mode stores all
settings in a single complete configuration file.

## Security notes

- **Set an API token.** Without one, anyone who can reach the HTTP port can
  operate the windows, including via cross-site requests from a web page.
- The TLS connection to the KLF-200 is verified against the embedded Velux
  CA. The CA certificate expired in July 2026 while devices keep using it,
  so the validity period is deliberately not enforced, only the signature chain.

## Development

```bash
go build ./... && go test -race ./...  # backend (unit + integration tests against a simulated KLF-200)
cd web && npm ci && npm test           # frontend tests (Vitest)
npm run build                          # frontend build (output in web/dist, served by the gateway)
```

Coverage: `go test -coverprofile=c.out -coverpkg=./... ./... && go tool cover -func=c.out | tail -1`
(CI requires at least 70 %) and `npm run test:coverage` in `web/`.

The package `internal/klf200/klf200test` contains a simulated KLF-200 gateway
(TLS + SLIP protocol) that the integration tests of the client, the gateway
service and the HTTP API run against.

The Go binary serves `./web/dist` when present. For frontend development run
`npm run dev` in `web/`; the Vite dev server proxies `/api` and `/health` to
`localhost:8080`.

## Repository notes

The [quality review and outstanding findings](docs/QUALITY_REVIEW.md) track
known issues and follow-up work.

Local runtime configuration (`config.yaml`, `loxone.yaml`, `.env`), test
coverage and generated frontend files are excluded from version control.

## License

MIT, see [LICENSE](LICENSE).
