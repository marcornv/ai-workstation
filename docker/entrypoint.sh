#!/bin/sh
# Container start-up for the Bancada app.
#  1. On the first start the workspace is empty: copy the examples into it.
#     bancada.yaml is not copied; the app creates it using the BANCADA_*
#     variables, so it already points at the other containers (evaluator, ollama).
#  2. Run the app as BANCADA_UID:BANCADA_GID (default 1000:1000) instead of root,
#     so files in ./bancada-data belong to you on the host and you can edit them.
set -e
uid="${BANCADA_UID:-1000}"
gid="${BANCADA_GID:-1000}"

if [ ! -f /workspace/bancada.yaml ]; then
  echo "First start: copying examples into the workspace"
  mkdir -p /workspace/experiments /workspace/evaluators /workspace/data
  # (busybox cp: copy file by file; "-n" never overwrites an existing file)
  cp -n /app/examples/experiments/* /workspace/experiments/
  cp -n /app/examples/evaluators/* /workspace/evaluators/
  cp -n /app/examples/data/* /workspace/data/
fi

if [ "$(id -u)" = "0" ]; then
  chown -R "$uid:$gid" /workspace
  exec su-exec "$uid:$gid" bancada -dir /workspace -no-browser
fi
exec bancada -dir /workspace -no-browser
