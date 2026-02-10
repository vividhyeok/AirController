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
import signal
import threading
import time
import json
import re
import pyperclip
import requests

# 전역 상태 관리
active_connections = 0
idle_timer = None
IDLE_TIMEOUT = 300  # 5분 (초 단위)
shutdown_timer = None
SITES_FILE = 'sites.json'
AI_CONFIG_FILE = 'ai_config.json'

def load_sites():
    if os.path.exists(SITES_FILE):
        try:
            with open(SITES_FILE, 'r', encoding='utf-8') as f:
                data = json.load(f)
                # 마이그레이션: 히스토리가 도메인만 있다면 리스트 유지, 즐겨찾기가 리스트면 객체 리스트로 변환
                if 'history' not in data: data['history'] = []
                if 'favorites' not in data: data['favorites'] = []
                if data['favorites'] and isinstance(data['favorites'][0], str):
                    data['favorites'] = [{"name": extract_domain(url).replace("https://","").replace("http://",""), "url": url, "icon": "⭐"} for url in data['favorites']]
                return data
        except Exception:
            pass
    return {"history": [], "favorites": []}

def save_sites(data):
    try:
        with open(SITES_FILE, 'w', encoding='utf-8') as f:
            json.dump(data, f, ensure_ascii=False, indent=4)
    except Exception as e:
        print(f"Error saving sites: {e}")

def load_ai_config():
    if os.path.exists(AI_CONFIG_FILE):
        try:
            with open(AI_CONFIG_FILE, 'r', encoding='utf-8') as f:
                return json.load(f)
        except Exception:
            pass
    return {
        "api_keys": {"openai": "", "google": "", "deepseek": ""},
        "selected_model": "gemini-1.5-flash",
        "mappings": {},
        "usage": {"total_krw": 0, "last_cost": 0},
        "enabled": False
    }

def save_ai_config(config):
    try:
        with open(AI_CONFIG_FILE, 'w', encoding='utf-8') as f:
            json.dump(config, f, ensure_ascii=False, indent=4)
    except Exception:
        pass

def extract_domain(url):
    try:
        # 도메인만 추출 (프로토콜 포함)
        match = re.search(r'https?://[^/]+', url)
        if match:
            return match.group(0)
    except Exception:
        pass
    return url

def exit_gracefully():
    print("Shutting down server...")
    os._exit(0)

def start_idle_timer():
    global idle_timer
    if idle_timer:
        idle_timer.cancel()
    idle_timer = threading.Timer(IDLE_TIMEOUT, exit_gracefully)
    idle_timer.start()

def stop_idle_timer():
    global idle_timer
    if idle_timer:
        idle_timer.cancel()
        idle_timer = None

# Flask 앱과 SocketIO 초기화
if getattr(sys, 'frozen', False):
    template_folder = os.path.join(sys._MEIPASS, 'templates')
    app = Flask(__name__, template_folder=template_folder)
else:
    app = Flask(__name__)

# cors_allowed_origins='*' 는 로컬 네트워크의 다른 기기 접속 허용
socketio = SocketIO(app, cors_allowed_origins='*', async_mode='threading')

# 마우스 안전모드 해제
pyautogui.FAILSAFE = False

# 마우스 이동 스로틀링을 위한 시간 저장
last_move_time = 0
MOVE_INTERVAL = 0.01  # 약 100Hz (CPU 부하 감소)

@app.route('/')
def index():
    return render_template('index.html')

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
    global active_connections
    active_connections += 1
    stop_idle_timer()
    print(f'Client connected. Active: {active_connections}')

@socketio.on('disconnect')
def handle_disconnect():
    global active_connections
    active_connections -= 1
    print(f'Client disconnected. Active: {active_connections}')
    if active_connections <= 0:
        start_idle_timer()

