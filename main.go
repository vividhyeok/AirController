package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed templates/*
var templatesFS embed.FS

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

const (
	defaultPort          = 5000
	portAttempts         = 20
	maxMessageBytes      = 32 * 1024
	maxTextRunes         = 10000
	maxClipboardRunes    = 1 << 20
	maxHotkeyKeys        = 8
	maxMoveDelta         = 250.0
	maxScrollDelta       = 10
	clipboardRetryCount  = 10
	clipboardRetryWaitMS = 20
)

var (
	writeWait    = 5 * time.Second
	pongWait     = 60 * time.Second
	pingInterval = 30 * time.Second

	lastMoveTime time.Time
	moveMu       sync.Mutex
	moveInterval = 10 * time.Millisecond
	clipboardMu  sync.Mutex
)

var vkMap = map[string]uint8{
	"enter": 0x0D, "backspace": 0x08, "tab": 0x09,
	"esc": 0x1B, "escape": 0x1B, "space": 0x20,
	"delete": 0x2E, "home": 0x24, "end": 0x23,
	"pageup": 0x21, "pagedown": 0x22,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	"win": 0x5B, "ctrl": 0x11, "alt": 0x12, "shift": 0x10,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73,
	"f5": 0x74, "f6": 0x75, "f7": 0x76, "f8": 0x77,
	"f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
	"volumeup": 0xAF, "volumedown": 0xAE, "volumemute": 0xAD,
	"medianexttrack": 0xB0, "mediaprevtrack": 0xB1,
	"mediastop": 0xB2, "mediaplaypause": 0xB3,
	"browserhome": 0xAC, "browserback": 0xA6,
	"browserforward": 0xA7, "browserrefresh": 0xA8,
	"a": 0x41, "b": 0x42, "c": 0x43, "d": 0x44, "e": 0x45, "f": 0x46,
	"g": 0x47, "h": 0x48, "i": 0x49, "j": 0x4A, "k": 0x4B, "l": 0x4C,
	"m": 0x4D, "n": 0x4E, "o": 0x4F, "p": 0x50, "q": 0x51, "r": 0x52,
	"s": 0x53, "t": 0x54, "u": 0x55, "v": 0x56, "w": 0x57, "x": 0x58,
	"y": 0x59, "z": 0x5A,
	"0": 0x30, "1": 0x31, "2": 0x32, "3": 0x33, "4": 0x34,
	"5": 0x35, "6": 0x36, "7": 0x37, "8": 0x38, "9": 0x39,
}

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

func (c *Client) SendEvent(event string, data interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return c.conn.WriteJSON(map[string]interface{}{"event": event, "data": data})
}

func (c *Client) ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.PingMessage, nil)
}

func getCursorPos() (int, int, bool) {
	var pt POINT
	ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if ret == 0 {
		return 0, 0, false
	}
	return int(pt.X), int(pt.Y), true
}

func moveRelative(dx, dy float64) {
	if !isFinite(dx) || !isFinite(dy) {
		return
	}
	dx = clampFloat(dx, -maxMoveDelta, maxMoveDelta)
	dy = clampFloat(dy, -maxMoveDelta, maxMoveDelta)
	if math.Abs(dx) < 0.01 && math.Abs(dy) < 0.01 {
		return
	}

	cx, cy, ok := getCursorPos()
	if !ok {
		return
	}
	procSetCursorPos.Call(uintptr(cx+int(math.Round(dx))), uintptr(cy+int(math.Round(dy))))
}

func mouseClick(button string) {
	switch strings.ToLower(strings.TrimSpace(button)) {
	case "right":
		procMouseEvent.Call(MOUSEEVENTF_RIGHTDOWN, 0, 0, 0, 0)
		time.Sleep(8 * time.Millisecond)
		procMouseEvent.Call(MOUSEEVENTF_RIGHTUP, 0, 0, 0, 0)
	default:
		procMouseEvent.Call(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
		time.Sleep(8 * time.Millisecond)
		procMouseEvent.Call(MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
	}
}

func mouseScroll(dy int) {
	dy = clampInt(dy, -maxScrollDelta, maxScrollDelta)
	if dy == 0 {
		return
	}
	amount := int32(dy * 120)
	procMouseEvent.Call(MOUSEEVENTF_WHEEL, 0, 0, uintptr(uint32(amount)), 0)
}

func keyDown(vk uint8) {
	procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
}

func keyUp(vk uint8) {
	procKeybdEvent.Call(uintptr(vk), 0, KEYEVENTF_KEYUP, 0)
}

func pressKey(name string) {
	if vk, ok := lookupKey(name); ok {
		keyDown(vk)
		time.Sleep(8 * time.Millisecond)
		keyUp(vk)
	}
}

func doHotkey(keys []string) {
	if len(keys) > maxHotkeyKeys {
		keys = keys[:maxHotkeyKeys]
	}

	pressed := make([]uint8, 0, len(keys))
	for _, k := range keys {
		if vk, ok := lookupKey(k); ok {
			keyDown(vk)
			pressed = append(pressed, vk)
		}
	}
	if len(pressed) == 0 {
		return
	}
	time.Sleep(12 * time.Millisecond)
	for i := len(pressed) - 1; i >= 0; i-- {
		keyUp(pressed[i])
	}
}

func lookupKey(name string) (uint8, bool) {
	vk, ok := vkMap[strings.ToLower(strings.TrimSpace(name))]
	return vk, ok
}

func getClipboardText() string {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	return getClipboardTextUnlocked()
}

func getClipboardTextUnlocked() string {
	if !openClipboardWithRetry() {
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

	runes := make([]uint16, 0, 256)
	for i := uintptr(0); i < maxClipboardRunes*2; i += 2 {
		ch := *(*uint16)(unsafe.Pointer(ptr + i))
		if ch == 0 {
			break
		}
		runes = append(runes, ch)
	}
	return syscall.UTF16ToString(runes)
}

func setClipboardText(text string) bool {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	return setClipboardTextUnlocked(text)
}

func setClipboardTextUnlocked(text string) bool {
	if strings.ContainsRune(text, '\x00') {
		text = strings.ReplaceAll(text, "\x00", "")
	}
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return false
	}

	size := len(utf16) * 2
	handle, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(size))
	if handle == 0 {
		return false
	}

	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		procGlobalFree.Call(handle)
		return false
	}

	for i, ch := range utf16 {
		*(*uint16)(unsafe.Pointer(ptr + uintptr(i*2))) = ch
	}
	procGlobalUnlock.Call(handle)

	if !openClipboardWithRetry() {
		procGlobalFree.Call(handle)
		return false
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	ret, _, _ := procSetClipboardData.Call(CF_UNICODETEXT, handle)
	if ret == 0 {
		procGlobalFree.Call(handle)
		return false
	}
	return true
}

func openClipboardWithRetry() bool {
	for i := 0; i < clipboardRetryCount; i++ {
		ret, _, _ := procOpenClipboard.Call(0)
		if ret != 0 {
			return true
		}
		time.Sleep(time.Duration(clipboardRetryWaitMS) * time.Millisecond)
	}
	return false
}

func typeText(text string, pressEnter bool) {
	text = trimRunes(text, maxTextRunes)
	if text == "" {
		return
	}

	clipboardMu.Lock()
	oldClip := getClipboardTextUnlocked()
	if !setClipboardTextUnlocked(text) {
		clipboardMu.Unlock()
		return
	}
	clipboardMu.Unlock()

	time.Sleep(80 * time.Millisecond)
	doHotkey([]string{"ctrl", "v"})
	if pressEnter {
		time.Sleep(60 * time.Millisecond)
		pressKey("enter")
	}

	time.Sleep(120 * time.Millisecond)
	clipboardMu.Lock()
	setClipboardTextUnlocked(oldClip)
	clipboardMu.Unlock()
}

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
	title := strings.TrimSpace(syscall.UTF16ToString(buf))
	if title == "" {
		return "Unknown"
	}
	return title
}

func getCurrentTabInfo() (string, string) {
	clipboardMu.Lock()
	oldClip := getClipboardTextUnlocked()

	doHotkey([]string{"ctrl", "l"})
	time.Sleep(220 * time.Millisecond)
	doHotkey([]string{"ctrl", "c"})
	time.Sleep(220 * time.Millisecond)

	rawURL := strings.TrimSpace(getClipboardTextUnlocked())
	setClipboardTextUnlocked(oldClip)
	clipboardMu.Unlock()

	title := getActiveWindowTitle()
	if !isHTTPURL(rawURL) {
		rawURL = ""
	}
	return rawURL, title
}

type ipCandidate struct {
	ip    net.IP
	iface net.Interface
	score int
}

func getLocalIP() string {
	preferred := getOutboundIPv4()
	candidates := collectIPv4Candidates(preferred)
	if len(candidates) == 0 {
		if preferred != nil && isUsableIPv4(preferred) {
			return preferred.String()
		}
		return "127.0.0.1"
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.score > best.score {
			best = candidate
		}
	}
	return best.ip.String()
}

func getOutboundIPv4() net.IP {
	conn, err := net.DialTimeout("udp4", "8.8.8.8:80", 600*time.Millisecond)
	if err != nil {
		return nil
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || !isUsableIPv4(addr.IP) {
		return nil
	}
	return addr.IP.To4()
}

func collectIPv4Candidates(preferred net.IP) []ipCandidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var candidates []ipCandidate
	seen := map[string]bool{}
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
			if !ok || !isUsableIPv4(ipnet.IP) {
				continue
			}
			ip := ipnet.IP.To4()
			key := ip.String()
			if seen[key] {
				continue
			}
			seen[key] = true

			score := 0
			if preferred != nil && ip.Equal(preferred) {
				score += 60
			}
			if ip.IsPrivate() {
				score += 40
			}
			if iface.Flags&net.FlagBroadcast != 0 {
				score += 5
			}
			if iface.Flags&net.FlagPointToPoint != 0 {
				score -= 25
			}
			score += interfaceNameScore(iface.Name)

			candidates = append(candidates, ipCandidate{ip: ip, iface: iface, score: score})
		}
	}
	return candidates
}

func isUsableIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return !ip4.IsUnspecified() &&
		!ip4.IsLoopback() &&
		!ip4.IsLinkLocalUnicast() &&
		!(ip4[0] == 169 && ip4[1] == 254)
}

func interfaceNameScore(name string) int {
	lower := strings.ToLower(name)
	virtualTerms := []string{
		"bluetooth", "docker", "hyper-v", "loopback", "npcap",
		"tap", "tailscale", "tun", "virtual", "vbox", "vmware",
		"vpn", "wintun", "wsl", "zerotier",
	}
	for _, term := range virtualTerms {
		if strings.Contains(lower, term) {
			return -60
		}
	}
	preferredTerms := []string{"ethernet", "lan", "wi-fi", "wifi", "wireless", "wlan", "이더넷", "무선", "로컬 영역"}
	for _, term := range preferredTerms {
		if strings.Contains(lower, term) {
			return 20
		}
	}
	return 0
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkWSOrigin,
}

func checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return normalizeHost(originURL.Host) == normalizeHost(r.Host)
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return strings.Trim(host, "[]")
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	client := &Client{conn: conn}
	conn.SetReadLimit(maxMessageBytes)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := client.ping(); err != nil {
					conn.Close()
					return
				}
			case <-stopPing:
				return
			}
		}
	}()

	log.Println("Client connected:", r.RemoteAddr)
	for {
		messageType, rawMsg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Client disconnected:", r.RemoteAddr)
			break
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		var msg WSMessage
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			log.Println("Invalid WebSocket message:", err)
			continue
		}
		handleEvent(client, msg)
	}
}

func handleEvent(client *Client, msg WSMessage) {
	if msg.Event == "" {
		return
	}
	data := msg.Data
	if data == nil {
		data = map[string]interface{}{}
	}

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

		dx, okX := toFloat(data["dx"])
		dy, okY := toFloat(data["dy"])
		if okX || okY {
			moveRelative(dx, dy)
		}

	case "scroll":
		dy, ok := toFloat(data["dy"])
		if ok && isFinite(dy) {
			mouseScroll(int(math.Round(dy)))
		}

	case "click":
		btn, _ := data["btn"].(string)
		mouseClick(btn)

	case "type":
		text, _ := data["text"].(string)
		pressEnter, _ := data["pressEnter"].(bool)
		typeText(text, pressEnter)

	case "key":
		key, _ := data["key"].(string)
		pressKey(key)

	case "hotkey":
		doHotkey(toStringSlice(data["keys"], maxHotkeyKeys))

	case "system":
		handleSystemEvent(client, data)

	case "get_current_tab":
		tabURL, title := getCurrentTabInfo()
		if err := client.SendEvent("current_tab", map[string]string{
			"url":   tabURL,
			"title": title,
		}); err != nil {
			log.Println("current_tab send error:", err)
		}

	case "open":
		urlVal, _ := data["url"].(string)
		if safeURL, ok := normalizeRemoteURL(urlVal); ok {
			if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", safeURL).Start(); err != nil {
				log.Println("open URL error:", err)
			}
		}
	}
}

func handleSystemEvent(client *Client, data map[string]interface{}) {
	action, _ := data["action"].(string)
	if action != "sleep" {
		return
	}

	delay, ok := toFloat(data["delay"])
	if !ok || !isFinite(delay) {
		delay = 0
	}
	delay = clampFloat(delay, 0, 24*60)

	if delay > 0 {
		duration := time.Duration(delay * float64(time.Minute))
		go func() {
			time.Sleep(duration)
			if err := exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0").Run(); err != nil {
				log.Println("sleep command error:", err)
			}
		}()
		if err := client.SendEvent("system_status", map[string]string{
			"message": fmt.Sprintf("%.0f분 후 절전 모드로 전환됩니다.", delay),
		}); err != nil {
			log.Println("system_status send error:", err)
		}
		return
	}

	if err := exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0").Run(); err != nil {
		log.Println("sleep command error:", err)
	}
}

func toFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, isFinite(val)
	case float32:
		f := float64(val)
		return f, isFinite(f)
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil && isFinite(f)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f, err == nil && isFinite(f)
	default:
		return 0, false
	}
}

func toStringSlice(v interface{}, limit int) []string {
	if limit <= 0 {
		return nil
	}

	switch val := v.(type) {
	case []interface{}:
		out := make([]string, 0, minInt(len(val), limit))
		for _, item := range val {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
			if len(out) >= limit {
				break
			}
		}
		return out
	case []string:
		out := make([]string, 0, minInt(len(val), limit))
		for _, s := range val {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
			if len(out) >= limit {
				break
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeRemoteURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 2048 || strings.ContainsRune(raw, '\x00') {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	return u.String(), true
}

func isHTTPURL(raw string) bool {
	_, ok := normalizeRemoteURL(raw)
	return ok
}

func trimRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func listenOnAvailablePort(startPort, attempts int) (net.Listener, int, error) {
	var lastErr error
	for port := startPort; port < startPort+attempts; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("no available port in %d-%d: %w", startPort, startPort+attempts-1, lastErr)
}

func renderTemplate(w http.ResponseWriter, tmpl *template.Template, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Println("template render error:", err)
	}
}

func serveEmbeddedAsset(path, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		data, err := templatesFS.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(data)
	}
}

func openLocalQRPage(port int) {
	qrURL := fmt.Sprintf("http://127.0.0.1:%d/qr", port)
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", qrURL).Start(); err != nil {
		log.Println("open QR page error:", err)
	}
}

func ensureFirewallRules(port int) {
	exePath, err := os.Executable()
	if err != nil {
		log.Println("firewall setup skipped:", err)
		return
	}
	if !strings.HasSuffix(strings.ToLower(exePath), ".exe") {
		return
	}

	for _, name := range []string{
		"Air Mouse",
		"AirMouse",
		"AirMouse.exe",
		"Air Controller",
		"AirController",
		"AirController.exe",
		"Air Mouse Port",
		"AirMouse Port",
		"Air Controller Port",
		"AirController Port",
	} {
		_ = runNetsh("advfirewall", "firewall", "delete", "rule", "name="+name)
	}

	if err := runNetsh(
		"advfirewall", "firewall", "add", "rule",
		"name=Air Mouse",
		"dir=in",
		"action=allow",
		"program="+exePath,
		"enable=yes",
		"profile=any",
		"protocol=TCP",
	); err != nil {
		log.Println("firewall setup failed; run AirMouse.exe as administrator once or allow it in Windows Firewall:", err)
		return
	}

	if err := runNetsh(
		"advfirewall", "firewall", "add", "rule",
		"name=Air Mouse Port",
		"dir=in",
		"action=allow",
		"enable=yes",
		"profile=any",
		"protocol=TCP",
		"localport="+strconv.Itoa(port),
	); err != nil {
		log.Println("firewall port setup failed; allow TCP port manually if phone cannot connect:", err)
		return
	}

	log.Printf("Firewall rules ready: Air Mouse, Air Mouse Port %d", port)
}

func runNetsh(args ...string) error {
	out, err := exec.Command("netsh", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

func main() {
	listener, port, err := listenOnAvailablePort(defaultPort, portAttempts)
	if err != nil {
		log.Fatal(err)
	}

	localIP := getLocalIP()
	serverURL := fmt.Sprintf("http://%s:%d", localIP, port)

	indexTmpl := template.Must(template.ParseFS(templatesFS, "templates/index.html"))
	qrTmpl := template.Must(template.ParseFS(templatesFS, "templates/qr.html"))

	mux := http.NewServeMux()
	mux.HandleFunc("/style.css", serveEmbeddedAsset("templates/style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/app.js", serveEmbeddedAsset("templates/app.js", "application/javascript; charset=utf-8"))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		renderTemplate(w, indexTmpl, map[string]string{"ServerURL": serverURL})
	})

	mux.HandleFunc("/qr", func(w http.ResponseWriter, r *http.Request) {
		png, err := qrcode.Encode(serverURL, qrcode.Medium, 256)
		if err != nil {
			http.Error(w, "failed to generate QR code", http.StatusInternalServerError)
			log.Println("QR encode error:", err)
			return
		}
		renderTemplate(w, qrTmpl, map[string]string{
			"AppURL": serverURL,
			"QRData": base64.StdEncoding.EncodeToString(png),
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ws", handleWS)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Printf("Air Mouse: %s\n", serverURL)
	if port != defaultPort {
		fmt.Printf("Port %d was busy; using %d instead.\n", defaultPort, port)
	}
	fmt.Println("Open this URL on a phone/tablet on the same network.")
	fmt.Printf("Health check: %s/health\n", serverURL)
	fmt.Println("If the phone cannot open /health, check Wi-Fi/LAN isolation or firewall.")
	fmt.Println("========================================")
	fmt.Println()

	ensureFirewallRules(port)
	openLocalQRPage(port)

	log.Printf("Server running at %s\n", serverURL)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
