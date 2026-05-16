# Air Mouse

Windows PC를 휴대폰이나 태블릿 브라우저에서 조작하는 영상 감상용 리모컨입니다. TV에 연결한 노트북으로 YouTube, LAFTEL, ANIPLUS, YouTube Music, CHZZK 같은 서비스를 볼 때 누워서 마우스, 재생, 볼륨, 브라우저 이동을 제어하는 목적에 맞춰져 있습니다.

## Features

- 터치패드 기반 마우스 이동
- 터치패드 오른쪽 영역 스크롤 및 탭 우클릭
- 나머지 터치패드 영역 탭 좌클릭
- 재생/일시정지, 5초 이동, 이전/다음, 전체화면, 볼륨 제어
- 브라우저 뒤로/앞으로, 새로고침, 탭 이동
- 텍스트 입력 및 Enter 전송
- 주소 열기, 즐겨찾기, 최근 기록 관리
- 현재 탭 주소 가져오기
- PC 절전 예약

## Download

최신 Windows 빌드는 GitHub Releases에서 받을 수 있습니다.

- https://github.com/vividhyeok/AirController/releases/latest

`main` 또는 `master` 브랜치에 push되면 GitHub Actions가 Windows에서 테스트와 빌드를 실행하고, `latest` 릴리스에 `AirMouse.exe`와 `AirMouse-windows-amd64.zip`을 갱신합니다.

## Network

PC와 조작 기기는 같은 네트워크에서 서로 접근 가능해야 합니다. PC가 랜선으로 연결되어 있고 휴대폰이 같은 공유기의 Wi-Fi에 연결된 경우에도 동작할 수 있습니다.

학교망, 회사망, 게스트 Wi-Fi처럼 장치 간 통신이 차단된 네트워크는 지원 범위에서 제외합니다.

휴대폰에서 계속 로딩되면 다음을 확인하세요.

- 휴대폰이 모바일 데이터나 게스트 Wi-Fi가 아니라 PC와 같은 네트워크를 사용하는지
- 휴대폰 IP 대역이 PC IP 대역과 같은지
- Windows 방화벽에서 Air Mouse 실행 파일 또는 TCP 포트가 허용되어 있는지
- 공유기에서 AP isolation/client isolation이 꺼져 있는지

## Usage

1. Windows PC에서 `AirMouse.exe`를 실행합니다.
2. 자동으로 열리는 QR 페이지를 휴대폰으로 스캔합니다.
3. 또는 표시된 URL을 휴대폰 브라우저에 직접 입력합니다.

소스에서 직접 실행할 수도 있습니다.

```bash
go run .
```

## Build

```bash
go build -ldflags "-s -w" -o AirMouse.exe .
```

GitHub Actions 자동 빌드는 Windows용만 수행합니다.

## Tech Stack

- Backend: Go (`net/http`, `gorilla/websocket`)
- Frontend: HTML/CSS/JavaScript
- PC 제어: Windows API (`user32.dll`, `kernel32.dll`) via syscall
- QR Code: `skip2/go-qrcode`
- Template: Go embed + `html/template`

## License

MIT License