@socketio.on('move')
def handle_move(data):
    global last_move_time
    current_time = time.time()
    if current_time - last_move_time < MOVE_INTERVAL:
        return
    
    try:
        dx = float(data.get('dx', 0))
        dy = float(data.get('dy', 0))
        pyautogui.moveRel(dx, dy, _pause=False)
        last_move_time = current_time
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
    global shutdown_timer
    action = data.get('action', '')
    
    if action == 'sleep' and sys.platform.startswith('win'):
        # 절전 모드 진입 전 서버 종료 예약
        threading.Timer(2.0, exit_gracefully).start()
        subprocess.run(["rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0"], shell=True)
        
    elif action == 'shutdown' and sys.platform.startswith('win'):
        subprocess.run(["shutdown", "/s", "/t", "60"], shell=True)
        emit('system_status', {'msg': 'PC가 60초 후 종료됩니다.'}, broadcast=True)
        
    elif action == 'restart' and sys.platform.startswith('win'):
        subprocess.run(["shutdown", "/r", "/t", "60"], shell=True)
        emit('system_status', {'msg': 'PC가 60초 후 재시작됩니다.'}, broadcast=True)
        
    elif action == 'cancel_shutdown' and sys.platform.startswith('win'):
        subprocess.run(["shutdown", "/a"], shell=True)
        if shutdown_timer:
            shutdown_timer.cancel()
            shutdown_timer = None
        emit('system_status', {'msg': '종료 예약이 취소되었습니다.'}, broadcast=True)
        
    elif action == 'timer' and sys.platform.startswith('win'):
        minutes = int(data.get('min', 0))
        if minutes > 0:
            seconds = minutes * 60
            subprocess.run(["shutdown", "/s", "/t", str(seconds)], shell=True)
            emit('system_status', {'msg': f'{minutes}분 후 종료 예약되었습니다.'}, broadcast=True)

    elif action == 'lock' and sys.platform.startswith('win'):
        subprocess.run(["rundll32.exe", "user32.dll,LockWorkStation"], shell=True)

    elif action == 'close_server':
        exit_gracefully()

@socketio.on('open')
def handle_open(data):
    url = data.get('url', '')
    if url:
        sites = load_sites()
        
        # 중복 제거 후 맨 앞에 추가 (전체 URL 저장)
        if url in sites['history']:
            sites['history'].remove(url)
        sites['history'].insert(0, url)
        
        # 최대 20개 유지
        sites['history'] = sites['history'][:20]
        
        save_sites(sites)
        emit('sync_sites', sites, broadcast=True)
        webbrowser.open_new_tab(url)

@socketio.on('get_sites')
def handle_get_sites():
    emit('sync_sites', load_sites())

@socketio.on('add_favorite')
def handle_add_favorite(data):
    # data: { name, url, icon }
    name = data.get('name', '새 즐겨찾기')
    url = data.get('url', '')
    icon = data.get('icon', '⭐')
    if not url: return
    
    sites = load_sites()
    # 중복 체크 및 업데이트
    for fav in sites['favorites']:
        if fav['url'] == url:
            fav['name'] = name
            fav['icon'] = icon
            break
    else:
        sites['favorites'].append({"name": name, "url": url, "icon": icon})
    
    save_sites(sites)
    emit('sync_sites', sites, broadcast=True)

@socketio.on('delete_site')
def handle_delete_site(data):
    url = data.get('url', '')
    stype = data.get('type', 'history') # 'history' or 'favorites'
    if not url: return
    
    sites = load_sites()
    if stype == 'history' and url in sites['history']:
        sites['history'].remove(url)
    elif stype == 'favorites':
        sites['favorites'] = [f for f in sites['favorites'] if f['url'] != url]
        
    save_sites(sites)
    emit('sync_sites', sites, broadcast=True)

@socketio.on('get_domain_info')
def handle_get_domain_info(data):
    url = data.get('url', '')
    if url:
        domain = extract_domain(url)
        emit('domain_info', {'url': url, 'domain': domain})

@socketio.on('save_ai_config')
def handle_save_ai_config(data):
    config = load_ai_config()
    if 'api_keys' in data: config['api_keys'].update(data['api_keys'])
    if 'selected_model' in data: config['selected_model'] = data['selected_model']
    if 'enabled' in data: config['enabled'] = data['enabled']
    save_ai_config(config)
    emit('ai_config_status', config)

@socketio.on('get_ai_config')
def handle_get_ai_config():
    emit('ai_config_status', load_ai_config())

