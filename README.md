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

When loading finishes, open:

- Grafana: <http://localhost:3000>
- Promtail: <http://localhost:9080>

The `load` command starts the containers automatically. The first run may take longer while Docker downloads the images.

## Make Commands

| Command | Description |
| --- | --- |
| `make help` | Display the available commands and usage. |
| `make load BUNDLE=/path/to/bundle.zip` | Remove the previous extracted bundle, extract the supplied ZIP, expand nested node ZIP files, start the stack, and ingest the logs. |
| `make build` | Build the custom Grafana image, including the Go backend and frontend plugin assets. |
| `make up` | Build and start Loki, Promtail, and Grafana, waiting for healthy services. |
| `make down` | Stop and remove the Compose containers. Loki's named data volume is retained. |
| `make logs` | Follow the logs from all services. |
| `make push IMAGE=registry.example.com/harv-logs TAG=13.1.0` | Build and push the custom Grafana image. |
| `make release TAG=13.1.0` | Create a tarball containing the Compose runtime files and Makefile. |
| `make clean` | Remove the extracted `bundle-logs/` directory. This does not remove Docker volumes. |
| `make purge` | Stop the stack and delete all Docker volumes, including Grafana and Loki data. |

To analyze another support bundle, run `make load` again with the new ZIP path. The previous extracted files are replaced and Promtail is reset to reread the new logs.

## Grafana and Log Queries

The Loki datasource is provisioned automatically. In Grafana, open **Explore**, select **Loki**, and use LogQL queries such as:

```logql
{job=~"pod-logs-.*"}
```

```logql
{level="error"}
```

```logql
{namespace="harvester-system"} |= "error"
```

Useful labels include:

- `job`: log source, such as `pod-logs-harvester`, `pod-logs-guest`, or `harvester-nodes`
- `namespace`, `pod`, `container`: Kubernetes pod log location
- `node`, `service`: node host log location
- `level`: normalized log level such as `info`, `warn`, or `error`

The Promtail pipelines normalize several formats, including JSON, logfmt, klog, Rancher-style logs, and timestamp-prefixed container logs. This makes errors easier to filter and gives the RCA application cleaner messages to analyze.

## AI-Assisted Analysis

Use the custom Grafana RCA application to review error logs, identify related events, and ask the AI assistant for troubleshooting guidance. Treat generated explanations as investigation aids: validate recommendations against the original logs, the cluster state, and the relevant Harvester or Rancher documentation before making changes.

Any model credentials, provider settings, or additional plugin configuration required by the RCA workflow must be supplied according to the custom Grafana image and its deployment configuration. They are not stored in this repository.

## Project Files

| File | Purpose |
| --- | --- |
| `Makefile` | Short commands for loading bundles and managing the stack. |
| `load-logs.sh` | Extracts bundles, expands node archives, and resets log ingestion. |
| `docker-compose.yaml` | Defines the Loki, Promtail, and Grafana services. |
| `promtail-config.yaml` | Defines log paths, parsing pipelines, timestamps, and labels. |
| `loki-config.yaml` | Configures Loki for large, historical support-bundle imports. |
| `grafana-datasources.yaml` | Provisions the Loki datasource in Grafana. |

## Troubleshooting

### Grafana opens but no logs are visible

Check that the bundle was extracted and that the services are running:

```bash
docker compose ps
docker compose logs promtail
```

If needed, reload the bundle:

```bash
make load BUNDLE=/path/to/support-bundle.zip
```

### `make load` reports that a file is missing

Use an absolute path or a path relative to this project directory, and verify that it points to a ZIP file:

```bash
ls -lh /path/to/support-bundle.zip
```

### Docker services need to be stopped

```bash
make down
```

To remove extracted logs before loading a different bundle:

```bash
make clean
```

## Publishing A Release

Build and publish the custom image to a registry accessible to users:

```bash
make push IMAGE=registry.example.com/harv-logs TAG=13.1.0
make release TAG=13.1.0
```

Users should copy `.env.example` to `.env`, set `HARV_LOGS_IMAGE` and `HARV_LOGS_TAG` to the published image, set a strong Grafana password, and run `make load BUNDLE=/path/to/support-bundle.zip`. The default image versions are pinned in `docker-compose.yaml`; update them deliberately during release testing rather than using `latest`.

## Data and Persistence

The extracted support bundle is stored locally in `bundle-logs/`. Loki and Grafana store data in named Docker volumes, so stopping the stack does not automatically remove data. Use `make purge` when a complete reset is required. Support bundles can contain sensitive cluster information; handle the extracted files, container logs, credentials, and any AI-provider requests according to your organization's security policy.