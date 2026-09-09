package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	featurePort = 5050

	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
	esContinuous      = 0x80000000

	sdcTopologyInternal = 0x00000001
	sdcTopologyClone    = 0x00000002
	sdcTopologyExtend   = 0x00000004
	sdcTopologyExternal = 0x00000008
	sdcApply            = 0x00000080
)

var (
	featureKernel32 = syscall.NewLazyDLL("kernel32.dll")
	featureUser32   = syscall.NewLazyDLL("user32.dll")
	featurePowrProf = syscall.NewLazyDLL("powrprof.dll")

	procCreateMutexWFeature     = featureKernel32.NewProc("CreateMutexW")
	procSetThreadExecutionState = featureKernel32.NewProc("SetThreadExecutionState")
	procSetDisplayConfigFeature = featureUser32.NewProc("SetDisplayConfig")
	procSetSuspendStateFeature  = featurePowrProf.NewProc("SetSuspendState")
	singleInstanceHandle        uintptr

	featureConfigMu          sync.RWMutex
	featureConfig            = defaultFeatureConfig()
	featureConfigInitialized bool

	powerMu       sync.RWMutex
	sleepTimer    *time.Timer
	sleepDeadline time.Time
	keepAwake     bool

	keepAwakeRequests = make(chan keepAwakeRequest)
)

type Favorite struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

type FeatureConfig struct {
	Version           int        `json:"version"`
	Favorites         []Favorite `json:"favorites"`
	RecentHistory     []string   `json:"recentHistory"`
	MouseSensitivity  float64    `json:"mouseSensitivity"`
	ScrollSensitivity float64    `json:"scrollSensitivity"`
	LastTab           string     `json:"lastTab"`
}

type featureStateResponse struct {
	Initialized bool          `json:"initialized"`
	Config      FeatureConfig `json:"config"`
	Power       powerState    `json:"power"`
}

type powerState struct {
	SleepDeadline int64 `json:"sleepDeadline"`
	KeepAwake     bool  `json:"keepAwake"`
}

type keepAwakeRequest struct {
	enable bool
	done   chan error
}

func init() {
	if runningUnderGoTest() {
		return
	}
	if !acquireSingleInstance() {
		reopenRunningController()
		os.Exit(0)
	}

	loadFeatureConfig()
	go keepAwakeWorker()
	startFeatureServer()
}

func runningUnderGoTest() bool {
	name := strings.ToLower(filepath.Base(os.Args[0]))
	return strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".test.exe")
}

func acquireSingleInstance() bool {
	name, err := syscall.UTF16PtrFromString(`Local\AirController.SingleInstance.v1`)
	if err != nil {
		log.Println("single-instance mutex skipped:", err)
		return true
	}

	handle, _, callErr := procCreateMutexWFeature.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		log.Println("single-instance mutex failed:", callErr)
		return true
	}
	singleInstanceHandle = handle

	if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(183) {
		return false
	}
	return true
}

func reopenRunningController() {
	client := &http.Client{Timeout: 140 * time.Millisecond}
	for port := defaultPort; port < defaultPort+portAttempts; port++ {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == "ok" {
			openLocalQRPage(port)
			return
		}
	}
}

func defaultFeatureConfig() FeatureConfig {
	return FeatureConfig{
		Version:           1,
		Favorites:         []Favorite{},
		RecentHistory:     []string{},
		MouseSensitivity:  2.5,
		ScrollSensitivity: 3,
		LastTab:           "touch",
	}
}

func featureConfigPath() string {
	base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if base == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			base = dir
		}
	}
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "AirController", "config.json")
}

func loadFeatureConfig() {
	path := featureConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Println("config read failed:", err)
		}
		return
	}

	var cfg FeatureConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Println("config parse failed:", err)
		return
	}

	featureConfigMu.Lock()
	featureConfig = sanitizeFeatureConfig(cfg)
	featureConfigInitialized = true
	featureConfigMu.Unlock()
}

