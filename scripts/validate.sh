#!/usr/bin/env bash
# Isolated validation: never starts the production Compose project or touches its volumes.
set -euo pipefail
suffix="${GITHUB_RUN_ID:-local}-$$"
network="pdf-reader-test-$suffix"
database="pdf-reader-test-db-$suffix"
extractor_image="pdf-reader-extractor-test:$suffix"
backend_image="pdf-reader-backend-test:$suffix"
frontend_image="pdf-reader-frontend-test:$suffix"
extractor="pdf-reader-test-extractor-$suffix"
backend="pdf-reader-test-backend-$suffix"
cleanup() {
  docker rm -f "$backend" "$extractor" "$database" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  docker image rm "$extractor_image" "$backend_image" "$frontend_image" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker network create "$network" >/dev/null
docker run -d --name "$database" --network "$network" -e POSTGRES_USER=pdfreader -e POSTGRES_PASSWORD=pdfreader -e POSTGRES_DB=pdfreader postgres:16-alpine >/dev/null
ready=false
for attempt in {1..30}; do
  if docker exec "$database" pg_isready -U pdfreader >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
"$ready"
docker build --target test -t "$extractor_image" extractor
docker run --rm --memory=2g --cpus=2 "$extractor_image" python -m compileall -q .
docker run --rm --memory=2g --cpus=2 "$extractor_image"
mkdir -p artifacts
docker run --rm --memory=2g --cpus=2 "$extractor_image" python tools/benchmark.py > artifacts/extraction-benchmark.json
docker build --target test -t "$backend_image" backend
docker run --rm --network "$network" -e DATABASE_URL="postgres://pdfreader:pdfreader@$database:5432/pdfreader?sslmode=disable" "$backend_image" sh -c 'go test -count=1 ./... && go vet ./...'
docker run -d --name "$extractor" --network "$network" --memory=2g --cpus=2 "$extractor_image" uvicorn main:app --host 0.0.0.0 --port 8000 >/dev/null
docker run -d --name "$backend" --network "$network" -e DATABASE_URL="postgres://pdfreader:pdfreader@$database:5432/pdfreader?sslmode=disable" -e EXTRACTOR_URL="http://$extractor:8000" -e STORAGE_DIR=/tmp/pdf-reader-data "$backend_image" /out/server >/dev/null
docker run --rm --network "$network" "$extractor_image" python tools/smoke.py "http://$backend:8080"
docker build -t "$frontend_image" frontend
docker compose config --quiet
