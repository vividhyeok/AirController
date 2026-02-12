# Air Controller

스마트폰을 무선 마우스 & 키보드로 사용하세요.

## Features
- 터치패드 스타일 마우스 조작 (이동, 좌클릭, 우클릭, 스크롤)
- 미디어 컨트롤 (재생/일시정지, 볼륨, 이전/다음 곡)
- 브라우저 탐색 (새 탭, 탭 전환, 뒤로/앞으로, 새로고침)
- 텍스트 입력 (클립보드 기반, 한글 완벽 지원)
- 즐겨찾기 & 최근 방문 관리
- 현재 탭 URL 가져오기
- PC 절전 예약
- 단일 exe 파일 — 외부 의존성 없음
- WebSocket 기반 저지연 통신

## Download
최신 Windows 빌드(AirController.exe)를 받으세요:
- https://github.com/vividhyeok/AirController/releases/latest

1. `AirController.exe` 다운로드
2. 실행 후 Windows Defender / 방화벽 허용
3. QR 코드를 스캔하거나 표시된 URL을 스마트폰에서 열기

## 네트워크 요구사항
- PC와 스마트폰이 **같은 Wi-Fi / LAN**에 연결되어 있어야 합니다
- TV에 HDMI 연결 시에도 동일하게 작동합니다 (디스플레이 확장일 뿐)
- 외부 네트워크에서 사용하려면 포트포워딩 또는 VPN 설정이 필요합니다

## Usage
### 1) PC에서 서버 실행
`AirController.exe`를 더블클릭하거나, 소스에서 직접 실행:

```bash
go run .
```

브라우저에 QR 코드 페이지가 자동으로 열립니다.

### 2) 스마트폰에서 컨트롤러 접속
- QR 코드를 스캔하거나
- 표시된 URL(예: `http://192.168.x.x:5000`)을 직접 입력

### 3) 조작
- **Touch** 탭: 터치패드, 탭하여 클릭, 스크롤
- **Input** 탭: 미디어/브라우저 제어, 텍스트 입력, 감도 조절
- **Apps** 탭: 즐겨찾기, URL 열기, 현재 탭 가져오기, PC 절전

## Build
소스에서 직접 빌드하려면:

```bash
go build -ldflags "-s -w" -o AirController.exe .
```

## Tech Stack
- Backend: Go (net/http, gorilla/websocket)
- Frontend: HTML/CSS/JS (Native WebSocket)
- PC 제어: Windows API (user32.dll, kernel32.dll) via syscall
- QR Code: skip2/go-qrcode
- 템플릿: Go embed + html/template

## License
MIT License
