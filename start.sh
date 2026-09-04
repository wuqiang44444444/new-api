#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$ROOT_DIR/web"
LOG_DIR="$ROOT_DIR/logs"
# All component ports stay inside 3500-3510.
FRONTEND_PORT="${FRONTEND_PORT:-3501}"
BACKEND_PORT="${BACKEND_PORT:-3500}"
STARTUP_TIMEOUT="${STARTUP_TIMEOUT:-300}"
FRONTEND_LOG="$LOG_DIR/frontend.log"
BACKEND_LOG="$LOG_DIR/backend.log"
FRONTEND_PID=""
BACKEND_PID=""

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

stop_port() {
  local port="$1"
  local pid
  local pids=()

  while IFS= read -r pid; do
    [[ -n "$pid" ]] && pids+=("$pid")
  done < <(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
  if (( ${#pids[@]} == 0 )); then
    return
  fi

  echo "正在停止占用端口 $port 的进程：${pids[*]}"
  kill "${pids[@]}" 2>/dev/null || true

  for _ in {1..20}; do
    if ! lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      return
    fi
    sleep 0.25
  done

  pids=()
  while IFS= read -r pid; do
    [[ -n "$pid" ]] && pids+=("$pid")
  done < <(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
  if (( ${#pids[@]} > 0 )); then
    echo "进程未及时退出，强制释放端口 ${port}：${pids[*]}"
    kill -KILL "${pids[@]}" 2>/dev/null || true
  fi

  for _ in {1..20}; do
    if ! lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      return
    fi
    sleep 0.25
  done

  echo "无法释放端口 $port，请手动检查占用进程。" >&2
  return 1
}

wait_for_port() {
  local name="$1"
  local port="$2"
  local pid="$3"
  local log_file="$4"
  local timeout="$5"
  local attempts=$((timeout * 2))

  for ((i = 0; i < attempts; i++)); do
    if lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      echo "$name 已启动：http://localhost:$port"
      return
    fi

    if ! kill -0 "$pid" 2>/dev/null; then
      echo "$name 启动失败，最近日志如下：" >&2
      tail -n 80 "$log_file" >&2 || true
      return 1
    fi
    sleep 0.5
  done

  echo "$name 在 ${timeout} 秒内未监听端口 ${port}，最近日志如下：" >&2
  tail -n 80 "$log_file" >&2 || true
  return 1
}

cleanup() {
  trap - EXIT INT TERM

  if [[ -n "$FRONTEND_PID" ]]; then
    kill "$FRONTEND_PID" 2>/dev/null || true
  fi
  if [[ -n "$BACKEND_PID" ]]; then
    kill "$BACKEND_PID" 2>/dev/null || true
  fi

  stop_port "$FRONTEND_PORT"
  stop_port "$BACKEND_PORT"
}

trap cleanup EXIT
trap 'exit 130' INT TERM

require_command bun
require_command go
require_command lsof

if [[ ! "$STARTUP_TIMEOUT" =~ ^[1-9][0-9]*$ ]]; then
  echo "STARTUP_TIMEOUT 必须是正整数（秒）：$STARTUP_TIMEOUT" >&2
  exit 1
fi

stop_port "$FRONTEND_PORT"
stop_port "$BACKEND_PORT"

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

echo "正在启动前端，端口：$FRONTEND_PORT"
(
  cd "$FRONTEND_DIR"
  exec env VITE_REACT_APP_SERVER_URL="http://localhost:$BACKEND_PORT" \
    bun run dev -- --host 0.0.0.0 --port "$FRONTEND_PORT"
) >>"$FRONTEND_LOG" 2>&1 &
FRONTEND_PID=$!

wait_for_port "后端" "$BACKEND_PORT" "$BACKEND_PID" "$BACKEND_LOG" "$STARTUP_TIMEOUT"
wait_for_port "前端" "$FRONTEND_PORT" "$FRONTEND_PID" "$FRONTEND_LOG" "$STARTUP_TIMEOUT"

echo "持续监控前后端日志，按 Ctrl+C 停止服务。"
tail -n 100 -F "$BACKEND_LOG" "$FRONTEND_LOG"
