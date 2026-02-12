package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	qrcode "github.com/skip2/go-qrcode"
)

// ─── Embed Templates ────────────────────────────────────────────

//go:embed templates/*
var templatesFS embed.FS

// ─── Windows API ────────────────────────────────────────────────

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSetCursorPos         = user32.NewProc("SetCursorPos")
	procGetCursorPos         = user32.NewProc("GetCursorPos")
	procMouseEvent           = user32.NewProc("mouse_event")
	procKeybdEvent           = user32.NewProc("keybd_event")
	procGetForegroundWindow  = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procOpenClipboard        = user32.NewProc("OpenClipboard")
	procCloseClipboard       = user32.NewProc("CloseClipboard")
	procGetClipboardData     = user32.NewProc("GetClipboardData")
	procSetClipboardData     = user32.NewProc("SetClipboardData")
	procEmptyClipboard       = user32.NewProc("EmptyClipboard")
	procGlobalAlloc          = kernel32.NewProc("GlobalAlloc")
	procGlobalFree           = kernel32.NewProc("GlobalFree")
	procGlobalLock           = kernel32.NewProc("GlobalLock")
	procGlobalUnlock         = kernel32.NewProc("GlobalUnlock")
)

// ─── Constants ──────────────────────────────────────────────────

const (
	MOUSEEVENTF_LEFTDOWN  = 0x0002
	MOUSEEVENTF_LEFTUP    = 0x0004
	MOUSEEVENTF_RIGHTDOWN = 0x0008
	MOUSEEVENTF_RIGHTUP   = 0x0010
	MOUSEEVENTF_WHEEL     = 0x0800
	KEYEVENTF_KEYUP       = 0x0002
	CF_UNICODETEXT        = 13
	GMEM_MOVEABLE         = 0x0002
)

// ─── Virtual Key Codes ──────────────────────────────────────────

var vkMap = map[string]uint8{
	// Control keys
	"enter": 0x0D, "backspace": 0x08, "tab": 0x09,
	"esc": 0x1B, "escape": 0x1B, "space": 0x20,
	"delete": 0x2E, "home": 0x24, "end": 0x23,
	"pageup": 0x21, "pagedown": 0x22,
	// Arrow keys
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	// Modifier keys
	"win": 0x5B, "ctrl": 0x11, "alt": 0x12, "shift": 0x10,
	// Function keys
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73,
	"f5": 0x74, "f6": 0x75, "f7": 0x76, "f8": 0x77,
	"f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
	// Media keys
	"volumeup": 0xAF, "volumedown": 0xAE, "volumemute": 0xAD,
	"medianexttrack": 0xB0, "mediaprevtrack": 0xB1,
	"mediastop": 0xB2, "mediaplaypause": 0xB3,
	// Browser keys
	"browserhome": 0xAC, "browserback": 0xA6,
	"browserforward": 0xA7, "browserrefresh": 0xA8,
	// Alphabet
	"a": 0x41, "b": 0x42, "c": 0x43, "d": 0x44, "e": 0x45, "f": 0x46,
	"g": 0x47, "h": 0x48, "i": 0x49, "j": 0x4A, "k": 0x4B, "l": 0x4C,
	"m": 0x4D, "n": 0x4E, "o": 0x4F, "p": 0x50, "q": 0x51, "r": 0x52,
	"s": 0x53, "t": 0x54, "u": 0x55, "v": 0x56, "w": 0x57, "x": 0x58,
	"y": 0x59, "z": 0x5A,
	// Numbers
	"0": 0x30, "1": 0x31, "2": 0x32, "3": 0x33, "4": 0x34,
	"5": 0x35, "6": 0x36, "7": 0x37, "8": 0x38, "9": 0x39,
}

// ─── Types ──────────────────────────────────────────────────────

type POINT struct {
	X, Y int32
}

type WSMessage struct {
	Event string                 `json:"event"`
	Data  map[string]interface{} `json:"data"`
}

type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *Client) SendEvent(event string, data interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := map[string]interface{}{"event": event, "data": data}
	c.conn.WriteJSON(msg)
}

// ─── Mouse Functions ────────────────────────────────────────────

