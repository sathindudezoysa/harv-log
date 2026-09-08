.DEFAULT_GOAL := help
.SHELL := /bin/bash

IMAGE ?= harv-logs
TAG ?= 13.1.0
LOKI_IMAGE ?= grafana/loki:3.5.0
PROMTAIL_IMAGE ?= grafana/promtail:3.5.0
RELEASE_DIR ?= releases/harv-logs-$(TAG)

.PHONY: help build build-images up down logs load image push release clean purge

# Default target
help:
	@echo "Usage:"
	@echo "  make load BUNDLE=<path-to-zip>  - Unzip bundle and load into Loki"
	@echo "  make build                      - Build the custom Grafana image"
	@echo "  make build-images              - Build all release images"
	@echo "  make up                         - Build and start the complete stack"
	@echo "  make down                       - Stop the RCA stack"
	@echo "  make logs                       - Follow logs from all services"
	@echo "  make push IMAGE=registry/name  - Push the custom Grafana image"
	@echo "  make release                    - Create a distributable runtime bundle"
	@echo "  make clean                      - Remove extracted logs"
	@echo "  make purge                      - Stop the stack and remove its volumes"

build:
	GRAFANA_VERSION=$(TAG) HARV_LOGS_IMAGE=$(IMAGE) HARV_LOGS_TAG=$(TAG) \
		docker compose build grafana

image: build

build-images: build
	docker build -f release/Dockerfile.loki -t $(IMAGE)-loki:$(TAG) .
	docker build -f release/Dockerfile.promtail -t $(IMAGE)-promtail:$(TAG) .

load:
	@if [ -z "$(BUNDLE)" ]; then \
		echo "Error: BUNDLE path is required."; \
		echo "Usage: make load BUNDLE=path/to/support-bundle.zip"; \
		exit 1; \
	fi
	bash scripts/load-logs.sh "$(BUNDLE)"

up:
	docker compose up -d --build --wait

logs:
	docker compose logs -f --tail=100

push: image
	docker push $(LOKI_IMAGE)
	docker push $(PROMTAIL_IMAGE)
	docker push $(IMAGE):$(TAG)

release:
	rm -rf "$(RELEASE_DIR)" "$(RELEASE_DIR).tar.gz"
	mkdir -p "$(RELEASE_DIR)/monitoring"
	cp release/docker-compose.yaml release/Makefile "$(RELEASE_DIR)/"
	cp -r scripts "$(RELEASE_DIR)/"
	cp monitoring/*.yaml "$(RELEASE_DIR)/monitoring/"
	tar -czf "$(RELEASE_DIR).tar.gz" -C releases "harv-logs-$(TAG)"
	rm -rf "$(RELEASE_DIR)"
	@echo "Created $(RELEASE_DIR).tar.gz"

down:
	docker compose down

clean:
	rm -rf ./bundle-logs

purge:
	docker compose down --volumes --remove-orphans