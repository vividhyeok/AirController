import sys
import os
import base64
import io
import subprocess
from flask import Flask, render_template, request, jsonify
from flask_socketio import SocketIO, emit
import pyautogui
import socket
import qrcode
import webbrowser
import threading
import time
import ctypes

# Flask 앱과 SocketIO 초기화
if getattr(sys, 'frozen', False):
    template_folder = os.path.join(sys._MEIPASS, 'templates')
    app = Flask(__name__, template_folder=template_folder)
else:
    app = Flask(__name__)

# Sleep Reservation Thread Global
sleep_timer = None

def get_active_window_title():
    try:
        GetForegroundWindow = ctypes.windll.user32.GetForegroundWindow
        GetWindowTextLength = ctypes.windll.user32.GetWindowTextLengthW
        GetWindowText = ctypes.windll.user32.GetWindowTextW

        hwnd = GetForegroundWindow()
        length = GetWindowTextLength(hwnd)
        buff = ctypes.create_unicode_buffer(length + 1)
        GetWindowText(hwnd, buff, length + 1)
        return buff.value
    except:
        return "Unknown"

# cors_allowed_origins='*' 는 로컬 네트워크의 다른 기기 접속 허용
socketio = SocketIO(app, cors_allowed_origins='*', async_mode='threading')

# 마우스 안전모드 해제 (화면 구석으로 가면 멈추는 기능 해제)
pyautogui.FAILSAFE = False

@app.route('/')
def index():
    local_ip = get_local_ip()
    port = app.config.get('PORT', 5000)
    server_url = f"http://{local_ip}:{port}"
    return render_template('index.html', server_url=server_url)

@app.route('/qr')
def qr_page():
    controller_url = app.config.get('CONTROLLER_URL')
    if not controller_url:
        local_ip = get_local_ip()
        port = app.config.get('PORT', 5000)
        controller_url = f"http://{local_ip}:{port}"

    qr = qrcode.QRCode(border=1)
    qr.add_data(controller_url)
    qr.make(fit=True)
    img = qr.make_image(fill_color="black", back_color="white")

    buffer = io.BytesIO()
    img.save(buffer, format='PNG')
    qr_data = base64.b64encode(buffer.getvalue()).decode('utf-8')

    return render_template('qr.html', controller_url=controller_url, qr_data=qr_data)

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

@socketio.on('hotkey')
def handle_hotkey(data):
    keys = data.get('keys', [])
    if keys:
        pyautogui.hotkey(*keys, _pause=False)

@socketio.on('system')
def handle_system(data):
    global sleep_timer
    action = data.get('action', '')
    delay = data.get('delay', 0)  # minutes
    
    if action == 'sleep' and sys.platform.startswith('win'):
        if delay > 0:
            def do_sleep():
                time.sleep(delay * 60)
                subprocess.run(
                    ["rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0"],
                    shell=True,
                    check=False,
                )
            
            if sleep_timer and sleep_timer.is_alive():
                # Already have a timer? Maybe cancel it? 
                # For now, let's just start a new one.
                pass
            
            sleep_timer = threading.Thread(target=do_sleep, daemon=True)
            sleep_timer.start()
            emit('system_status', {'message': f'{delay}분 후 절전 모드로 전환됩니다.'})
        else:
            subprocess.run(
                ["rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0"],
                shell=True,
                check=False,
            )

@socketio.on('get_current_tab')
def handle_get_current_tab():
    try:
        import pyperclip
        # Save current clipboard
        old_clipboard = pyperclip.paste()
        
        # Try to get URL from address bar (Ctrl+L, Ctrl+C)
        pyautogui.hotkey('ctrl', 'l', _pause=False)
        time.sleep(0.2)
        pyautogui.hotkey('ctrl', 'c', _pause=False)
        time.sleep(0.2)
        
        url = pyperclip.paste()
        title = get_active_window_title()
        
        # If the clipboard didn't change or doesn't look like a URL, 
        # it might just be the window title or old clipboard content.
        if not (url.startswith('http://') or url.startswith('https://') or url.startswith('www.')):
            url = ""
            
        emit('current_tab', {'url': url, 'title': title})
        
        # Restore old clipboard (optional, but polite)
        # pyperclip.copy(old_clipboard)
    except Exception as e:
        emit('current_tab', {'error': str(e)})

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
    try:
        qr.print_ascii(invert=True)
    except UnicodeEncodeError:
        for row in qr.get_matrix():
            print(''.join('##' if cell else '  ' for cell in row))
    print("--------------------------------------\n")

if __name__ == '__main__':
    local_ip = get_local_ip()
    port = 5000
    url = f"http://{local_ip}:{port}"

    app.config['CONTROLLER_URL'] = url
    app.config['PORT'] = port
    
    print_qr(url)
    try:
        webbrowser.open_new_tab(f"http://127.0.0.1:{port}/qr")
    except Exception:
        pass
    socketio.run(app, host='0.0.0.0', port=port, debug=False, allow_unsafe_werkzeug=True)
