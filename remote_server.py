import sys
import os
import json
import tkinter as tk
from tkinter import Button, Label
from threading import Thread
from PIL import Image, ImageTk
import io
from flask import Flask, render_template, request, jsonify
from flask_socketio import SocketIO, emit
import pyautogui
import socket
import webbrowser
import time
import qrcode

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
    clicks = data.get('clicks', 1)
    if btn == 'left':
        pyautogui.click(clicks=clicks, _pause=False)
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

def show_qr_window(url):
    root = tk.Tk()
    root.title("Air Controller Server")
    root.geometry("400x520")
    root.configure(bg='white')

    # 정보 라벨
    Label(root, text="Mobile Air Controller", font=("Helvetica", 16, "bold"), bg='white').pack(pady=(20, 10))
    Label(root, text=f"접속 주소:\n{url}", font=("Helvetica", 12), bg='white', fg='blue').pack(pady=5)

    # QR 코드 생성
    try:
        qr = qrcode.QRCode(box_size=10, border=2)
        qr.add_data(url)
        qr.make(fit=True)
        img = qr.make_image(fill_color="black", back_color="white")
        
        # PIL 이미지를 Tkinter 호환 이미지로 변환
        tk_img = ImageTk.PhotoImage(img)
        
        qr_label = Label(root, image=tk_img, bg='white')
        qr_label.image = tk_img # 참조 유지
        qr_label.pack(pady=10)
    except Exception as e:
        Label(root, text=f"QR 오류: {e}", bg='white').pack()

    # 설명
    Label(root, text="스마트폰으로 QR코드를 스캔하여 접속하세요.", bg='white').pack(pady=10)
    
    # 종료 버튼
    def on_close():
        root.destroy()
        os._exit(0)  # 전체 프로세스 종료

    root.protocol("WM_DELETE_WINDOW", on_close)
    root.mainloop()

if __name__ == '__main__':
    local_ip = get_local_ip()
    port = 5000
    conn_url = f"http://{local_ip}:{port}"
    
    # GUI 스레드 실행
    gui_thread = Thread(target=show_qr_window, args=(conn_url,))
    gui_thread.daemon = True
    gui_thread.start()

    # Flask 서버 실행
    socketio.run(app, host='0.0.0.0', port=port, allow_unsafe_werkzeug=True)
    server_thread = threading.Thread(target=lambda: socketio.run(app, host='0.0.0.0', port=port, debug=False))
    server_thread.daemon = True
    server_thread.start()

    # QR 코드 윈도우 생성 (Tkinter)
    try:
        import tkinter as tk
        from PIL import Image, ImageTk
        import qrcode

        def show_qr_window():
            window = tk.Tk()
            window.title("AirController QR")
            window.geometry("400x450")
            window.configure(bg='white')

            # 설명 라벨
            label_info = tk.Label(window, text=f"Scan to Connect\n{url}", font=("Arial", 14), bg='white')
            label_info.pack(pady=20)

            # QR 코드 생성
            qr = qrcode.QRCode(box_size=10, border=2)
            qr.add_data(url)
            qr.make(fit=True)
            qr_img = qr.make_image(fill="black", back_color="white")
            
            # Tkinter 호환 이미지로 변환
            tk_img = ImageTk.PhotoImage(qr_img)
            
            # 이미지 라벨
            label_img = tk.Label(window, image=tk_img, bg='white')
            label_img.image = tk_img # 참조 유지
            label_img.pack()

            # 닫기 버튼
            btn_close = tk.Button(window, text="Close Window (Server Running)", command=window.destroy, font=("Arial", 10))
            btn_close.pack(pady=20)

            window.mainloop()
            
        print("Starting QR Window...")
        show_qr_window()

    except ImportError as e:
        print(f"GUI Error: {e}")
        # GUI 실패 시 메인 스레드 대기
        while True:
            time.sleep(1)

