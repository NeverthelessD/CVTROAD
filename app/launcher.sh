#!/bin/bash
# ─────────────────────────────────────────────────────────────
#  CVTROAD — 파일 변환 앱 (macOS, Apple Silicon)
#  made by. Nevertheless_D
#
#  더블클릭하면 이 스크립트가 실행됩니다.
#  1) 앱 안의 엔진을 앱 전용 폴더로 복사하고  2) 엔진을 켠 뒤  3) 앱 화면을 창으로 엽니다.
#  엔진은 켜질 때 백그라운드 도우미(Shift-드래그 메뉴 · 단축키 · 메뉴 막대)도 함께 준비해요.
# ─────────────────────────────────────────────────────────────
export PATH="/usr/bin:/bin:/usr/sbin:/sbin"

HERE="$(cd "$(dirname "$0")" && pwd)"
APP="$(cd "$HERE/../.." && pwd)"
RES="$HERE/../Resources"
SUPPORT="$HOME/Library/Application Support/CVTROAD"
BIN="$SUPPORT/bin"
BUILD="$(cat "$RES/version.txt" 2>/dev/null)"
mkdir -p "$BIN"

dialog() {
  osascript -e 'on run argv' \
    -e 'display dialog (item 1 of argv) with title "CVTROAD" buttons {"확인"} default button 1 with icon caution' \
    -e 'end run' "$1" >/dev/null 2>&1
}

# 앱 창 열기: Chrome·Edge·Brave가 있으면 주소창 없는 "앱 창"으로, 없으면 기본 브라우저(Safari 등)로 엽니다.
open_ui() {
  local url="http://127.0.0.1:$1/?t=$2" b
  for b in "Google Chrome" "Microsoft Edge" "Brave Browser"; do
    if [ -d "/Applications/$b.app" ] || [ -d "$HOME/Applications/$b.app" ]; then
      open -na "$b" --args --app="$url" --window-size=1180,820 && return
    fi
  done
  open "$url"
}

ping_engine() { # port token
  curl -fs -m 2 -H "x-token: $2" "http://127.0.0.1:$1/api/ping" 2>/dev/null
}

if [ "$(uname -m)" != "arm64" ]; then
  dialog "CVTROAD는 Apple Silicon(M1·M2·M3·M4…) 맥 전용이에요.
이 맥에서는 실행할 수 없어요."
  exit 1
fi

RESTART="${CVTROAD_RESTART:-}"
PREF_PORT=""
TOKEN=""

# 1) 이미 실행 중이면 창만 다시 열기 (다른 버전이 켜져 있으면 끄고 새로 시작)
if [ -z "$RESTART" ] && [ -s "$SUPPORT/port" ] && [ -s "$SUPPORT/token" ]; then
  P="$(cat "$SUPPORT/port")"; T="$(cat "$SUPPORT/token")"
  R="$(ping_engine "$P" "$T")"
  case "$R" in
    *"\"build\":\"$BUILD\""*) open_ui "$P" "$T"; exit 0 ;;
    *"\"ok\":true"*)
      curl -fs -m 3 -X POST -H "x-token: $T" "http://127.0.0.1:$P/api/quit" >/dev/null 2>&1
      sleep 1.2 ;;
  esac
fi

# 업데이트 후 다시 시작: 같은 포트·토큰을 써서 열려 있던 창이 그대로 이어지게 합니다
if [ -n "$RESTART" ]; then
  PREF_PORT="${CVTROAD_PORT:-}"
  TOKEN="${CVTROAD_TOKEN:-}"
  for _ in $(seq 1 30); do
    [ -n "$PREF_PORT" ] && [ -n "$(ping_engine "$PREF_PORT" "$TOKEN")" ] || break
    sleep 0.3
  done
fi

# 2) 엔진을 앱 전용 폴더로 복사해서 실행
#    (다운로드 폴더에서 바로 실행하거나, 나중에 앱을 옮기거나 지워도 도우미가 계속 동작하도록)
ENGINE="$BIN/cvtroad-engine"
if [ ! -x "$ENGINE" ] || [ "$(cat "$BIN/build" 2>/dev/null)" != "$BUILD" ]; then
  rm -f "$ENGINE.new"
  if cat "$RES/cvtroad-engine" > "$ENGINE.new" && chmod 755 "$ENGINE.new" && mv -f "$ENGINE.new" "$ENGINE"; then
    printf '%s' "$BUILD" > "$BIN/build"
  fi
fi
xattr -c "$ENGINE" 2>/dev/null
if [ ! -x "$ENGINE" ]; then
  dialog "😢 CVTROAD를 준비하지 못했어요.
CVTROAD를 응용 프로그램 폴더로 옮긴 뒤 다시 실행해 주세요."
  exit 1
fi

# 3) 엔진 켜기
#    엔진이 쉬다가 꺼졌어도, 열려 있던 앱 창이 저절로 다시 이어지도록 지난번 열쇠(토큰)와 포트를 다시 써요
REUSED=""
if [ -z "$TOKEN" ] && [ -s "$SUPPORT/token" ]; then TOKEN="$(cat "$SUPPORT/token")"; fi
if [ -z "$PREF_PORT" ] && [ -s "$SUPPORT/last-port" ]; then PREF_PORT="$(cat "$SUPPORT/last-port")"; REUSED=1; fi
[ -n "$TOKEN" ] || TOKEN="$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom 2>/dev/null | head -c 32)"
rm -f "$SUPPORT/port"
CVTROAD_TOKEN="$TOKEN" CVTROAD_SUPPORT="$SUPPORT" CVTROAD_APP="$APP" CVTROAD_PORT="$PREF_PORT" CVTROAD_RESTART="$RESTART" \
  nohup "$ENGINE" > /dev/null 2>&1 < /dev/null &
disown

for _ in $(seq 1 75); do [ -s "$SUPPORT/port" ] && break; sleep 0.2; done
if [ ! -s "$SUPPORT/port" ]; then
  dialog "😢 CVTROAD를 시작하지 못했어요. 앱을 다시 실행해 보세요.
계속 안 되면 컴퓨터를 재시동한 뒤 시도해 주세요."
  exit 1
fi
PORT="$(cat "$SUPPORT/port")"
printf '%s' "$PORT" > "$SUPPORT/last-port"

# 4) 앱 창 열기 (다시 시작이고 같은 포트라면 열려 있던 창이 스스로 새로고침해요)
if [ -n "$RESTART" ] && [ "$PORT" = "$PREF_PORT" ]; then
  exit 0
fi
# 같은 포트로 다시 켰다면, 열려 있던 창이 몇 초 안에 다시 이어지는지 보고 — 이어지면 새 창을 또 열지 않아요
if [ -n "$REUSED" ] && [ "$PORT" = "$PREF_PORT" ]; then
  for _ in $(seq 1 12); do
    case "$(ping_engine "$PORT" "$TOKEN")" in *'"page":true'*) exit 0 ;; esac
    sleep 0.25
  done
fi
open_ui "$PORT" "$TOKEN"
exit 0
