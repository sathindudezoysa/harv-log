# Harvester Support Bundle Log Analyzer

This project is a local analysis stack for Harvester support bundles. It extracts a support-bundle ZIP, expands nested node archives, ingests the logs into Loki, and makes them available through Grafana.

The Grafana image is built locally from the custom plugin source. It includes the RCA application and installs the Grafana LLM plugin, allowing support engineers to inspect parsed errors, search related logs, and ask an AI assistant for help diagnosing an incident.

## How It Works

1. `make load BUNDLE=...` clears the previous extracted bundle and extracts the new ZIP into `bundle-logs/`.
2. ZIP files found in `nodes` directories are extracted so node logs are available to the collector.
3. Docker Compose builds the custom Grafana image, then starts Loki, Promtail, and Grafana.
4. Promtail rereads the bundle and sends logs to Loki.
5. Promtail parses common Harvester log formats and adds labels such as `job`, `namespace`, `pod`, `container`, `node`, `service`, and `level`.
6. Grafana provides log search, error analysis, and AI-assisted support workflows.

## Prerequisites

- Docker Engine with the Compose plugin (`docker compose`)
- GNU Make
- `unzip`
- A Harvester support-bundle ZIP file
- Network access to pull the container images on the first run
- A `.env` file containing a strong `GF_SECURITY_ADMIN_PASSWORD` (copy `.env.example`)

The commands below are intended to be run from this project directory.

## Quick Start

Prepare credentials, load a support bundle, and start the stack:

```bash
cp .env.example .env
# Edit .env and set GF_SECURITY_ADMIN_PASSWORD
make load BUNDLE=/path/to/support-bundle.zip
```
# Harv-logs

Harv-logs is a local Grafana analysis stack for Harvester support bundles. It
extracts a support-bundle ZIP, expands nested node archives, ships the logs to
Loki with Promtail, and provides a Grafana app for timeline-based root-cause
analysis and AI-assisted investigation.

## Architecture

```text
Support bundle ZIP
				|
				v
scripts/load-logs.sh -> bundle-logs/ -> Promtail -> Loki
																						 |
																						 v
															Grafana + Harv-logs app
```

- **Promtail** reads extracted pod and node logs, normalizes common formats, and
	adds labels such as `namespace`, `pod`, `container`, `node`, `service`, and
	`level`.
- **Loki** stores and indexes the imported logs using the configuration in
	`monitoring/loki-config.yaml`.
- **Grafana** provisions Loki and builds the custom `harv-logs` app from
	`grafana-plugin/`.
- **Harv-logs** correlates matching events with `grafana-plugin/rules/rca-rules.yaml`,
	displays an incident timeline, and can generate an AI report through
	`grafana-llm-app`.

## Requirements

- Docker Engine with the Compose plugin (`docker compose`)
- GNU Make and `unzip`
- Node.js 22 and npm 11 for frontend development
- Go 1.26 and Mage for backend development
- A Harvester support-bundle ZIP file
- Network access to pull images and install dependencies on first use

Support bundles may contain sensitive cluster data. Keep `bundle-logs/`, Docker
volumes, credentials, and AI-provider requests within your organization's data
handling policy. Local `.env` files are ignored and must never be committed.

## Quick Start

Run these commands from the repository root:

```bash
make load BUNDLE=/absolute/path/to/support-bundle.zip
```

The command clears the previous extracted bundle, expands nested archives,
builds the local Grafana image, starts Loki, Promtail, and Grafana, and resets
Promtail so the new logs are reread. Open:

- Grafana: http://localhost:3000
- Promtail: http://localhost:9080
- Loki readiness: http://localhost:3100/ready

Set environment variables before starting Compose when needed. Common examples
are `GF_SECURITY_ADMIN_PASSWORD`, `BIND_ADDRESS`, `GRAFANA_VERSION`,
`HARV_LOGS_IMAGE`, and `HARV_LOGS_TAG`.

## Commands

| Command | Purpose |
| --- | --- |
| `make help` | Show available commands. |
| `make load BUNDLE=/path/bundle.zip` | Extract and ingest a support bundle. |
| `make build` | Build the custom Grafana image. |
| `make build-images` | Build the custom Grafana, Loki, and Promtail images. |
| `make up` | Build and start the local stack. |
| `make down` | Stop the stack and retain named volumes. |
| `make logs` | Follow logs from all services. |
| `make clean` | Remove extracted support-bundle files. |
| `make purge` | Stop the stack and remove all named volumes. |
| `make push IMAGE=name TAG=version` | Push the images used by a release. |
| `make release TAG=version` | Create a self-contained runtime tarball in `releases/`. |

## Grafana App

After opening Grafana, configure the Harv-logs app with the Loki datasource UID,
the namespace label, and an optional node label. Open **RCA**, select an incident
window, review the correlated timeline, and generate a report or ask follow-up
questions about an event. The app requires Grafana 12.3 or newer and the
`grafana-llm-app` for AI features.

In Grafana Explore, useful LogQL examples include:

```logql
{job=~"pod-logs-.*"}
{level="error"}
{namespace="harvester-system"} |= "error"
```

## Repository Layout

| Path | Purpose |
| --- | --- |
| `Makefile` | Development, stack management, image, and release commands. |
| `docker-compose.yaml` | Local stack that builds the Grafana plugin image. |
| `release/` | Published-image Compose file, release Makefile, and image Dockerfiles. |
| `scripts/load-logs.sh` | Shared bundle extraction and ingestion workflow. |
| `monitoring/` | Loki, Promtail, and Grafana datasource configuration. |
| `grafana-plugin/pkg/` | Go backend for Loki queries, rules, timeline, and AI calls. |
| `grafana-plugin/src/` | React/TypeScript Grafana app frontend. |
| `grafana-plugin/rules/` | Rule definitions used for event correlation. |
| `grafana-plugin/tests/` | Browser end-to-end tests. |

Generated output such as `grafana-plugin/dist/`, `node_modules/`, extracted
`bundle-logs/`, and release archives is intentionally excluded from version
control.

## Development

Install frontend dependencies and run the checks from `grafana-plugin/`:

```bash
npm ci
npm run typecheck
npm run lint
npm run test:ci
go test ./...
```

Use `npm run build` to build the frontend assets. The Docker build runs both the
Go backend build and the frontend build, then packages them into Grafana.

## Releases

Build and publish images, then create the runtime archive:

```bash
make build-images TAG=13.1.0 IMAGE=ghcr.io/example/harv-logs
make push TAG=13.1.0 IMAGE=ghcr.io/example/harv-logs
make release TAG=13.1.0
```

The generated archive contains the published-image Compose file, release
Makefile, shared loader script, and monitoring configuration. It does not contain
support-bundle data, credentials, source dependencies, or build output.

## Troubleshooting

- Check service state with `docker compose ps`.
- Inspect ingestion with `docker compose logs promtail`.
- Reload a bundle with `make load BUNDLE=/path/to/another-bundle.zip`.
- Reset extracted data with `make clean`; reset Docker state with `make purge`.
- Use an absolute bundle path if a relative path is not found.