func getCursorPos() (int, int) {
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

func moveRelative(dx, dy float64) {
	cx, cy := getCursorPos()
	procSetCursorPos.Call(uintptr(cx+int(dx)), uintptr(cy+int(dy)))
}

func mouseClick(button string) {
	switch button {
	case "left":
		procMouseEvent.Call(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
		procMouseEvent.Call(MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
	case "right":
		procMouseEvent.Call(MOUSEEVENTF_RIGHTDOWN, 0, 0, 0, 0)
		procMouseEvent.Call(MOUSEEVENTF_RIGHTUP, 0, 0, 0, 0)
	}
}

func mouseScroll(dy int) {
	amount := int32(dy * 120)
	procMouseEvent.Call(MOUSEEVENTF_WHEEL, 0, 0, uintptr(amount), 0)
}

// ─── Keyboard Functions ─────────────────────────────────────────

func keyDown(vk uint8) {
	procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
}

func keyUp(vk uint8) {
	procKeybdEvent.Call(uintptr(vk), 0, KEYEVENTF_KEYUP, 0)
}

func pressKey(name string) {
	name = strings.ToLower(name)
	if vk, ok := vkMap[name]; ok {
		keyDown(vk)
		keyUp(vk)
	}
}

func doHotkey(keys []string) {
	var pressed []uint8
	for _, k := range keys {
		if vk, ok := vkMap[strings.ToLower(k)]; ok {
			keyDown(vk)
			pressed = append(pressed, vk)
		}
	}
	for i := len(pressed) - 1; i >= 0; i-- {
		keyUp(pressed[i])
	}
}

// ─── Clipboard Functions (Windows API) ──────────────────────────

func getClipboardText() string {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	handle, _, _ := procGetClipboardData.Call(CF_UNICODETEXT)
	if handle == 0 {
		return ""
	}

	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(handle)

	var runes []uint16
	for i := uintptr(0); ; i += 2 {
		ch := *(*uint16)(unsafe.Pointer(ptr + i))
		if ch == 0 {
			break
		}
		runes = append(runes, ch)
	}
	return syscall.UTF16ToString(runes)
}

func setClipboardText(text string) {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	utf16, _ := syscall.UTF16FromString(text)
	size := len(utf16) * 2
	handle, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(size))
	if handle == 0 {
		return
	}

	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		procGlobalFree.Call(handle)
		return
	}

	for i, ch := range utf16 {
		*(*uint16)(unsafe.Pointer(ptr + uintptr(i*2))) = ch
	}
	procGlobalUnlock.Call(handle)
	procSetClipboardData.Call(CF_UNICODETEXT, handle)
}

// ─── Text Typing (via Clipboard) ────────────────────────────────

func typeText(text string, pressEnter bool) {
	setClipboardText(text)
	time.Sleep(50 * time.Millisecond)
	doHotkey([]string{"ctrl", "v"})
	if pressEnter {
		time.Sleep(50 * time.Millisecond)
		pressKey("enter")
	}
}

// ─── Window Title ───────────────────────────────────────────────

func getActiveWindowTitle() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "Unknown"
	}
	length, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if length == 0 {
		return "Unknown"
	}
	buf := make([]uint16, length+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(length+1))
	return syscall.UTF16ToString(buf)
}

// ─── Network ────────────────────────────────────────────────────

func getLocalIP() string {
	// 1차: 실제 라우팅 가능한 IP를 UDP 다이얼로 확인
	if conn, err := net.DialTimeout("udp4", "8.8.8.8:80", 2*time.Second); err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return addr.IP.String()
		}
	}

	// 2차: 네트워크 인터페이스 순회 (169.254.x.x 링크로컬 제외)
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			// 169.254.x.x (APIPA/링크로컬) 제외
			if ip4[0] == 169 && ip4[1] == 254 {
				continue
			}
			return ip4.String()
		}
	}
	return "127.0.0.1"
}

// ─── WebSocket Handler ──────────────────────────────────────────

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	lastMoveTime time.Time
	moveMu       sync.Mutex
	moveInterval = 10 * time.Millisecond
)

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	client := &Client{conn: conn}
	log.Println("Client connected")

	for {
		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Client disconnected")
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			continue
		}

		handleEvent(client, msg)
	}
}

