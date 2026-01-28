# Air Controller

Turn your smartphone into a wireless mouse and keyboard for your PC.

## Features
- Touchpad-style mouse control with gestures
- Left click, right click, and scrolling
- Text input (supports voice input on the phone)
- Quick-launch buttons for favorite sites
- Low-latency control via WebSocket
- Responsive UI for phone/tablet, portrait/landscape

## Download (no Python required)
Get the latest Windows build (AirController.exe):
- https://github.com/vividhyeok/AirController/releases/latest

1. Download `AirController.exe`
2. Run it and allow Windows Defender / firewall if prompted
3. Scan the QR code or open the shown URL on your phone

## Network scope (important)
This app is designed for local network use.
- Your PC and phone must be on the same Wi-Fi / LAN
- Using HDMI to a TV does not affect anything (it’s just a display)
- If you need to use it across different networks, set up port forwarding or a VPN

## Usage
### 1) Run the server on your PC
Double-click `AirController.exe`, or run from source:

```bash
python remote_server.py
```

You will see a URL and a QR code in the console.

### 2) Open the controller on your phone
- Scan the QR code, or
- Open the printed URL (e.g. `http://192.168.x.x:5000`)

### 3) Use the controller
Tabs:
- Touch: touchpad, tap, right-click, scroll
- Input: text, enter/backspace/esc/space, sensitivity
- Apps: quick-launch buttons (customizable)

## Build the EXE yourself
```bash
pip install -r requirements.txt
pip install pyinstaller
.\\build_exe.bat
```

The output is placed in `dist/`.

## Troubleshooting
- Phone cannot connect: make sure both devices are on the same Wi-Fi and Windows Firewall allows the app
- Input feels too fast/slow: adjust sensitivity in the Input tab
- Korean input issues: clipboard-based paste may be blocked in some apps

## Tech Stack
- Backend: Python, Flask, Flask-SocketIO
- Frontend: HTML/CSS/JS (Socket.IO)
- Control: PyAutoGUI, Pyperclip
- Build: PyInstaller

## License
MIT License
