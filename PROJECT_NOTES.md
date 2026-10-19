# PROJECT_NOTES — CVTROAD

> **이 파일을 읽은 Claude는 작업이 끝날 때마다 이 파일을 업데이트할 것.**
> 또한 작업 도중에도 의미 있는 단위(파일 몇 개, 30분 이내)마다 `dev` 브랜치에 커밋·푸시해서 세션이 끊겨도 작업이 남도록 할 것.

## 개요
- 앱 이름: **CVTROAD** — Tangerine(Leap Studio)에서 영감을 받은 오프라인 파일 변환 앱
- 대상: macOS 13+, Apple 실리콘 (Windows판은 추후 별도 요청 예정 → 엔진은 Go로 작성해 이식 쉽게 유지)
- 저장소: https://github.com/NeverthelessD/CVTROAD
- 저작권 표기: 화면 하단 `made by. Nevertheless_D`
- 형식: 더블클릭으로 실행하는 "스크립트형 앱"(.app 안에 launcher.sh + Go 엔진). Xcode 불필요.

## 구조
```
app/            앱 아이콘(AppIcon.svg/png/icns), 메뉴 막대 아이콘(StatusIcon.svg)
engine/         Go 엔진 (로컬 웹 UI 서버 + 변환기 + 업데이트)
  main.go       진입점, 설정(appearance: system|light|dark), 로컬 HTTP 서버
  convert.go    변환 라우팅 / formats.go 포맷 표
  image.go media.go doc.go pdf.go archive.go   종류별 변환
  tools.go meta.go   도구(PDF 분할·병합·압축, 메타데이터 제거, QR 등)
  jobs.go       작업 큐 / cli.go  명령행 모드(--run)
  update.go     GitHub 자동 업데이트 (releases/latest.json 확인)
  agent.go + agent/   백그라운드 도우미(JXA): Shift-드래그 원형 메뉴, 단축키, 메뉴 막대
  helper.go helper.js 시스템 도우미 연동
  web/          UI (index.html, css/app.css, js/app.js·panels.js·tools.js·art.js, 폰트, pdf.js)
scripts/test/   샘플 생성·변환 매트릭스 테스트
```

## 결정 사항
- 모든 변환은 로컬 처리, 파일 외부 전송 없음. 결과는 원본 옆에 새 파일로 저장.
- UI는 엔진이 띄우는 로컬 웹 화면. 라이트/다크 수동 선택 가능, 기본값은 macOS 설정 따름.
- 앱을 열지 않아도 쓰는 기능: Shift-드래그 원형 변환 메뉴(Shift+Option = 도구), 단축키 ⌃⌥C(변환)·⌃⌥T(도구), Finder 빠른 동작, 메뉴 막대 아이콘. 도우미는 로그인 시 launchd로 실행.
- 오디오는 MP3 포함.
- 업데이트: 자동 확인 + 수동 "업데이트 확인" 버튼 + 현재 버전 표시. 피드: `releases/latest.json` (main 브랜치, raw.githubusercontent).
- 기존 SROAD/YTROAD/LIBROAD 프로젝트와 같은 패턴(app/launcher.sh, releases/, docs/ 가이드 이미지)을 따름.
- 사용자에게 보이는 글(앱 화면·업데이트 안내·가이드 이미지)에는 GitHub·저장소 이름을 쓰지 않음 ("새 버전이 나오면 앱이 알아서 받아요").
- 도우미는 손쉬운 사용·입력 모니터링 권한 없이 마우스·키 "상태"만 살펴서 동작하도록 설계.
- ffmpeg(MP3·영상)는 첫 실행 때 자동으로 받음. 다른 앱(YTROAD·SROAD)이 받아 둔 것이 있으면 복사해서 씀.
- `releases/`는 출시할 때만 커밋(main). dev에서는 커밋하지 않음.

## 출시 방법
1. `VERSION` 올리기 → `scripts/build.sh --notes "바뀐 점"` → `releases/CVTROAD-<v>.zip`, `releases/latest.json` 생성
2. main에 커밋·푸시하면 설치된 앱이 6시간 안에(또는 '업데이트 확인'으로) 받음. releases/에는 최신 zip 3개만 유지.

## 리눅스에서 시험하기
- `go build -o /tmp/cv ./engine` 후 env: `CVTROAD_SUPPORT`, `CVTROAD_TOKEN`, `CVTROAD_PORT`, `CVTROAD_FFMPEG_DIR=/usr/bin`, `CVTROAD_HELPER_CMD="python3 scripts/test/fakehelper.py"`, `CVTROAD_APP`, `CVTROAD_UPDATE_URL`
- `python3 scripts/test/make_samples.py <dir>` → `run_matrix.py <engine> <dir> <out>`
- Playwright는 `LANG=C.UTF-8`로 실행해야 한글 파일명이 들어감 (시험 환경 문제, 앱 문제 아님)

## 진행 사항
- [x] 아이콘 (app/)
- [x] 엔진 소스 전체 (약 9,400줄) — `GOOS=darwin GOARCH=arm64` 컴파일 성공 (2026-10-19)
- [x] UI (engine/web)
- [x] 백그라운드 도우미 (engine/agent)
- [x] 테스트 스크립트 (scripts/test)
- [x] app/Info.plist, app/launcher.sh — .app 번들 조립
- [x] 빌드·패키징 스크립트 (scripts/build.sh, zipapp.py), VERSION 1.0.0
- [x] 변환 매트릭스 테스트: 68개 중 67개 통과 (HEIC 저장은 맥 sips 전용이라 리눅스에서만 실패 — 정상)
- [x] UI 확인 (Playwright, 라이트·다크, 변환 흐름, 도우미·설정 화면) — 콘솔 오류 없음
- [x] 업데이트 시험: 로컬 피드로 확인 → 내려받기 → sha256 확인 → 앱 교체 → 백업·되돌리기 가능
- [ ] 사용법·주의사항 한 장 이미지 (docs/CVTROAD_사용가이드.png)
- [ ] 사용자 맥에 설치·실행 확인, 손쉬운 사용 권한 안내
- [ ] dev → main 병합, 첫 릴리스 1.0.0

## 작업 기록
- 2026-10-19 12:4x — 세션이 응답 도중 끊겨 커밋 안 된 작업물을 `dev` 브랜치에 긴급 저장(254902b). 이 노트 작성.
