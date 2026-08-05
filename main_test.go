package main

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedTemplatesParse(t *testing.T) {
	if _, err := template.ParseFS(templatesFS, "templates/index.html"); err != nil {
		t.Fatalf("index template parse failed: %v", err)
	}
	if _, err := template.ParseFS(templatesFS, "templates/qr.html"); err != nil {
		t.Fatalf("qr template parse failed: %v", err)
	}
	for _, name := range []string{"templates/style.css", "templates/app.js"} {
		if _, err := templatesFS.ReadFile(name); err != nil {
			t.Fatalf("%s read failed: %v", name, err)
		}
	}
}

func TestServeEmbeddedAsset(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/style.css", nil)

	serveEmbeddedAsset("templates/style.css", "text/css; charset=utf-8")(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), ".app-container") {
		t.Fatal("style.css response did not contain expected CSS")
	}
}

func TestQRTemplateShowsBluetoothSetupWhenDisconnected(t *testing.T) {
	tmpl := template.Must(template.ParseFS(templatesFS, "templates/qr.html"))
	var output bytes.Buffer
	err := tmpl.Execute(&output, QRPageData{
		Options: []ConnectionOption{{
			Label: "유선 LAN", Interface: "Ethernet", AppURL: "http://192.168.0.2:5000", QRData: "abc",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Bluetooth 연결 준비 중") {
		t.Fatal("Bluetooth PAN setup guidance was not rendered")
	}
}

func TestParseBluetoothCapabilities(t *testing.T) {
	output := `
Instance ID: BTHENUM\\{00001116-0000-1000-8000-00805F9B34FB}_LOCALMFG&0002\\8&ABC&0&F0CD310B8084_C00000000
Device Description: Personal Area Network NAP Service
Instance ID: USB\\VID_33FA&PID_0001\\5&ABC&0&5
Device Description: BARROT Bluetooth Adapter
`
	got := parseBluetoothCapabilities(output)
	if !got.AdapterPresent || !got.PANServicePresent {
		t.Fatalf("parseBluetoothCapabilities() = %+v, want adapter and PAN service", got)
	}
}

func TestParseBluetoothCapabilitiesWithoutPAN(t *testing.T) {
	got := parseBluetoothCapabilities("Device Description: Generic Bluetooth Adapter")
	if !got.AdapterPresent || got.PANServicePresent {
		t.Fatalf("parseBluetoothCapabilities() = %+v, want adapter only", got)
	}
}

func TestNormalizeRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		ok   bool
	}{
		{name: "https", raw: "https://www.youtube.com", ok: true},
		{name: "http", raw: "http://172.18.199.69:5000/health", ok: true},
		{name: "missing host", raw: "https://", ok: false},
		{name: "blocked scheme", raw: "file:///C:/Windows", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := normalizeRemoteURL(tt.raw)
			if ok != tt.ok {
				t.Fatalf("normalizeRemoteURL(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			}
		})
	}
}

func TestClassifyInterface(t *testing.T) {
	tests := []struct {
		name      string
		iface     string
		kind      string
		advertise bool
		bluetooth bool
	}{
		{name: "ethernet", iface: "Intel(R) Ethernet Connection", kind: "유선 LAN", advertise: true},
		{name: "wifi", iface: "Intel(R) Wi-Fi 6 AX201", kind: "Wi-Fi", advertise: true},
		{name: "wlan is not lan", iface: "WLAN", kind: "Wi-Fi", advertise: true},
		{name: "bluetooth pan", iface: "Bluetooth Network Connection", kind: "Bluetooth PAN", advertise: true, bluetooth: true},
		{name: "overlay", iface: "Tailscale Tunnel", kind: "VPN / 오버레이 네트워크", advertise: true},
		{name: "internal virtual", iface: "vEthernet (WSL)", kind: "내부 가상 네트워크", advertise: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, _, advertise, bluetooth := classifyInterface(tt.iface)
			if kind != tt.kind || advertise != tt.advertise || bluetooth != tt.bluetooth {
				t.Fatalf("classifyInterface(%q) = (%q, %v, %v), want (%q, %v, %v)",
					tt.iface, kind, advertise, bluetooth, tt.kind, tt.advertise, tt.bluetooth)
			}
		})
	}
}

func TestFirewallSetupEndpointGuards(t *testing.T) {
	t.Run("requires POST", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/firewall/setup", nil)
		req.RemoteAddr = "127.0.0.1:12345"

		handleFirewallSetup(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("rejects remote clients", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/firewall/setup", nil)
		req.RemoteAddr = "192.0.2.10:12345"

		handleFirewallSetup(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("rejects cross origin local requests", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5000/firewall/setup", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Origin", "https://example.com")

		handleFirewallSetup(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}

func TestBluetoothOpenSettingsEndpointGuards(t *testing.T) {
	t.Run("requires POST", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/bluetooth/open-settings", nil)
		req.RemoteAddr = "127.0.0.1:12345"

		handleBluetoothOpenSettings(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("rejects remote clients", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/bluetooth/open-settings", nil)
		req.RemoteAddr = "192.0.2.10:12345"

		handleBluetoothOpenSettings(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("rejects cross origin local requests", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5000/bluetooth/open-settings", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Origin", "https://example.com")

		handleBluetoothOpenSettings(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}
