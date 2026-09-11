# AirController

휴대폰이나 태블릿을 **Windows PC용 무선 리모컨 + 터치패드**로 사용하는 로컬 웹 컨트롤러입니다.

TV나 외부 모니터에 연결한 Windows PC에서 YouTube, YouTube Music, LAFTEL, ANIPLUS, CHZZK 같은 콘텐츠를 볼 때, 소파나 침대에서 일어나지 않고 브라우저를 조작하는 용도로 만들어졌습니다.

별도 모바일 앱은 필요하지 않습니다. Windows에서 `AirMouse.exe`를 실행한 뒤, 같은 네트워크의 휴대폰/태블릿에서 QR 코드만 열면 바로 사용할 수 있습니다.

## 주요 기능

### Touch

- 터치패드로 마우스 커서 이동
- 터치패드 우측 스트립으로 스크롤
- 일반 영역 탭: 좌클릭
- 스크롤 영역 탭: 우클릭
- 마우스/스크롤 감도 조절
- 터치 이동 이벤트를 프레임 단위로 묶어 전송해 조작 안정성 개선

### Media

- 재생 / 일시정지
- 5초 이전 / 다음
- 이전 트랙 / 다음 트랙
- 전체화면
- 볼륨 증가 / 감소 / 음소거
- `±5초`, `볼륨 ±` 버튼 길게 누르기 연속 입력 지원

### Browser & Input

- 뒤로 / 앞으로 / 새로고침
- 새 탭 / 탭 닫기
- 이전 탭 / 다음 탭
- 브라우저 홈
- Space, Enter, Backspace, ESC, Tab, Win, 방향키
- 휴대폰에서 작성한 텍스트를 PC에 입력
- 텍스트 입력 후 Enter까지 한 번에 전송

### Apps & URL

- 주소 또는 도메인을 입력해 PC 브라우저에서 열기
- 현재 활성 브라우저 탭의 제목/URL 가져오기
- 즐겨찾기 추가, 삭제, 순서 변경
- 최근 열었던 주소 기록
- 즐겨찾기/최근 기록/감도/마지막 탭 설정 저장

설정은 브라우저 `localStorage`와 함께 Windows의 다음 위치에도 저장됩니다.

```text
%LOCALAPPDATA%\AirController\config.json
```

따라서 휴대폰 브라우저의 로컬 데이터가 사라지더라도 PC 쪽 설정을 다시 불러올 수 있습니다.

### Power & Display

- 즉시 또는 지정 시간 후 PC 절전
- 30 / 60 / 90분 빠른 절전 예약
- 절전 예약 취소
- 자동 절전 방지 켜기 / 끄기
- 남은 절전 예약 시간 표시
- Windows 디스플레이 모드 전환
  - 노트북 화면만
  - 복제
  - 확장
  - 외부 화면(TV)만

### Reliability

- WebSocket 자동 재연결
- Ping/Pong 기반 연결 상태 확인
- 사용 중인 포트가 있으면 `5000`부터 최대 20개 포트 범위에서 자동으로 빈 포트 탐색
- 실행 시 Windows 방화벽 허용 규칙 설정 시도
- 동일 프로그램 중복 실행 방지
- 이미 실행 중인 상태에서 다시 실행하면 기존 서버의 QR 페이지를 다시 열도록 처리

## 최신 업데이트

최근 업데이트에서는 거실/TV 리모컨 용도에서 불편했던 부분을 중심으로 안정성을 보강했습니다.

- 즐겨찾기, 최근 기록, 감도, 마지막 탭을 PC에 영구 저장
- 절전 예약 상태와 남은 시간 표시
- 절전 예약 취소 및 자동 절전 방지 추가
- 노트북/복제/확장/외부 화면 디스플레이 전환 추가
- 미디어 버튼 길게 누르기 반복 입력 지원
- 터치 이동/스크롤 이벤트 배치 처리
- 중복 실행 시 기존 AirController 재사용
- Windows PR CI 추가

## 빠른 시작

### 1. 다운로드

최신 Windows 빌드:

- https://github.com/vividhyeok/AirController/releases/latest

Release에는 다음 파일이 제공됩니다.

- `AirMouse.exe` — 바로 실행 가능한 단일 실행 파일
- `AirMouse-windows-amd64.zip` — Windows x64 압축 배포본

### 2. PC에서 실행

`AirMouse.exe`를 실행합니다.

실행되면 AirController가 로컬 서버를 시작하고 브라우저에 QR 페이지를 자동으로 엽니다.

