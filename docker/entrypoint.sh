#!/bin/sh
# Runs the Go API and the SvelteKit SSR server; if either dies, the other is
# stopped and the container exits so the restart policy can take over.
set -u

bifrost &
API_PID=$!

node /app/build &
WEB_PID=$!

shutdown() {
  kill -TERM "$API_PID" "$WEB_PID" 2>/dev/null || true
}
trap shutdown TERM INT

EXIT_CODE=0
while kill -0 "$API_PID" 2>/dev/null && kill -0 "$WEB_PID" 2>/dev/null; do
  sleep 1
done

if ! kill -0 "$API_PID" 2>/dev/null; then
  wait "$API_PID" || EXIT_CODE=$?
fi
if ! kill -0 "$WEB_PID" 2>/dev/null; then
  wait "$WEB_PID" || EXIT_CODE=$?
fi

shutdown
wait "$API_PID" "$WEB_PID" 2>/dev/null || true
exit "$EXIT_CODE"
