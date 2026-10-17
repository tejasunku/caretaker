FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Unique per-service marker so each service image has a distinct digest
COPY dockerfiles/content/auth.txt /service-id.txt
