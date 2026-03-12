#!/usr/bin/env bash
set -euo pipefail
LOG_PATH="${HOME}/Library/Logs/Claude/Claude Code.log"
mkdir -p "$(dirname "$LOG_PATH")"
: > "$LOG_PATH"

echo "[mock-claude] writing logs to $LOG_PATH"

for iteration in {1..5}; do
  echo "[$(date '+%H:%M:%S')] go test ./..." >> "$LOG_PATH"
  sleep 2.0
  if (( iteration % 2 == 0 )); then
    echo "[$(date '+%H:%M:%S')] FAIL	sakura-kodama/internal/monitor 0.${iteration}s" >> "$LOG_PATH"
    echo "[$(date '+%H:%M:%S')] exit code: 1" >> "$LOG_PATH"
  else
    echo "[$(date '+%H:%M:%S')] ok	sakura-kodama/internal/monitor" >> "$LOG_PATH"
    echo "[$(date '+%H:%M:%S')] exit code: 0" >> "$LOG_PATH"
  fi
  sleep 10.0
  echo "[$(date '+%H:%M:%S')] generate ui shell" >> "$LOG_PATH"
  sleep 2.0
  echo "[$(date '+%H:%M:%S')] lint frontend" >> "$LOG_PATH"
  sleep 2.0
  echo "[$(date '+%H:%M:%S')] panic: mock stack" >> "$LOG_PATH"
  sleep 2.0
  echo "---" >> "$LOG_PATH"
  sleep 5.0
done

echo "mock claude finished"
