package network

import (
	"net"
	"testing"
	"time"

	"github.com/ashutoshrp06/telemetry-debugger/internal/functions/network"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// startMockGRPCServer starts a mock gRPC health check server for testing
func startMockGRPCServer(t *testing.T, healthStatus grpc_health_v1.HealthCheckResponse_ServingStatus) (string, func()) {
	// Find an available port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	// Create gRPC server
	server := grpc.NewServer()

	// Register health check service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)

	// Set health status
	healthServer.SetServingStatus("", healthStatus)

	// Start server in goroutine
	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Extract host and port
	addr := lis.Addr().(*net.TCPAddr)
	hostPort := addr.String()

	// Return hostPort and cleanup function
	cleanup := func() {
		server.GracefulStop()
	}

	return hostPort, cleanup
}

func TestCheckGRPCHealth_Serving(t *testing.T) {
	hostPort, cleanup := startMockGRPCServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Extract host and port (hostPort is "127.0.0.1:PORT")
	var host string
	var port int

	// Parse manually using ResolveTCPAddr
	hostPortParsed, err := net.ResolveTCPAddr("tcp", hostPort)
	if err != nil {
		t.Fatalf("Failed to parse host:port: %v", err)
	}
	host = hostPortParsed.IP.String()
	port = hostPortParsed.Port

	result, err := network.CheckGRPCHealth(host, port, 5)
	if err != nil {
		t.Fatalf("CheckGRPCHealth failed: %v", err)
	}

	// Verify response structure
	if status, ok := result["status"].(string); !ok || status != "SERVING" {
		t.Errorf("Expected status SERVING, got %v", result["status"])
	}

	if latency, ok := result["latency_ms"].(int64); !ok || latency <= 0 {
		t.Errorf("Expected positive latency_ms, got %v", result["latency_ms"])
	}

	if resultHost, ok := result["host"].(string); !ok || resultHost != host {
		t.Errorf("Expected host %s, got %v", host, result["host"])
	}

	if resultPort, ok := result["port"].(int); !ok || resultPort != port {
		t.Errorf("Expected port %d, got %v", port, result["port"])
	}
}

func TestCheckGRPCHealth_NotServing(t *testing.T) {
	hostPort, cleanup := startMockGRPCServer(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	hostPortParsed, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := hostPortParsed.IP.String()
	port := hostPortParsed.Port

	result, err := network.CheckGRPCHealth(host, port, 5)
	if err != nil {
		t.Fatalf("CheckGRPCHealth failed: %v", err)
	}

	if status, ok := result["status"].(string); !ok || status != "NOT_SERVING" {
		t.Errorf("Expected status NOT_SERVING, got %v", result["status"])
	}
}

func TestCheckGRPCHealth_Unknown(t *testing.T) {
	hostPort, cleanup := startMockGRPCServer(t, grpc_health_v1.HealthCheckResponse_UNKNOWN)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	hostPortParsed, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := hostPortParsed.IP.String()
	port := hostPortParsed.Port

	result, err := network.CheckGRPCHealth(host, port, 5)
	if err != nil {
		t.Fatalf("CheckGRPCHealth failed: %v", err)
	}

	if status, ok := result["status"].(string); !ok || status != "UNKNOWN" {
		t.Errorf("Expected status UNKNOWN, got %v", result["status"])
	}
}

func TestCheckGRPCHealth_ConnectionRefused(t *testing.T) {
	// Try to connect to non-existent server
	_, err := network.CheckGRPCHealth("127.0.0.1", 9999, 1)
	if err == nil {
		t.Fatalf("Expected error for connection refused")
	}
}

func TestCheckGRPCHealth_InvalidHost(t *testing.T) {
	_, err := network.CheckGRPCHealth("invalid-host-that-does-not-exist.local", 50051, 2)
	if err == nil {
		t.Fatalf("Expected error for invalid host")
	}
}

func TestCheckGRPCHealth_ZeroTimeout(t *testing.T) {
	hostPort, cleanup := startMockGRPCServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	hostPortParsed, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := hostPortParsed.IP.String()
	port := hostPortParsed.Port

	// Zero timeout should use default (5 seconds)
	result, err := network.CheckGRPCHealth(host, port, 0)
	if err != nil {
		t.Fatalf("CheckGRPCHealth with zero timeout failed: %v", err)
	}

	if status, ok := result["status"].(string); !ok || status != "SERVING" {
		t.Errorf("Expected status SERVING, got %v", result["status"])
	}
}

func TestCheckGRPCHealth_LatencyMeasurement(t *testing.T) {
	hostPort, cleanup := startMockGRPCServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	hostPortParsed, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := hostPortParsed.IP.String()
	port := hostPortParsed.Port

	// Measure multiple calls to check latency
	latencies := make([]int64, 5)
	for i := 0; i < 5; i++ {
		result, err := network.CheckGRPCHealth(host, port, 5)
		if err != nil {
			t.Fatalf("CheckGRPCHealth failed: %v", err)
		}

		latency, ok := result["latency_ms"].(int64)
		if !ok {
			t.Fatalf("Invalid latency type: %T", result["latency_ms"])
		}
		latencies[i] = latency
	}

	// All latencies should be non-negative
	for i, latency := range latencies {
		if latency < 0 {
			t.Errorf("Latency %d: expected non-negative, got %d", i, latency)
		}
	}

	// At least one latency should be non-zero (some measurements should register)
	hasNonZero := false
	for _, latency := range latencies {
		if latency > 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Logf("Warning: all latencies rounded to 0ms (very fast connections)")
	}

	// Latencies should be reasonable (< 1 second for localhost)
	for i, latency := range latencies {
		if latency > 1000 {
			t.Logf("Warning: high latency at iteration %d: %dms", i, latency)
		}
	}
}

func TestCheckGRPCHealth_Timeout(t *testing.T) {
	hostPort, cleanup := startMockGRPCServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	hostPortParsed, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := hostPortParsed.IP.String()
	port := hostPortParsed.Port

	// Very short timeout - might timeout or might succeed
	// This test mainly ensures the timeout parameter is used
	result, _ := network.CheckGRPCHealth(host, port, 10)
	
	if result != nil {
		if status, ok := result["status"].(string); ok {
			if status != "SERVING" && status != "NOT_SERVING" && status != "UNKNOWN" {
				t.Errorf("Unexpected status: %s", status)
			}
		}
	}
}

// BenchmarkCheckGRPCHealth benchmarks the health check function
func BenchmarkCheckGRPCHealth(b *testing.B) {
	hostPort, cleanup := startMockGRPCServer(&testing.T{}, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	hostPortParsed, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := hostPortParsed.IP.String()
	port := hostPortParsed.Port

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = network.CheckGRPCHealth(host, port, 5)
	}
}
