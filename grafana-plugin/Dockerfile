ARG grafana_version=13.1.0
ARG grafana_image=grafana

FROM golang:1.26.5-bookworm AS backend-build
WORKDIR /build
COPY go.mod go.sum Magefile.go ./
COPY pkg ./pkg
COPY src ./src
RUN go run github.com/magefile/mage@v1.17.2

FROM node:22-bookworm-slim AS frontend-build
WORKDIR /build
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY . .
COPY --from=backend-build /build/dist ./dist
RUN npm run build

FROM grafana/${grafana_image}:${grafana_version}

ENV GF_DEFAULT_APP_MODE=production \
    GF_AUTH_ANONYMOUS_ENABLED=false \
    GF_AUTH_BASIC_ENABLED=true

USER root
COPY --from=frontend-build --chown=grafana:root /build/dist /var/lib/grafana/plugins/harv-logs
COPY --chown=grafana:root provisioning /etc/grafana/provisioning

USER grafana