func saveFeatureConfig(cfg FeatureConfig) error {
	cfg = sanitizeFeatureConfig(cfg)
	path := featureConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// Windows may refuse to replace an existing file with Rename.
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = os.Remove(tmp)
			return err
		}
		if retryErr := os.Rename(tmp, path); retryErr != nil {
			_ = os.Remove(tmp)
			return retryErr
		}
	}

	featureConfigMu.Lock()
	featureConfig = cfg
	featureConfigInitialized = true
	featureConfigMu.Unlock()
	return nil
}

func sanitizeFeatureConfig(cfg FeatureConfig) FeatureConfig {
	out := defaultFeatureConfig()
	out.Version = 1

	seenFavorites := map[string]bool{}
	for _, item := range cfg.Favorites {
		if len(out.Favorites) >= 24 {
			break
		}
		u, ok := normalizeRemoteURL(item.URL)
		if !ok || seenFavorites[u] {
			continue
		}
		seenFavorites[u] = true
		label := strings.TrimSpace(item.Label)
		label = trimRunes(label, 80)
		out.Favorites = append(out.Favorites, Favorite{URL: u, Label: label})
	}

	seenRecent := map[string]bool{}
	for _, raw := range cfg.RecentHistory {
		if len(out.RecentHistory) >= 12 {
			break
		}
		u, ok := normalizeRemoteURL(raw)
		if !ok || seenRecent[u] {
			continue
		}
		seenRecent[u] = true
		out.RecentHistory = append(out.RecentHistory, u)
	}

	if cfg.MouseSensitivity >= 1 && cfg.MouseSensitivity <= 5 {
		out.MouseSensitivity = cfg.MouseSensitivity
	}
	if cfg.ScrollSensitivity >= 1 && cfg.ScrollSensitivity <= 5 {
		out.ScrollSensitivity = cfg.ScrollSensitivity
	}
	switch cfg.LastTab {
	case "touch", "input", "apps":
		out.LastTab = cfg.LastTab
	}
	return out
}

func currentFeatureState() featureStateResponse {
	featureConfigMu.RLock()
	cfg := featureConfig
	initialized := featureConfigInitialized
	featureConfigMu.RUnlock()
	return featureStateResponse{
		Initialized: initialized,
		Config:      cfg,
		Power:       currentPowerState(),
	}
}

func currentPowerState() powerState {
	powerMu.RLock()
	defer powerMu.RUnlock()
	deadline := int64(0)
	if !sleepDeadline.IsZero() {
		deadline = sleepDeadline.UnixMilli()
	}
	return powerState{SleepDeadline: deadline, KeepAwake: keepAwake}
}

func startFeatureServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/feature/state", featureCORS(handleFeatureState))
	mux.HandleFunc("/feature/sleep", featureCORS(handleFeatureSleep))
	mux.HandleFunc("/feature/keep-awake", featureCORS(handleFeatureKeepAwake))
	mux.HandleFunc("/feature/display", featureCORS(handleFeatureDisplay))
	mux.HandleFunc("/feature/health", featureCORS(func(w http.ResponseWriter, r *http.Request) {
		writeFeatureJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", featurePort))
	if err != nil {
		log.Printf("optional feature server disabled on port %d: %v", featurePort, err)
		return
	}
	go func() {
		log.Printf("Feature server running on :%d", featurePort)
		server := &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Println("feature server error:", err)
		}
	}()
}

func featureCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			originURL, err := url.Parse(origin)
			if err != nil || normalizeHost(originURL.Host) != normalizeHost(r.Host) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func handleFeatureState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeFeatureJSON(w, http.StatusOK, currentFeatureState())
	case http.MethodPost:
		var cfg FeatureConfig
		if err := decodeFeatureJSON(r, &cfg); err != nil {
			writeFeatureError(w, http.StatusBadRequest, err)
			return
		}
		if err := saveFeatureConfig(cfg); err != nil {
			writeFeatureError(w, http.StatusInternalServerError, err)
			return
		}
		writeFeatureJSON(w, http.StatusOK, currentFeatureState())
	default:
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		writeFeatureError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func handleFeatureSleep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeFeatureError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	var req struct {
		Action  string  `json:"action"`
		Minutes float64 `json:"minutes"`
	}
	if err := decodeFeatureJSON(r, &req); err != nil {
		writeFeatureError(w, http.StatusBadRequest, err)
		return
	}

	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "set":
		minutes := clampFloat(req.Minutes, 0, 24*60)
		if req.Minutes < 0 || req.Minutes > 24*60 {
			writeFeatureError(w, http.StatusBadRequest, fmt.Errorf("minutes must be between 0 and 1440"))
			return
		}
		setSleepTimer(minutes)
	case "cancel":
		cancelSleepTimer()
	default:
		writeFeatureError(w, http.StatusBadRequest, fmt.Errorf("unknown sleep action"))
		return
	}
	writeFeatureJSON(w, http.StatusOK, currentPowerState())
}

