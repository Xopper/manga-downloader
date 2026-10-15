#!/usr/bin/env bash
# Starts a Suwayomi-Server for `manga-downloader catalog crawl-suwayomi`:
# downloads the pinned JAR once, runs it headless on 127.0.0.1:4567 with its
# data under $SUWAYOMI_HOME (default ~/.cache/manga-downloader-suwayomi), waits
# for the API and registers the keiyoushi extension store. Prints the URL.
# The nightly job does the same steps on GitHub (.github/workflows/catalog-suwayomi.yaml).
set -euo pipefail

VERSION="${SUWAYOMI_VERSION:-v2.4.2366}"
HOME_DIR="${SUWAYOMI_HOME:-$HOME/.cache/manga-downloader-suwayomi}"
PORT="${SUWAYOMI_PORT:-4567}"
PARALLEL="${SUWAYOMI_PARALLEL:-4}"
STORE="${SUWAYOMI_STORE:-https://raw.githubusercontent.com/keiyoushi/extensions/repo/index.pb}"
URL="http://127.0.0.1:$PORT"
JAR="$HOME_DIR/Suwayomi-Server-$VERSION.jar"

mkdir -p "$HOME_DIR/data"
if [ ! -f "$JAR" ]; then
  echo "downloading Suwayomi-Server $VERSION…" >&2
  curl -fsSL -o "$JAR.part" "https://github.com/Suwayomi/Suwayomi-Server/releases/download/$VERSION/Suwayomi-Server-$VERSION.jar"
  mv "$JAR.part" "$JAR"
fi

cat > "$HOME_DIR/data/server.conf" <<CONF
server.ip = "127.0.0.1"
server.port = $PORT
server.webUIEnabled = false
server.initialOpenInBrowserEnabled = false
server.systemTrayEnabled = false
server.downloadAsCbz = false
server.maxSourcesInParallel = $PARALLEL
CONF

if ! curl -fsS --max-time 2 "$URL/api/v1/source/list" >/dev/null 2>&1; then
  nohup java -Dsuwayomi.tachidesk.config.server.rootDir="$HOME_DIR/data" -jar "$JAR" >"$HOME_DIR/server.log" 2>&1 &
  echo $! > "$HOME_DIR/server.pid"
  for _ in $(seq 1 60); do
    sleep 2
    curl -fsS --max-time 2 "$URL/api/v1/source/list" >/dev/null 2>&1 && break
  done
fi
curl -fsS --max-time 2 "$URL/api/v1/source/list" >/dev/null || { echo "Suwayomi did not start; see $HOME_DIR/server.log" >&2; exit 1; }

# the keiyoushi store; adding it twice is harmless
curl -fsS -X POST "$URL/api/graphql" -H 'Content-Type: application/json' \
  -d "{\"query\":\"mutation { addExtensionStore(input:{indexUrl:\\\"$STORE\\\"}) { clientMutationId } }\"}" >/dev/null
echo "$URL"
