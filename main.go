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
	"sort"
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
	shell32  = syscall.NewLazyDLL("shell32.dll")

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
	procShellExecuteW        = shell32.NewProc("ShellExecuteW")
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
	firewallRuleName     = "AirController Remote"
	firewallPortRange    = "5000-5019"
	swHide               = 0
	swShowNormal         = 1
	bluetoothCacheTTL    = 10 * time.Second
)

var (
	writeWait    = 5 * time.Second
	pongWait     = 60 * time.Second
	pingInterval = 30 * time.Second

	clipboardMu  sync.Mutex
	automationMu sync.Mutex

	bluetoothCacheMu sync.Mutex
	bluetoothCacheAt time.Time
	bluetoothCache   bluetoothCapabilities
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
	buffer := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), maxClipboardRunes)
	for _, ch := range buffer {
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

	buffer := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(buffer, utf16)
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
	automationMu.Lock()
	defer automationMu.Unlock()

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
	automationMu.Lock()
	defer automationMu.Unlock()

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
	ip        net.IP
	iface     net.Interface
	score     int
	kind      string
	advertise bool
	bluetooth bool
}

type ConnectionOption struct {
	Label       string
	Interface   string
	AppURL      string
	QRData      string
	Recommended bool
	Bluetooth   bool
}

type QRPageData struct {
	Options       []ConnectionOption
	FirewallReady bool
	HasBluetooth  bool
}

type bluetoothCapabilities struct {
	AdapterPresent    bool
	PANServicePresent bool
}

type BluetoothStatus struct {
	AdapterPresent      bool   `json:"adapterPresent"`
	PANServicePresent   bool   `json:"panServicePresent"`
	InterfacePresent    bool   `json:"interfacePresent"`
	Connected           bool   `json:"connected"`
	Interface           string `json:"interface,omitempty"`
	AppURL              string `json:"appURL,omitempty"`
	AutoOpenRecommended bool   `json:"autoOpenRecommended"`
}

func hasBluetoothOption(options []ConnectionOption) bool {
	for _, option := range options {
		if option.Bluetooth {
			return true
		}
	}
	return false
}

func getBluetoothStatus(port int) BluetoothStatus {
	capabilities := getBluetoothCapabilities()
	status := BluetoothStatus{
		AdapterPresent:    capabilities.AdapterPresent,
		PANServicePresent: capabilities.PANServicePresent,
	}

	hasWiFiInterface := false
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			_, _, _, bluetooth := classifyInterface(iface.Name)
			if bluetooth {
				status.InterfacePresent = true
				if status.Interface == "" {
					status.Interface = iface.Name
				}
			}
			kind, _, _, _ := classifyInterface(iface.Name)
			if kind == "Wi-Fi" {
				hasWiFiInterface = true
			}
		}
	}

	for _, candidate := range collectIPv4Candidates() {
		if !candidate.bluetooth || !candidate.advertise {
			continue
		}
		status.Connected = true
		status.Interface = candidate.iface.Name
		status.AppURL = fmt.Sprintf("http://%s:%d", candidate.ip.String(), port)
		break
	}

	status.AutoOpenRecommended = status.PANServicePresent && !status.Connected && !hasWiFiInterface
	return status
}

func getBluetoothCapabilities() bluetoothCapabilities {
	bluetoothCacheMu.Lock()
	defer bluetoothCacheMu.Unlock()

	if !bluetoothCacheAt.IsZero() && time.Since(bluetoothCacheAt) < bluetoothCacheTTL {
		return bluetoothCache
	}

	out, err := exec.Command("pnputil", "/enum-devices", "/class", "Bluetooth", "/connected").CombinedOutput()
	if err == nil {
		bluetoothCache = parseBluetoothCapabilities(string(out))
	}
	bluetoothCacheAt = time.Now()
	return bluetoothCache
}

