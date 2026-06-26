#!/bin/sh
# Map environment variables to wa-server flags so the container is configured
# the Docker way (env) while the binary keeps its plain flag interface.
set -e
exec wa-server \
  -addr "${WA_ADDR:-:8080}" \
  -apikey "${WA_APIKEY:-}" \
  -dir "${WA_DIR:-/data/instances}"
