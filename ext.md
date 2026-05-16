# EXE 실행 범위 정리

## 결론

현재 빌드되는 Air Mouse 실행 파일은 Windows용 exe입니다. 모든 기기에서 직접 실행하는 파일이 아니라, Windows PC에서 실행하고 Android/iPhone/태블릿은 브라우저로 접속해 조작하는 구조입니다.

## 실행 가능한 쪽

- `AirMouse.exe`: Windows PC에서 실행
- 제어 대상: exe가 실행 중인 Windows PC
- 조작 기기: Android, iPhone, 태블릿, 다른 PC 등 최신 브라우저와 WebSocket을 지원하고 같은 네트워크에서 접속 가능한 기기

## 직접 실행이 불가능한 쪽

- Android에서는 `.exe`를 직접 실행할 수 없습니다.
- iPhone/iPad에서도 `.exe`를 직접 실행할 수 없습니다.
- macOS/Linux에서도 현재 Windows용 exe 파일 자체는 실행 대상이 아닙니다.

## 이유

현재 Air Mouse는 Windows API인 `user32.dll`, `kernel32.dll`, `rundll32.exe`를 사용해서 마우스, 키보드, 클립보드, 절전 명령을 처리합니다. 이 API들은 Windows 전용이라 같은 exe를 Android나 iOS에서 직접 실행할 수 없습니다.

## 호환성 기준

- 이번 빌드는 Windows용입니다.
- 일반적인 64-bit Windows 10/11 PC 사용을 기준으로 합니다.
- Windows ARM 기기까지 보장하려면 `GOARCH=arm64`로 별도 빌드하고 실제 기기 테스트가 필요합니다.
- macOS/Linux용으로 만들려면 빌드만 바꾸는 것으로는 부족하고, 마우스/키보드/클립보드 제어 로직을 각 OS 전용 API로 새로 구현해야 합니다.

## 자동 빌드

GitHub Actions 자동 빌드는 Windows용만 수행합니다. `main` 또는 `master` 브랜치에 push되면 Windows runner에서 테스트 후 `AirMouse.exe`를 빌드하고, `latest` 릴리스에 실행 파일과 zip 파일을 갱신합니다.