func parseBluetoothCapabilities(output string) bluetoothCapabilities {
	lower := strings.ToLower(output)
	panServicePresent := strings.Contains(lower, "{00001116-0000-1000-8000-00805f9b34fb}") ||
		strings.Contains(lower, "personal area network nap service")
	adapterPresent := panServicePresent ||
		strings.Contains(lower, "bluetooth adapter") ||
		strings.Contains(lower, "bth\\ms_bthbrb")
	return bluetoothCapabilities{
		AdapterPresent:    adapterPresent,
		PANServicePresent: panServicePresent,
	}
}

func getConnectionOptions(port int) []ConnectionOption {
	candidates := collectIPv4Candidates()
	options := make([]ConnectionOption, 0, len(candidates))

	for _, candidate := range candidates {
		if !candidate.advertise {
			continue
		}
		appURL := fmt.Sprintf("http://%s:%d", candidate.ip.String(), port)
		png, err := qrcode.Encode(appURL, qrcode.Medium, 256)
		if err != nil {
			log.Printf("QR encode error for %s: %v", appURL, err)
			continue
		}
		options = append(options, ConnectionOption{
			Label:       candidate.kind,
			Interface:   candidate.iface.Name,
			AppURL:      appURL,
			QRData:      base64.StdEncoding.EncodeToString(png),
			Recommended: len(options) == 0,
			Bluetooth:   candidate.bluetooth,
		})
	}

	if len(options) == 0 {
		appURL := fmt.Sprintf("http://127.0.0.1:%d", port)
		png, _ := qrcode.Encode(appURL, qrcode.Medium, 256)
		options = append(options, ConnectionOption{
			Label:       "이 PC에서만",
			Interface:   "Loopback",
			AppURL:      appURL,
			QRData:      base64.StdEncoding.EncodeToString(png),
			Recommended: true,
		})
	}

	return options
}

func collectIPv4Candidates() []ipCandidate {
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

			kind, baseScore, advertise, bluetooth := classifyInterface(iface.Name)
			score := baseScore
			if ip.IsPrivate() {
				score += 40
			}
			if iface.Flags&net.FlagBroadcast != 0 {
				score += 5
			}
			if iface.Flags&net.FlagPointToPoint != 0 {
				score -= 25
			}

			candidates = append(candidates, ipCandidate{
				ip: ip, iface: iface, score: score, kind: kind,
				advertise: advertise, bluetooth: bluetooth,
			})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].ip.String() < candidates[j].ip.String()
		}
		return candidates[i].score > candidates[j].score
	})
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

func classifyInterface(name string) (kind string, score int, advertise bool, bluetooth bool) {
	lower := strings.ToLower(name)
	if containsAny(lower, []string{"bluetooth", "블루투스"}) {
		return "Bluetooth PAN", 70, true, true
	}
	if containsAny(lower, []string{"docker", "hyper-v", "loopback", "npcap", "virtual", "vbox", "vethernet", "vmware", "wsl"}) {
		return "내부 가상 네트워크", -80, false, false
	}
	if containsAny(lower, []string{"ethernet", "이더넷", "로컬 영역"}) || strings.TrimSpace(lower) == "lan" {
		return "유선 LAN", 100, true, false
	}
	if containsAny(lower, []string{"wi-fi", "wifi", "wireless", "wlan", "무선"}) {
		return "Wi-Fi", 90, true, false
	}
	if containsAny(lower, []string{"tailscale", "zerotier", "vpn", "wintun", "tun", "tap"}) {
		return "VPN / 오버레이 네트워크", 35, true, false
	}
	return "기타 네트워크", 25, true, false
}