func setSleepTimer(minutes float64) {
	powerMu.Lock()
	if sleepTimer != nil {
		sleepTimer.Stop()
		sleepTimer = nil
	}
	if minutes <= 0 {
		sleepDeadline = time.Time{}
		powerMu.Unlock()
		go suspendComputer()
		return
	}

	duration := time.Duration(minutes * float64(time.Minute))
	sleepDeadline = time.Now().Add(duration)
	sleepTimer = time.AfterFunc(duration, func() {
		powerMu.Lock()
		sleepTimer = nil
		sleepDeadline = time.Time{}
		powerMu.Unlock()
		suspendComputer()
	})
	powerMu.Unlock()
}

func cancelSleepTimer() {
	powerMu.Lock()
	if sleepTimer != nil {
		sleepTimer.Stop()
		sleepTimer = nil
	}
	sleepDeadline = time.Time{}
	powerMu.Unlock()
}

func suspendComputer() {
	ret, _, callErr := procSetSuspendStateFeature.Call(0, 0, 0)
	if ret != 0 {
		return
	}
	log.Println("SetSuspendState failed; falling back to rundll32:", callErr)
	if err := exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,0,0").Run(); err != nil {
		log.Println("sleep command error:", err)
	}
}

func keepAwakeWorker() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for req := range keepAwakeRequests {
		flags := uintptr(esContinuous)
		if req.enable {
			flags |= uintptr(esSystemRequired | esDisplayRequired)
		}
		ret, _, callErr := procSetThreadExecutionState.Call(flags)
		if ret == 0 {
			req.done <- fmt.Errorf("SetThreadExecutionState failed: %v", callErr)
			continue
		}
		req.done <- nil
	}
}

func setKeepAwake(enabled bool) error {
	done := make(chan error, 1)
	keepAwakeRequests <- keepAwakeRequest{enable: enabled, done: done}
	if err := <-done; err != nil {
		return err
	}
	powerMu.Lock()
	keepAwake = enabled
	powerMu.Unlock()
	return nil
}

func handleFeatureKeepAwake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeFeatureError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeFeatureJSON(r, &req); err != nil {
		writeFeatureError(w, http.StatusBadRequest, err)
		return
	}
	if err := setKeepAwake(req.Enabled); err != nil {
		writeFeatureError(w, http.StatusInternalServerError, err)
		return
	}
	writeFeatureJSON(w, http.StatusOK, currentPowerState())
}

func handleFeatureDisplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeFeatureError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decodeFeatureJSON(r, &req); err != nil {
		writeFeatureError(w, http.StatusBadRequest, err)
		return
	}
	if err := applyDisplayMode(req.Mode); err != nil {
		writeFeatureError(w, http.StatusBadRequest, err)
		return
	}
	writeFeatureJSON(w, http.StatusOK, map[string]string{"mode": req.Mode})
}

func applyDisplayMode(mode string) error {
	var topology uint32
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "internal":
		topology = sdcTopologyInternal
	case "clone":
		topology = sdcTopologyClone
	case "extend":
		topology = sdcTopologyExtend
	case "external":
		topology = sdcTopologyExternal
	default:
		return fmt.Errorf("unknown display mode")
	}

	ret, _, _ := procSetDisplayConfigFeature.Call(0, 0, 0, 0, uintptr(sdcApply|topology))
	if int32(ret) != 0 {
		return fmt.Errorf("SetDisplayConfig failed with code %d", int32(ret))
	}
	return nil
}

func decodeFeatureJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}

func writeFeatureError(w http.ResponseWriter, status int, err error) {
	writeFeatureJSON(w, status, map[string]string{"error": err.Error()})
}

func writeFeatureJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
