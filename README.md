# Air Mouse

Windows PC를 휴대폰이나 태블릿 브라우저에서 조작하는 가벼운 리모컨입니다. PC에서는 단일 실행 파일만 실행하고, 조작 기기에는 별도 앱을 설치하지 않습니다.

## 주요 기능

- 터치패드 마우스 이동, 좌·우 클릭, 스크롤
- 재생·볼륨·브라우저·키보드 제어
- 텍스트 입력, 주소 열기, 현재 탭 가져오기
- 이름을 지정할 수 있는 즐겨찾기와 최근 기록
- 즐겨찾기·감도·마지막 탭 설정 백업 및 복원
- PC 절전 예약
- 유선 LAN, Wi-Fi, 모바일 핫스팟, Bluetooth PAN 등 여러 연결 주소 안내

## 연결 방식

Air Mouse는 Windows에서 HTTP/WebSocket 서버를 열고 휴대폰 브라우저가 접속하는 구조입니다. Wi-Fi 전용이 아니며, 두 기기 사이에 IP 통신만 가능하면 동작합니다.

지원하는 대표 구성:

- PC 유선 LAN + 휴대폰은 같은 공유기의 Wi-Fi
- PC와 휴대폰이 같은 Wi-Fi
- 휴대폰 핫스팟에 PC 연결
- Windows 모바일 핫스팟에 휴대폰 연결
- PC 블루투스 동글 + 휴대폰 Bluetooth 테더링/개인용 핫스팟(Bluetooth PAN)
- 양쪽 기기가 함께 접속한 VPN 또는 오버레이 네트워크

실행하면 연결 가능한 어댑터별 QR 코드가 표시됩니다. 휴대폰이 실제로 연결된 방식과 같은 카드를 선택하세요.

### Bluetooth PAN

Bluetooth를 직접 제어 프로토콜로 사용하는 것이 아니라, Bluetooth PAN 위에서 기존 웹 UI와 WebSocket을 사용합니다.

1. Windows PC에 블루투스 동글을 연결하고 휴대폰과 페어링합니다.
2. Android에서는 `Bluetooth 테더링`, iPhone에서는 `개인용 핫스팟`과 Bluetooth 연결을 켭니다.
3. Air Mouse가 동글·페어링·PAN 상태를 자동으로 감지합니다. Wi-Fi 어댑터가 없고 페어링된 PAN 휴대폰이 있으면 Windows Bluetooth 연결 화면도 자동으로 엽니다.
4. 열린 화면에서 휴대폰의 `개인 영역 네트워크(PAN)` 연결을 선택합니다.
5. 연결이 성립되면 Air Mouse가 별도 새로고침 없이 `Bluetooth PAN` 주소와 QR로 자동 전환합니다.

PC는 인터넷에 유선 LAN으로 연결된 상태여도 괜찮습니다. Air Mouse 조작 트래픽만 Bluetooth PAN을 통해 휴대폰과 오갑니다. Wi-Fi 동글은 필요하지 않습니다.

최초 페어링, 휴대폰의 테더링 활성화, Windows의 PAN 연결 승인은 운영체제 보안상 사용자가 직접 해야 합니다. Windows에는 PAN 연결을 무인으로 시작하는 안정적인 공개 API가 없어 화면 버튼을 강제로 자동 클릭하지 않습니다.

## 방화벽

이전 버전은 실행할 때마다 여러 방화벽 규칙을 삭제하고 다시 만들었습니다. 일반 권한에서 재등록이 실패하거나 EXE 경로·대체 포트가 바뀌면 Windows 허용 창이 반복될 수 있었습니다.

현재 버전은 방화벽을 자동 변경하지 않습니다. 연결 화면에서 필요할 때만 `한 번만 허용`을 누르면 UAC 승인 후 다음 규칙 하나를 등록합니다.

- TCP `5000-5019`
- 로컬 서브넷에서만 접근 허용
- 모든 Windows 네트워크 프로필에서 동일하게 적용

포트 기반 규칙이라 EXE를 업데이트하거나 같은 PC 안에서 위치를 옮겨도 매번 다시 등록할 필요가 없습니다. 앱을 처음 실행했을 때 Windows 자체 허용 창이 먼저 나타나면 로컬 네트워크만 허용하면 됩니다.

## 사용법

1. Windows PC에서 `AirMouse.exe`를 실행합니다.
2. 자동으로 열린 연결 화면에서 현재 연결 방식의 QR을 고릅니다.
3. 휴대폰으로 QR을 스캔해 브라우저 리모컨을 엽니다.
4. 연결이 되지 않을 때만 연결 화면의 방화벽 버튼을 한 번 사용합니다.

소스에서 실행:

```bash
go run .
```

테스트와 빌드:

```bash
go test ./...
go build -ldflags "-s -w" -o AirMouse.exe .
```

EXE·ZIP·MSI 일괄 빌드(Windows, WiX 3.14.1 필요):

```powershell
$env:WIX_BIN = 'C:\tools\wix314'
.\scripts\build-release.ps1 -ProductVersion '1.1.0'
```

MSI는 `Program Files\AirController`에 설치하고 시작 메뉴에 `Air Mouse` 바로가기를 만듭니다. 제거는 Windows의 설치된 앱 화면에서 할 수 있습니다. 앱 실행 중 방화벽을 자동 수정하지 않는 원칙은 MSI 설치에서도 동일합니다.

## 입력 지연 최적화

- 포인터 이동과 스크롤을 브라우저 화면 주사율 단위로 합쳐 전송
- WebSocket 전송 적체 시 오래된 이동 입력을 쌓지 않고 폐기
- TCP `NoDelay`와 keep-alive 적용
- 텍스트 입력·현재 탭 조회 같은 느린 자동화 작업을 포인터 처리와 분리
- 네트워크가 복구되거나 브라우저로 돌아오면 빠르게 재연결

## 다운로드

최신 Windows 빌드:

- https://github.com/vividhyeok/AirController/releases/latest

GitHub Actions의 `Windows Release` 실행 결과에서도 다음 파일을 한 번에 받을 수 있습니다.

- `AirMouse.exe`: 바로 실행하는 단일 파일
- `AirMouse-windows-amd64.zip`: 휴대용 압축본
- `AirMouse-Setup-x64.msi`: 시작 메뉴와 제거 기능이 포함된 설치본

브랜치와 PR에서는 Actions 아티팩트를 만들고, `main` 또는 `master` 브랜치에 push되면 `latest` 릴리스까지 갱신합니다.

## 기술 구성

- Backend: Go `net/http`, `gorilla/websocket`
- Frontend: HTML/CSS/JavaScript
- Windows 제어: `user32.dll`, `kernel32.dll`
- QR: `skip2/go-qrcode`
- 패키징: 외부 런타임이 필요 없는 단일 Windows 실행 파일

## License

MIT License
