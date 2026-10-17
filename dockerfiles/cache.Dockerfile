FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Unique per-service marker so each service image has a distinct digest
COPY dockerfiles/content/cache.txt /service-id.txt
