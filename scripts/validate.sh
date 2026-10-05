#!/usr/bin/env bash
# Isolated validation: never starts the production Compose project or touches its volumes.
set -euo pipefail
suffix="${GITHUB_RUN_ID:-local}-$$"
network="pdf-reader-test-$suffix"
database="pdf-reader-test-db-$suffix"
extractor_image="pdf-reader-extractor-test:$suffix"
backend_image="pdf-reader-backend-test:$suffix"
backend_runtime="pdf-reader-backend-runtime:$suffix"
extractor_runtime="pdf-reader-extractor-runtime:$suffix"
frontend_image="pdf-reader-frontend-test:$suffix"
extractor="pdf-reader-test-extractor-$suffix"
backend="pdf-reader-test-backend-$suffix"
cleanup() {
  docker rm -f "$backend" "$extractor" "$database" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  docker image rm "$extractor_image" "$backend_image" "$frontend_image" "$backend_runtime" "$extractor_runtime" >/dev/null 2>&1 || true
}
trap cleanup EXIT
mkdir -p artifacts
{ nproc; free -m; docker info --format '{{.Architecture}} cpus={{.NCPU}} memory_bytes={{.MemTotal}}'; } > artifacts/execution-resources.txt
mkdir -p artifacts/previous-extractor
# Reproducible baseline from the committed pre-hardening code; shallow Actions checkout fetches it if needed.
if ! git cat-file -e bdffb782e62d7d835c895106ba28564fbb06b1b9^{commit} 2>/dev/null; then
 git fetch --depth=1 origin bdffb782e62d7d835c895106ba28564fbb06b1b9
fi
git archive bdffb782e62d7d835c895106ba28564fbb06b1b9 extractor | tar --strip-components=1 -x -C artifacts/previous-extractor
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
docker run --rm --memory=2g --cpus=2 -v "$PWD/artifacts/previous-extractor:/previous:ro" "$extractor_image" python tools/benchmark.py --previous-dir /previous > artifacts/extraction-benchmark.json
docker run --rm --memory=2g --cpus=2 "$extractor_image" python tools/ocr_audit.py > artifacts/ocr-audit.json
docker build --target test -t "$backend_image" backend
docker run --rm --network "$network" -e DATABASE_URL="postgres://pdfreader:pdfreader@$database:5432/pdfreader?sslmode=disable" "$backend_image" sh -c 'go test -count=1 ./... && go vet ./...'
docker build --target production -t "$backend_runtime" backend
docker build --target production -t "$extractor_runtime" extractor
docker run -d --name "$extractor" --network "$network" --memory=2g --cpus=2 --pids-limit=64 --read-only --tmpfs /tmp:size=96m,mode=1777 --cap-drop=ALL --security-opt=no-new-privileges "$extractor_runtime" uvicorn main:app --host 0.0.0.0 --port 8000 >/dev/null
docker run -d --name "$backend" --network "$network" -e DATABASE_URL="postgres://pdfreader:pdfreader@$database:5432/pdfreader?sslmode=disable" -e EXTRACTOR_URL="http://$extractor:8000" -e STORAGE_DIR=/tmp/pdf-reader-data "$backend_runtime" >/dev/null
docker run --rm --network "$network" "$extractor_image" python tools/smoke.py "http://$backend:8080"
docker exec -i "$extractor" python - < extractor/tools/runtime_security.py > artifacts/runtime-security.json
docker run --rm --network "$network" "$extractor_image" python tools/concurrency.py "http://$extractor:8000" > artifacts/concurrency.json
docker build --target test -t "$frontend_image" frontend
docker run --rm "$frontend_image"
# The pre-existing lint script names ESLint, but package/config were never supplied.
# Keep its actual outcome visible; TypeScript checking is mandatory in the build.
if ! docker run --rm "$frontend_image" npm run lint > artifacts/frontend-lint.txt 2>&1; then
 echo "::warning::Existing frontend ESLint command failed; see frontend-lint.txt. TypeScript build checks passed."
fi
docker build --target production -t "$frontend_image" frontend
docker compose config --quiet
