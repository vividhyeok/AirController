import sys
import os
from flask import Flask, render_template, request, jsonify
from flask_socketio import SocketIO, emit
import pyautogui
import socket
import qrcode
import webbrowser
import threading
import time

# Flask 앱과 SocketIO 초기화
if getattr(sys, 'frozen', False):
    template_folder = os.path.join(sys._MEIPASS, 'templates')
    app = Flask(__name__, template_folder=template_folder)
else:
    app = Flask(__name__)

# cors_allowed_origins='*' 는 로컬 네트워크의 다른 기기 접속 허용
socketio = SocketIO(app, cors_allowed_origins='*', async_mode='threading')

# 마우스 안전모드 해제 (화면 구석으로 가면 멈추는 기능 해제)
pyautogui.FAILSAFE = False

@app.route('/')
def index():
    return render_template('index.html')

# === WebSocket 이벤트 핸들러 ===

@socketio.on('connect')
def handle_connect():
    print('Client connected')

@socketio.on('move')
def handle_move(data):
    try:
        dx = float(data.get('dx', 0))
        dy = float(data.get('dy', 0))
        pyautogui.moveRel(dx, dy, _pause=False)
    except Exception:
        pass

@socketio.on('scroll')
def handle_scroll(data):
    try:
        dy = int(data.get('dy', 0))
        pyautogui.scroll(dy, _pause=False)
    except Exception:
        pass

@socketio.on('click')
def handle_click(data):
    btn = data.get('btn', 'left')
    if btn == 'left':
        pyautogui.click(_pause=False)
    elif btn == 'right':
        pyautogui.rightClick(_pause=False)

@socketio.on('type')
def handle_type(data):
    text = data.get('text', '')
    press_enter = data.get('pressEnter', False)
    if text:
        import pyperclip
        pyperclip.copy(text)
        pyautogui.hotkey('ctrl', 'v', _pause=False)
        if press_enter:
            pyautogui.press('enter', _pause=False)

@socketio.on('key')
def handle_key(data):
    key = data.get('key', '')
    if key:
        pyautogui.press(key, _pause=False)

@socketio.on('open')
def handle_open(data):
    url = data.get('url', '')
    if url:
        webbrowser.open_new_tab(url)

def get_local_ip():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(('10.255.255.255', 1))
        IP = s.getsockname()[0]
    except Exception:
        IP = '127.0.0.1'
    finally:
        s.close()
    return IP

def print_qr(url):
    qr = qrcode.QRCode()
    qr.add_data(url)
    qr.make()
    print(f"\n--- Remote Controller URL: {url} ---")
    qr.print_ascii(invert=True)
    print("--------------------------------------\n")

if __name__ == '__main__':
    local_ip = get_local_ip()
    port = 5000
    url = f"http://{local_ip}:{port}"
    
    print_qr(url)
    socketio.run(app, host='0.0.0.0', port=port, debug=False)