Windows 방화벽 관련 확인 창이 나타나면 **같은 네트워크의 휴대폰이 접속할 수 있도록 허용**해주세요.

### 3. 휴대폰/태블릿 연결

PC와 같은 공유기에 연결된 휴대폰 또는 태블릿에서:

1. PC에 표시된 QR 코드를 스캔합니다.
2. 또는 화면에 표시된 `http://<PC-IP>:<PORT>` 주소를 직접 엽니다.
3. 상단 상태가 `Online`이면 바로 사용할 수 있습니다.

별도 앱 설치나 계정 로그인은 필요하지 않습니다.

## 화면 구성

AirController 모바일 UI는 세 개의 탭으로 구성됩니다.

| 탭 | 용도 |
| --- | --- |
| **Touch** | 마우스 이동, 좌/우 클릭, 스크롤 |
| **Input** | 미디어, 브라우저, 키보드, 텍스트 입력 |
| **Apps** | URL 열기, 현재 탭, 즐겨찾기, 최근 기록, 전원/디스플레이 제어 |

## 네트워크 조건

AirController는 인터넷 서버를 거치지 않고 **로컬 네트워크에서 PC와 휴대폰이 직접 통신**합니다.

PC는 유선 LAN이고 휴대폰은 같은 공유기의 Wi-Fi여도 서로 접근 가능한 네트워크라면 사용할 수 있습니다.

학교/회사/게스트 Wi-Fi처럼 기기 간 통신을 차단하는 네트워크에서는 연결되지 않을 수 있습니다.

### 휴대폰에서 접속되지 않을 때

다음 순서로 확인하세요.

1. 휴대폰이 모바일 데이터가 아니라 PC와 같은 네트워크에 연결되어 있는지 확인합니다.
2. PC에 표시된 주소의 `/health`를 휴대폰에서 열어봅니다.
   - 예: `http://192.168.0.10:5000/health`
3. Windows 방화벽에서 `AirMouse.exe`의 네트워크 접근을 허용합니다.
4. 공유기의 `AP Isolation`, `Client Isolation`, 게스트 네트워크 격리 기능이 켜져 있지 않은지 확인합니다.
5. VPN이나 보안 프로그램이 로컬 네트워크 통신을 차단하고 있지 않은지 확인합니다.

## 개발 환경에서 실행

### 요구 사항

- Windows
- Go 1.25+

```bash
git clone https://github.com/vividhyeok/AirController.git
cd AirController
go run .
```

## 빌드

```bash
go test ./...
go build -ldflags "-s -w" -o AirMouse.exe .
```

프론트엔드는 Go `embed`로 실행 파일 안에 포함되므로 별도 정적 파일 배포가 필요하지 않습니다.

## 자동 빌드

`main` 또는 `master` 브랜치에 push되면 GitHub Actions가 Windows에서:

1. `go test ./...`
2. `AirMouse.exe` 빌드
3. ZIP 패키징
4. `latest` GitHub Release의 실행 파일 갱신

을 수행합니다.

Pull Request에서는 별도의 Windows CI가 테스트, 빌드, JavaScript 문법 검사를 수행합니다.

## 기술 구성

- **Backend:** Go (`net/http`)
- **Realtime:** `gorilla/websocket`
- **Frontend:** HTML / CSS / Vanilla JavaScript
- **PC Control:** Windows API via `syscall`
- **Power / Display:** Windows `kernel32.dll`, `user32.dll`, `powrprof.dll`
- **QR:** `skip2/go-qrcode`
- **Assets:** Go `embed`

## 동작 구조

```text
Phone / Tablet Browser
        │
        │ HTTP + WebSocket (LAN)
        ▼
AirMouse.exe on Windows
        │
        ├─ Mouse / Keyboard / Media control
        ├─ URL / Browser control
        ├─ Sleep / Keep-awake control
        └─ Windows display mode control
```

기본 웹 서버는 `5000`번 포트부터 사용 가능한 포트를 찾습니다. 추가 전원/디스플레이 기능은 내부적으로 `5050` 포트의 기능 서버를 사용합니다.

## 보안 범위

AirController는 로컬 네트워크에서 입력 제어 명령을 받는 프로그램입니다. 신뢰할 수 없는 공용 네트워크에서 실행하지 않는 것을 권장합니다.

WebSocket 및 확장 기능 요청은 호스트/Origin을 검사하며, URL 열기는 `http`와 `https` 주소만 허용합니다.

## License

MIT License