func handleEvent(client *Client, msg WSMessage) {
	data := msg.Data

	switch msg.Event {
	case "move":
		moveMu.Lock()
		now := time.Now()
		if now.Sub(lastMoveTime) < moveInterval {
			moveMu.Unlock()
			return
		}
		lastMoveTime = now
		moveMu.Unlock()

		dx, _ := toFloat(data["dx"])
		dy, _ := toFloat(data["dy"])
		moveRelative(dx, dy)

	case "scroll":
		dy, _ := toFloat(data["dy"])
		mouseScroll(int(dy))

	case "click":
		btn, _ := data["btn"].(string)
		if btn == "" {
			btn = "left"
		}
		mouseClick(btn)

	case "type":
		text, _ := data["text"].(string)
		pe, _ := data["pressEnter"].(bool)
		if text != "" {
			typeText(text, pe)
		}

	case "key":
		key, _ := data["key"].(string)
		if key != "" {
			pressKey(key)
		}

	case "hotkey":
		keysRaw, ok := data["keys"].([]interface{})
		if !ok {
			return
		}
		keys := make([]string, len(keysRaw))
		for i, k := range keysRaw {
			keys[i], _ = k.(string)
		}
		doHotkey(keys)

	case "system":
		action, _ := data["action"].(string)
		delay, _ := toFloat(data["delay"])

		if action == "sleep" {
			if delay > 0 {
				go func() {
					time.Sleep(time.Duration(delay) * time.Minute)
					exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0").Run()
				}()
				client.SendEvent("system_status", map[string]string{
					"message": fmt.Sprintf("%.0f분 후 절전 모드로 전환됩니다.", delay),
				})
			} else {
				exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0").Run()
			}
		}

	case "get_current_tab":
		oldClip := getClipboardText()

		doHotkey([]string{"ctrl", "l"})
		time.Sleep(200 * time.Millisecond)
		doHotkey([]string{"ctrl", "c"})
		time.Sleep(200 * time.Millisecond)

		url := getClipboardText()
		title := getActiveWindowTitle()

		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = ""
		}

		client.SendEvent("current_tab", map[string]string{
			"url":   url,
			"title": title,
		})

		if oldClip != "" {
			setClipboardText(oldClip)
		}

	case "open":
		urlVal, _ := data["url"].(string)
		if urlVal != "" {
			exec.Command("rundll32", "url.dll,FileProtocolHandler", urlVal).Start()
		}
	}
}

// ─── Helpers ────────────────────────────────────────────────────

func toFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// ─── Main ───────────────────────────────────────────────────────

func main() {
	localIP := getLocalIP()
	port := "5000"
	serverURL := fmt.Sprintf("http://%s:%s", localIP, port)

	// Parse templates
	indexTmpl := template.Must(template.ParseFS(templatesFS, "templates/index.html"))
	qrTmpl := template.Must(template.ParseFS(templatesFS, "templates/qr.html"))

	// Routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		indexTmpl.Execute(w, map[string]string{"ServerURL": serverURL})
	})

	http.HandleFunc("/qr", func(w http.ResponseWriter, r *http.Request) {
		png, _ := qrcode.Encode(serverURL, qrcode.Medium, 256)
		qrData := base64.StdEncoding.EncodeToString(png)
		qrTmpl.Execute(w, map[string]string{
			"ControllerURL": serverURL,
			"QRData":        qrData,
		})
	})

	http.HandleFunc("/ws", handleWS)

	// Print server info
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Printf("║   Air Controller: %-23s║\n", serverURL)
	fmt.Println("╠══════════════════════════════════════════╣")
	fmt.Println("║   Scan QR code or open the URL above    ║")
	fmt.Println("║   on your smartphone to connect.        ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	// Open QR page in default browser
	exec.Command("rundll32", "url.dll,FileProtocolHandler",
		fmt.Sprintf("http://127.0.0.1:%s/qr", port)).Start()

	log.Printf("Server running at %s\n", serverURL)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
