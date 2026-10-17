FROM node:18-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Unique per-service marker so each service image has a distinct digest
COPY dockerfiles/content/payments.txt /service-id.txt