func containsAny(value string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
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
	if tcpConn, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

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
		go typeText(text, pressEnter)

	case "key":
		key, _ := data["key"].(string)
		pressKey(key)

	case "hotkey":
		doHotkey(toStringSlice(data["keys"], maxHotkeyKeys))

	case "system":
		handleSystemEvent(client, data)

	case "get_current_tab":
		go func() {
			tabURL, title := getCurrentTabInfo()
			if err := client.SendEvent("current_tab", map[string]string{
				"url":   tabURL,
				"title": title,
			}); err != nil {
				log.Println("current_tab send error:", err)
			}
		}()

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

func setupFirewallRule() error {
	for _, name := range []string{
		"Air Mouse", "AirMouse", "AirMouse.exe", "Air Mouse Port", "AirMouse Port",
		"Air Controller", "AirController", "AirController.exe", "Air Controller Port", "AirController Port",
	} {
		_ = runNetsh("advfirewall", "firewall", "delete", "rule", "name="+name)
	}
	_ = runNetsh("advfirewall", "firewall", "delete", "rule", "name="+firewallRuleName)
	return runNetsh(
		"advfirewall", "firewall", "add", "rule",
		"name="+firewallRuleName,
		"dir=in",
		"action=allow",
		"enable=yes",
		"profile=any",
		"protocol=TCP",
		"localport="+firewallPortRange,
		"remoteip=LocalSubnet",
		"edge=no",
	)
}

func firewallRuleConfigured() bool {
	return runNetsh("advfirewall", "firewall", "show", "rule", "name="+firewallRuleName) == nil
}

func launchElevatedFirewallSetup() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exePath)
	params, _ := syscall.UTF16PtrFromString("--setup-firewall")
	result, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		swHide,
	)
	if result <= 32 {
		return fmt.Errorf("elevation request failed (%d): %v", result, callErr)
	}
	return nil
}

func openBluetoothSettings() error {
	verb, _ := syscall.UTF16PtrFromString("open")
	target, _ := syscall.UTF16PtrFromString("ms-settings:connecteddevices")
	result, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(target)),
		0,
		0,
		swShowNormal,
	)
	if result <= 32 {
		return fmt.Errorf("Bluetooth settings could not be opened (%d): %v", result, callErr)
	}
	return nil
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func handleFirewallSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
		http.Error(w, "firewall setup is only available on this PC", http.StatusForbidden)
		return
	}
	if !checkWSOrigin(r) {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}
	if firewallRuleConfigured() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := launchElevatedFirewallSetup(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleBluetoothOpenSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
		http.Error(w, "Bluetooth setup is only available on this PC", http.StatusForbidden)
		return
	}
	if !checkWSOrigin(r) {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}
	if err := openBluetoothSettings(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func hasArg(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}

func main() {
	if hasArg("--setup-firewall") {
		if err := setupFirewallRule(); err != nil {
			log.Fatal("firewall setup failed: ", err)
		}
		return
	}

	listener, port, err := listenOnAvailablePort(defaultPort, portAttempts)
	if err != nil {
		log.Fatal(err)
	}

	connectionOptions := getConnectionOptions(port)
	serverURL := connectionOptions[0].AppURL

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
		renderTemplate(w, indexTmpl, map[string]string{"ServerURL": "http://" + r.Host})
	})

	mux.HandleFunc("/qr", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) {
			http.Error(w, "connection setup is only available on this PC", http.StatusForbidden)
			return
		}
		options := getConnectionOptions(port)
		renderTemplate(w, qrTmpl, QRPageData{
			Options:       options,
			FirewallReady: firewallRuleConfigured(),
			HasBluetooth:  hasBluetoothOption(options),
		})
	})
	mux.HandleFunc("/firewall/setup", handleFirewallSetup)
	mux.HandleFunc("/bluetooth/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLoopbackRequest(r) {
			http.Error(w, "Bluetooth status is only available on this PC", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(getBluetoothStatus(port)); err != nil {
			log.Println("Bluetooth status response error:", err)
		}
	})
	mux.HandleFunc("/bluetooth/open-settings", handleBluetoothOpenSettings)
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
	fmt.Println("Open the local QR page to choose LAN, Wi-Fi, hotspot, VPN, or Bluetooth PAN.")
	fmt.Printf("Health check: %s/health\n", serverURL)
	fmt.Println("If the phone cannot open /health, check the selected adapter and the one-time firewall rule.")
	fmt.Println("========================================")
	fmt.Println()

	if !hasArg("--no-browser") {
		openLocalQRPage(port)
	}

	log.Printf("Server running at %s\n", serverURL)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
