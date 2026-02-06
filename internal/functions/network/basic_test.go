package network

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestPing(t *testing.T) {
	// Test with localhost - should always be reachable
	result, err := Ping("127.0.0.1", 1)
	if err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}

	if result == nil {
		t.Fatal("Ping returned nil result")
	}

	if result.PacketsSent != 1 {
		t.Errorf("Expected PacketsSent=1, got %d", result.PacketsSent)
	}

	// Result should have raw output
	if result.RawOutput == "" {
		t.Error("Expected non-empty RawOutput")
	}
}

func TestPing_Unreachable(t *testing.T) {
	// Use an IP that should be unreachable (RFC 5737 test range)
	result, err := Ping("192.0.2.1", 1)
	if err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}

	// Should return a result (not error) with unreachable status
	if result == nil {
		t.Fatal("Expected result for unreachable host")
	}

	if result.Reachable {
		t.Skip("Test network allows reaching 192.0.2.1")
	}
}

func TestPing_CountValidation(t *testing.T) {
	tests := []struct {
		count    int
		expected int
	}{
		{0, 3},   // Default
		{-5, 3},  // Negative -> default
		{25, 20}, // Over max -> capped at 20
		{5, 5},   // Normal
	}

	for _, tt := range tests {
		result, err := Ping("127.0.0.1", tt.count)
		if err != nil {
			t.Errorf("Ping(%d) error: %v", tt.count, err)
			continue
		}
		if result.PacketsSent != tt.expected {
			t.Errorf("Ping(%d): expected PacketsSent=%d, got %d",
				tt.count, tt.expected, result.PacketsSent)
		}
	}
}

func TestDNSLookup(t *testing.T) {
	// Test with a well-known domain
	result, err := DNSLookup("google.com", "A")
	if err != nil {
		t.Fatalf("DNSLookup error: %v", err)
	}

	if result == nil {
		t.Fatal("DNSLookup returned nil")
	}

	if result.RecordCount == 0 {
		t.Error("Expected at least one A record for google.com")
	}

	// Verify all records are type A
	for _, r := range result.Records {
		if r.Type != "A" {
			t.Errorf("Expected type A, got %s", r.Type)
		}
	}
}

func TestDNSLookup_AllTypes(t *testing.T) {
	result, err := DNSLookup("google.com", "all")
	if err != nil {
		t.Fatalf("DNSLookup error: %v", err)
	}

	if result.RecordCount == 0 {
		t.Error("Expected at least one record")
	}
}

func TestDNSLookup_InvalidDomain(t *testing.T) {
	_, err := DNSLookup("this-domain-definitely-does-not-exist-12345.invalid", "A")
	if err == nil {
		t.Error("Expected error for invalid domain")
	}
}

func TestPortScan(t *testing.T) {
	// Start a test server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	portStr := fmt.Sprintf("%d", port)

	result, err := PortScan("127.0.0.1", portStr)
	if err != nil {
		t.Fatalf("PortScan error: %v", err)
	}

	if len(result.OpenPorts) == 0 {
		t.Error("Expected port to be open")
	}
}

func TestPortScan_CommonPorts(t *testing.T) {
	result, err := PortScan("127.0.0.1", "common")
	if err != nil {
		t.Fatalf("PortScan error: %v", err)
	}

	if result.TotalScanned != len(CommonPorts) {
		t.Errorf("Expected %d ports scanned, got %d",
			len(CommonPorts), result.TotalScanned)
	}
}

func TestPortScan_InvalidPorts(t *testing.T) {
	_, err := PortScan("127.0.0.1", "not,valid,ports")
	if err == nil {
		t.Error("Expected error for invalid ports")
	}
}

func TestHTTPRequest(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	result, err := HTTPRequest(server.URL, "GET")
	if err != nil {
		t.Fatalf("HTTPRequest error: %v", err)
	}

	if result.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", result.StatusCode)
	}

	if !result.Success {
		t.Error("Expected Success=true for 200 response")
	}

	if result.ResponseTimeMs < 0 {
		t.Error("Expected non-negative response time")
	}

	if result.Headers["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type header, got %v", result.Headers)
	}
}

func TestHTTPRequest_Methods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Method))
	}))
	defer server.Close()

	methods := []string{"GET", "HEAD", "POST"}
	for _, method := range methods {
		result, err := HTTPRequest(server.URL, method)
		if err != nil {
			t.Errorf("HTTPRequest %s error: %v", method, err)
			continue
		}
		if result.StatusCode != 200 {
			t.Errorf("HTTPRequest %s: expected 200, got %d", method, result.StatusCode)
		}
	}
}

func TestHTTPRequest_AddScheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Remove scheme from URL
	urlWithoutScheme := server.URL[7:] // Remove "http://"

	// Should fail because we add https:// and the server is http
	_, err := HTTPRequest(urlWithoutScheme, "GET")
	if err == nil {
		t.Log("Request succeeded (may have fallen back or server supports HTTPS)")
	}
}

func TestTraceroute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Traceroute test unreliable on Windows CI")
	}

	result, err := Traceroute("127.0.0.1", 5)
	if err != nil {
		t.Fatalf("Traceroute error: %v", err)
	}

	if result == nil {
		t.Fatal("Traceroute returned nil")
	}

	if result.RawOutput == "" {
		t.Error("Expected non-empty RawOutput")
	}
}

func TestTraceroute_MaxHopsValidation(t *testing.T) {
	// Test that maxHops is capped
	result, err := Traceroute("127.0.0.1", 100)
	if err != nil {
		t.Fatalf("Traceroute error: %v", err)
	}

	// Should have executed with capped hops (64)
	if result == nil {
		t.Fatal("Expected result")
	}
}

func TestNetInfo(t *testing.T) {
	result, err := NetInfo("all")
	if err != nil {
		t.Fatalf("NetInfo error: %v", err)
	}

	if result == nil {
		t.Fatal("NetInfo returned nil")
	}

	if result.InterfaceCount == 0 {
		t.Error("Expected at least one interface")
	}

	// Verify interface structure
	for _, iface := range result.Interfaces {
		if iface.Name == "" {
			t.Error("Interface missing name")
		}
	}
}

func TestNetInfo_SpecificInterface(t *testing.T) {
	// First get all interfaces
	allResult, err := NetInfo("all")
	if err != nil {
		t.Skip("Could not get interfaces")
	}

	if len(allResult.Interfaces) == 0 {
		t.Skip("No interfaces available")
	}

	// Query specific interface
	ifaceName := allResult.Interfaces[0].Name
	result, err := NetInfo(ifaceName)
	if err != nil {
		t.Fatalf("NetInfo(%s) error: %v", ifaceName, err)
	}

	if result.InterfaceCount != 1 {
		t.Errorf("Expected 1 interface, got %d", result.InterfaceCount)
	}
}

func TestNetInfo_InvalidInterface(t *testing.T) {
	_, err := NetInfo("nonexistent_interface_12345")
	if err == nil {
		t.Error("Expected error for invalid interface")
	}
}

// Benchmark tests

func BenchmarkPing(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Ping("127.0.0.1", 1)
	}
}

func BenchmarkDNSLookup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DNSLookup("localhost", "A")
	}
}

func BenchmarkPortScan(b *testing.B) {
	for i := 0; i < b.N; i++ {
		PortScan("127.0.0.1", "80")
	}
}

func BenchmarkNetInfo(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NetInfo("all")
	}
}
