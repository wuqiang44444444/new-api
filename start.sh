#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$ROOT_DIR/web"
LOG_DIR="$ROOT_DIR/logs"
# Default development ports; callers can override them explicitly.
FRONTEND_PORT="${FRONTEND_PORT:-3500}"
BACKEND_PORT="${BACKEND_PORT:-3501}"
STARTUP_TIMEOUT="${STARTUP_TIMEOUT:-300}"
FRONTEND_LOG="$LOG_DIR/frontend.log"
BACKEND_LOG="$LOG_DIR/backend.log"
FRONTEND_PID=""
BACKEND_PID=""
TAIL_PID=""

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少命令：$1" >&2
    exit 1
  fi
}

prepare_frontend() {
  if [[ ! -x "$ROOT_DIR/web/node_modules/.bin/rsbuild" ]]; then
    echo "前端依赖不存在，正在安装..."
    (
      cd "$ROOT_DIR/web"
      bun install --frozen-lockfile
    )
  fi

  if [[ ! -f "$FRONTEND_DIR/dist/index.html" ]]; then
    echo "正在创建本地开发嵌入入口..."
    mkdir -p "$FRONTEND_DIR/dist"
    printf '%s\n' \
      '<!doctype html><html><body>Development assets are served by the frontend dev server.</body></html>' \
      >"$FRONTEND_DIR/dist/index.html"
  fi
}

# Stops any process listening on the port, then waits for the port to be freed.
# Fails only when the port cannot be freed.
stop_port_occupiers() {
  local port="$1" pids pid
  pids=$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
  if [[ -z "$pids" ]]; then
    return 0
  fi
  for pid in $pids; do
    echo "端口 $port 被占用：PID ${pid}（$(ps -p "$pid" -o command= 2>/dev/null || true)），正在停止。"
  done
  # shellcheck disable=SC2086 # Intentional word splitting of the PID list.
  kill -TERM $pids 2>/dev/null || true
  local deadline=$((SECONDS + 5))
  while (( SECONDS < deadline )); do
    pids=$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
    if [[ -z "$pids" ]]; then
      return 0
    fi
    sleep 0.2
  done
  echo "占用进程未响应 SIGTERM，强制停止。"
  # shellcheck disable=SC2086 # Intentional word splitting of the PID list.
  kill -KILL $pids 2>/dev/null || true
  sleep 0.5
  if lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "端口 $port 仍被占用，无法自动释放，请手动处理。" >&2
    return 1
  fi
}

wait_for_health() {
  local name="$1" port="$2" pid="$3" log_file="$4" endpoint="$5"
  local deadline=$((SECONDS + STARTUP_TIMEOUT))
  local response
  while (( SECONDS < deadline )); do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "$name 启动失败，最近日志如下：" >&2
      tail -n 80 "$log_file" >&2 || true
      return 1
    fi
    if response=$(curl --noproxy '*' --fail --silent --max-time 1 "http://127.0.0.1:$port$endpoint"); then
      if [[ "$endpoint" != "/api/status" ]] || [[ "$response" =~ \"success\"[[:space:]]*:[[:space:]]*true ]]; then
        echo "$name 已启动：http://localhost:$port"
        return 0
      fi
    fi
    sleep 0.5
  done
  echo "$name 在 ${STARTUP_TIMEOUT} 秒内未通过健康检查，最近日志如下：" >&2
  tail -n 80 "$log_file" >&2 || true
  return 1
}

# shellcheck disable=SC2329 # Called by the EXIT trap; it clears traps before exiting.
cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  # Job control gives each launched job its own group, including go/bun children.
  # Signal only groups created by this invocation, even if a parent already died.
  local pid
  for pid in "$TAIL_PID" "$FRONTEND_PID" "$BACKEND_PID"; do
    [[ -z "$pid" ]] || kill -TERM -- "-$pid" 2>/dev/null || true
  done
  local deadline=$((SECONDS + 5)) alive
  while (( SECONDS < deadline )); do
    alive=false
    for pid in "$TAIL_PID" "$FRONTEND_PID" "$BACKEND_PID"; do
      if [[ -n "$pid" ]] && kill -0 -- "-$pid" 2>/dev/null; then alive=true; fi
    done
    if [[ "$alive" == false ]]; then break; fi
    sleep 0.1
  done
  for pid in "$TAIL_PID" "$FRONTEND_PID" "$BACKEND_PID"; do
    if [[ -n "$pid" ]]; then
      kill -KILL -- "-$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  exit "$exit_code"
}

require_command bun
require_command go
require_command lsof
require_command curl

if [[ ! "$STARTUP_TIMEOUT" =~ ^[1-9][0-9]{0,9}$ ]] || (( STARTUP_TIMEOUT > 2147483647 )); then
  echo "STARTUP_TIMEOUT 必须是 1 到 2147483647 的整数（秒）：$STARTUP_TIMEOUT" >&2
  exit 1
fi
for port in "$BACKEND_PORT" "$FRONTEND_PORT"; do
  if [[ ! "$port" =~ ^[1-9][0-9]{0,4}$ ]] || (( port > 65535 )); then
    echo "端口必须是 1 到 65535 的整数：$port" >&2
    exit 1
  fi
done
if [[ "$BACKEND_PORT" == "$FRONTEND_PORT" ]]; then
  echo "前后端端口不能相同。" >&2
  exit 1
fi

# Validate the complete configuration before stopping either running service.
for port in "$BACKEND_PORT" "$FRONTEND_PORT"; do
  stop_port_occupiers "$port"
done

trap 'cleanup' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
set -m

prepare_frontend

mkdir -p "$LOG_DIR"
: >"$FRONTEND_LOG"
: >"$BACKEND_LOG"

echo "正在启动后端，端口：$BACKEND_PORT"
(
  cd "$ROOT_DIR"
  exec env PORT="$BACKEND_PORT" go run .
) >>"$BACKEND_LOG" 2>&1 &
BACKEND_PID=$!

wait_for_health "后端" "$BACKEND_PORT" "$BACKEND_PID" "$BACKEND_LOG" "/api/status"

echo "正在启动前端，端口：$FRONTEND_PORT"
(
  cd "$FRONTEND_DIR"
  exec env VITE_REACT_APP_SERVER_URL="http://localhost:$BACKEND_PORT" \
    bun run dev -- --host 0.0.0.0 --port "$FRONTEND_PORT"
) >>"$FRONTEND_LOG" 2>&1 &
FRONTEND_PID=$!

wait_for_health "前端" "$FRONTEND_PORT" "$FRONTEND_PID" "$FRONTEND_LOG" "/"

echo "持续监控前后端日志，按 Ctrl+C 停止服务。"
tail -n 100 -F "$BACKEND_LOG" "$FRONTEND_LOG" &
TAIL_PID=$!
while kill -0 "$BACKEND_PID" 2>/dev/null && kill -0 "$FRONTEND_PID" 2>/dev/null && kill -0 "$TAIL_PID" 2>/dev/null; do
  sleep 1
done
echo "前后端服务或日志监控已退出，正在清理本次启动的进程。" >&2
exit 1
