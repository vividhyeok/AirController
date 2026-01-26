import tkinter as tk
from tkinter import ttk
import speech_recognition as sr
import threading
import webbrowser
import pyautogui
import pyperclip
import time
import keyboard

class VoiceControllerApp:
    def __init__(self, root):
        self.root = root
        self.root.title("Voice Commander")
        self.root.geometry("400x300")
        
        # 창을 항상 맨 위에 고정 (TV 연결 시 편리함)
        self.root.attributes('-topmost', True)
        # 투명도 조절 (배경이 살짝 보이게)
        self.root.attributes('-alpha', 0.9)

        self.is_listening = False
        self.recognizer = sr.Recognizer()
        self.microphone = sr.Microphone()
        
        # 스타일 설정
        style = ttk.Style()
        style.configure("TButton", font=("맑은 고딕", 10), padding=5)
        style.configure("Toggle.TButton", font=("맑은 고딕", 12, "bold"))
        
        self.create_widgets()

        # 음성 인식 임계값 자동 조정 (초기 1회)
        with self.microphone as source:
            self.recognizer.adjust_for_ambient_noise(source, duration=1)

    def create_widgets(self):
        # 1. 상태 표시 라벨
        self.status_label = tk.Label(self.root, text="대기 중...", font=("맑은 고딕", 12), bg="lightgray", width=40)
        self.status_label.pack(pady=10, fill='x')

        # 2. 음성 인식 토글 버튼
        self.toggle_btn = ttk.Button(self.root, text="음성 입력 시작 (OFF)", style="Toggle.TButton", command=self.toggle_listening)
        self.toggle_btn.pack(pady=10, fill='x', padx=20)

        # 3. 구분선
        ttk.Separator(self.root, orient='horizontal').pack(fill='x', pady=10)

        # 4. 스트림덱 스타일 버튼 그리드 프레임
        btn_frame = tk.Frame(self.root)
        btn_frame.pack(pady=5)

        # 사이트 목록 (이름, URL)
        sites = [
            ("YouTube", "https://www.youtube.com"),
            ("YT Music", "https://music.youtube.com"),
            ("TVWiki", "https://tvwiki.life"), # 도메인이 자주 바뀌므로 주의 필요
            ("Laftel", "https://laftel.net"),
            ("Google", "https://www.google.com"),
            ("Enter Key", "ENTER") # 엔터키 기능 추가
        ]

        # 버튼 생성 (2열 배치)
        for i, (name, url) in enumerate(sites):
            if url == "ENTER":
                btn = ttk.Button(btn_frame, text=name, command=self.press_enter)
            else:
                btn = ttk.Button(btn_frame, text=name, command=lambda u=url: self.open_site(u))
            
            row = i // 2
            col = i % 2
            btn.grid(row=row, column=col, padx=5, pady=5, sticky="ew")

    def toggle_listening(self):
        if not self.is_listening:
            self.is_listening = True
            self.toggle_btn.config(text="음성 입력 중지 (ON)")
            self.status_label.config(text="듣고 있습니다... (말씀하세요)", bg="#ffcccc")
            # 별도 스레드에서 음성 인식 시작
            threading.Thread(target=self.listen_loop, daemon=True).start()
        else:
            self.is_listening = False
            self.toggle_btn.config(text="음성 입력 시작 (OFF)")
            self.status_label.config(text="대기 중...", bg="lightgray")

    def listen_loop(self):
        while self.is_listening:
            try:
                with self.microphone as source:
                    # 너무 오래 기다리지 않도록 timeout 설정
                    try:
                        audio = self.recognizer.listen(source, timeout=3, phrase_time_limit=5)
                    except sr.WaitTimeoutError:
                        continue # 말이 없으면 다시 루프

                    # 음성 인식 처리
                    self.update_status("변환 중...")
                    text = self.recognizer.recognize_google(audio, language='ko-KR')
                    
                    if text:
                        print(f"인식된 텍스트: {text}")
                        self.type_text(text)
                        self.update_status(f"입력: {text}")
                        
            except sr.UnknownValueError:
                # 인식 실패 시 무시
                pass
            except sr.RequestError:
                self.update_status("오류: 인터넷 연결 확인 필요")
            except Exception as e:
                print(f"Error: {e}")
            
            time.sleep(0.1)

    def type_text(self, text):
        """
        한글 입력 문제를 해결하기 위해 클립보드 복사 -> 붙여넣기 방식을 사용합니다.
        pyautogui.write는 한글을 직접 지원하지 않는 경우가 많습니다.
        """
        pyperclip.copy(text + " ") # 편의를 위해 뒤에 띄어쓰기 추가
        pyautogui.hotkey('ctrl', 'v')

    def open_site(self, url):
        webbrowser.open_new_tab(url)
        self.update_status(f"이동: {url}")

    def press_enter(self):
        pyautogui.press('enter')
        self.update_status("Enter 키 입력됨")

    def update_status(self, msg):
        # Tkinter UI 업데이트는 메인 스레드에서 해야 함
        self.root.after(0, lambda: self.status_label.config(text=msg))

if __name__ == "__main__":
    root = tk.Tk()
    app = VoiceControllerApp(root)
    root.mainloop()