@socketio.on('analyze_site')
def handle_analyze_site(data):
    url = data.get('url', '')
    if not url: return
    
    config = load_ai_config()
    if not config['enabled']:
        emit('system_status', {'msg': 'AI 분석 기능이 비활성화되어 있습니다.'})
        return
        
    model = config['selected_model']
    prompt = f"""Analyze the website '{url}' and provide a JSON mapping of common media/navigation keyboard shortcuts for this specific site on Windows.
    Format your response ONLY as a JSON object with keys:
    - "playpause": (e.g., "k" for YouTube, "space" for others)
    - "next": (next video/track shortcut)
    - "prev": (previous video/track shortcut)
    - "fwd": (forward e.g. "l" or "right")
    - "rewd": (rewind e.g. "j" or "left")
    - "fullscreen": (e.g. "f")
    - "mute": (e.g. "m")
    
    If a shortcut is not well-known or exists, set its value to null.
    Also provide a short "site_name" and "reasoning".
    """

    mapping = None
    cost_krw = 0

    try:
        if "gemini" in model:
            key = config['api_keys'].get('google')
            if not key: raise Exception("Google API Key missing")
            api_url = f"https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key={key}"
            res = requests.post(api_url, json={"contents": [{"parts": [{"text": prompt}]}]})
            res_data = res.json()
            text = res_data['candidates'][0]['content']['parts'][0]['text']
            # Extract JSON from potential Markdown formatting
            json_match = re.search(r'\{.*\}', text, re.DOTALL)
            if json_match:
                mapping = json.loads(json_match.group())
            cost_krw = 10 # Approximate for flash
            
        elif "gpt" in model:
            key = config['api_keys'].get('openai')
            if not key: raise Exception("OpenAI API Key missing")
            headers = {"Authorization": f"Bearer {key}", "Content-Type": "application/json"}
            payload = {
                "model": "gpt-4o-mini",
                "messages": [{"role": "user", "content": prompt}],
                "response_format": {"type": "json_object"}
            }
            res = requests.post("https://api.openai.com/v1/chat/completions", headers=headers, json=payload)
            res_data = res.json()
            mapping = json.loads(res_data['choices'][0]['message']['content'])
            # ~0.01$ = 13 KRW
            cost_krw = 15
            
        elif "deepseek" in model:
            key = config['api_keys'].get('deepseek')
            if not key: raise Exception("DeepSeek API Key missing")
            headers = {"Authorization": f"Bearer {key}", "Content-Type": "application/json"}
            payload = {
                "model": "deepseek-chat",
                "messages": [{"role": "user", "content": prompt}]
            }
            res = requests.post("https://api.deepseek.com/v1/chat/completions", headers=headers, json=payload)
            res_data = res.json()
            text = res_data['choices'][0]['message']['content']
            json_match = re.search(r'\{.*\}', text, re.DOTALL)
            if json_match:
                mapping = json.loads(json_match.group())
            cost_krw = 5

        if mapping:
            config['mappings'][url] = mapping
            config['usage']['total_krw'] += cost_krw
            config['usage']['last_cost'] = cost_krw
            save_ai_config(config)
            emit('ai_mapping_result', {'url': url, 'mapping': mapping, 'usage': config['usage']})
        else:
            raise Exception("AI 응답 해석 실패")
            
    except Exception as e:
        emit('system_status', {'msg': f'AI 분석 실패: {str(e)}'})

@socketio.on('sync_from_pc_silent')
def handle_sync_from_pc_silent():
    # URL만 가져와서 폰에 알림 (분석용)
    pyautogui.hotkey('ctrl', 'l', _pause=False)
    time.sleep(0.1)
    pyautogui.hotkey('ctrl', 'c', _pause=False)
    time.sleep(0.1)
    url = pyperclip.paste().strip()
    if re.match(r'^https?://', url):
        emit('pc_url_received', {'url': url})

@socketio.on('sync_from_pc')
def handle_sync_from_pc():
    import pyperclip
    # 기존 클립보드 저장
    old_clipboard = pyperclip.paste()
    
    # 브라우저 주소창 포커스 및 복사 시도 (Ctrl+L, Ctrl+C)
    pyautogui.hotkey('ctrl', 'l', _pause=False)
    time.sleep(0.1)
    pyautogui.hotkey('ctrl', 'c', _pause=False)
    time.sleep(0.1)
    
    current_url = pyperclip.paste().strip()
    
    # URL 형식이 아니면 클립보드 원복 및 취소
    if not re.match(r'^https?://', current_url):
        pyperclip.copy(old_clipboard)
        emit('system_status', {'msg': '브라우저에서 URL을 가져오지 못했습니다. (주소창을 선택해주세요)'})
        return

    # 폰으로 URL 전송
    emit('open_on_mobile', {'url': current_url})
    
    # 히스토리에 추가
    domain = extract_domain(current_url)
    sites = load_sites()
    if domain in sites['history']:
        sites['history'].remove(domain)
    sites['history'].insert(0, domain)
    sites['history'] = sites['history'][:15]
    save_sites(sites)
    emit('sync_sites', sites, broadcast=True)
    
    # 클립보드 원복
    pyperclip.copy(old_clipboard)

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
