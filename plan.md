# Air Mouse 연결 구조

Air Mouse는 Wi-Fi 전용 앱이 아니라 IP 기반 브라우저 리모컨이다. Windows PC가 모든 활성 네트워크 어댑터에서 HTTP/WebSocket 요청을 받고, 연결 화면은 사용 가능한 실제 IPv4 주소를 종류별로 보여준다.

## 지원 구조

- 유선 LAN과 같은 공유기의 Wi-Fi
- 같은 Wi-Fi
- Android/iPhone 또는 Windows 핫스팟
- Bluetooth PAN
- 사용자가 구성한 VPN/오버레이 네트워크

Docker, WSL, Hyper-V, VMware 같은 내부 전용 가상 어댑터는 잘못된 QR을 만들 가능성이 높으므로 안내 대상에서 제외한다.

## Bluetooth 원칙

브라우저 호환성이 낮은 Web Bluetooth나 전용 BLE 프로토콜을 만들지 않는다. 휴대폰이 제공하는 Bluetooth 테더링/개인용 핫스팟으로 BTPAN을 구성하고 그 위에 기존 HTTP/WebSocket을 그대로 사용한다. 따라서 Android와 iPhone에서 같은 컨트롤 UI를 유지할 수 있다.

## 방화벽 원칙

- 실행 시 규칙을 삭제하거나 다시 만들지 않는다.
- 사용자가 로컬 연결 화면에서 요청할 때만 UAC를 통해 한 번 설정한다.
- 앱 경로가 아닌 고정 포트 범위 `5000-5019`를 허용한다.
- 원격 주소는 `LocalSubnet`으로 제한한다.

## 성능 원칙

- 고빈도 포인터 이벤트는 화면 프레임마다 하나로 합친다.
- 지연된 포인터 입력을 큐에 길게 쌓지 않는다.
- 느린 Windows 자동화 작업은 WebSocket 읽기 루프와 분리한다.
- 단일 Go 실행 파일과 기존 의존성만 유지한다.
