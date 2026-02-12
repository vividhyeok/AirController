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
import ctypes

# Flask ?±Í≥º SocketIO Ï¥àÍ∏∞??
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

# cors_allowed_origins='*' ??Î°úÏª¨ ?§Ìä∏?åÌÅ¨???§Î•∏ Í∏∞Í∏∞ ?ëÏÜç ?àÏö©
socketio = SocketIO(app, cors_allowed_origins='*', async_mode='threading')

# ÎßàÏö∞???àÏ†ÑÎ™®Îìú ?¥Ï†ú
pyautogui.FAILSAFE = False

# ÎßàÏö∞???¥Îèô ?§Î°ú?ÄÎßÅÏùÑ ?ÑÌïú ?úÍ∞Ñ ?Ä??
last_move_time = 0
MOVE_INTERVAL = 0.01  # ??100Hz (CPU Î∂Ä??Í∞êÏÜå)

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

# === WebSocket ?¥Î≤§???∏Îì§??===

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
            emit('system_status', {'message': f'{delay}Î∂????àÏ†Ñ Î™®ÎìúÎ°??ÑÌôò?©Îãà??'})
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
        sites = load_sites()
        
        # Ï§ëÎ≥µ ?úÍ±∞ ??Îß??ûÏóê Ï∂îÍ? (?ÑÏ≤¥ URL ?Ä??
        if url in sites['history']:
            sites['history'].remove(url)
        sites['history'].insert(0, url)
        
        # ÏµúÎ? 20Í∞??†Ï?
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
    name = data.get('name', '??Ï¶êÍ≤®Ï∞æÍ∏∞')
    url = data.get('url', '')
    icon = data.get('icon', '‚≠?)
    if not url: return
    
    sites = load_sites()
    # Ï§ëÎ≥µ Ï≤¥ÌÅ¨ Î∞??ÖÎç∞?¥Ìä∏
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
        emit('system_status', {'msg': 'AI Î∂ÑÏÑù Í∏∞Îä•??ÎπÑÌôú?±Ìôî?òÏñ¥ ?àÏäµ?àÎã§.'})
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
            raise Exception("AI ?ëÎãµ ?¥ÏÑù ?§Ìå®")
            
    except Exception as e:
        emit('system_status', {'msg': f'AI Î∂ÑÏÑù ?§Ìå®: {str(e)}'})

@socketio.on('sync_from_pc_silent')
def handle_sync_from_pc_silent():
    # URLÎß?Í∞Ä?∏Ï????∞Ïóê ?åÎ¶º (Î∂ÑÏÑù??
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
    # Í∏∞Ï°¥ ?¥Î¶ΩÎ≥¥Îìú ?Ä??
    old_clipboard = pyperclip.paste()
    
    # Î∏åÎùº?∞Ï? Ï£ºÏÜåÏ∞??¨Ïª§??Î∞?Î≥µÏÇ¨ ?úÎèÑ (Ctrl+L, Ctrl+C)
    pyautogui.hotkey('ctrl', 'l', _pause=False)
    time.sleep(0.1)
    pyautogui.hotkey('ctrl', 'c', _pause=False)
    time.sleep(0.1)
    
    current_url = pyperclip.paste().strip()
    
    # URL ?ïÏãù???ÑÎãàÎ©??¥Î¶ΩÎ≥¥Îìú ?êÎ≥µ Î∞?Ï∑®ÏÜå
    if not re.match(r'^https?://', current_url):
        pyperclip.copy(old_clipboard)
        emit('system_status', {'msg': 'Î∏åÎùº?∞Ï??êÏÑú URL??Í∞Ä?∏Ïò§ÏßÄ Î™ªÌñà?µÎãà?? (Ï£ºÏÜåÏ∞ΩÏùÑ ?†ÌÉù?¥Ï£º?∏Ïöî)'})
        return

    # ?∞ÏúºÎ°?URL ?ÑÏÜ°
    emit('open_on_mobile', {'url': current_url})
    
    # ?àÏä§?†Î¶¨??Ï∂îÍ?
    domain = extract_domain(current_url)
    sites = load_sites()
    if domain in sites['history']:
        sites['history'].remove(domain)
    sites['history'].insert(0, domain)
    sites['history'] = sites['history'][:15]
    save_sites(sites)
    emit('sync_sites', sites, broadcast=True)
    
    # ?¥Î¶ΩÎ≥¥Îìú ?êÎ≥µ
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
