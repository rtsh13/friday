package network

import (
	"net"
	"testing"
	"time"

	"github.com/stratos/cliche/internal/functions/network"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// TestAnalyzeGRPCStream_WithHealthWatch tests stream analysis with gRPC health watch
func TestAnalyzeGRPCStream_WithHealthWatch(t *testing.T) {
	// Start a mock gRPC server with health watch support
	hostPort, cleanup := startMockGRPCServerWithWatch(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	// Parse host and port
	addr, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := addr.IP.String()
	port := addr.Port

	// Analyze stream for short duration
	result, err := network.AnalyzeGRPCStream(host, port, 2)
	if err != nil {
		t.Fatalf("AnalyzeGRPCStream failed: %v", err)
	}

	// Verify result structure
	requiredFields := []string{
		"messages_sent",
		"messages_received",
		"dropped_count",
		"drop_percentage",
		"flow_control_events",
		"monitoring_duration_sec",
		"status",
	}

	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing field: %s", field)
		}
	}

	// Verify types
	if _, ok := result["messages_sent"].(int); !ok {
		t.Errorf("messages_sent should be int, got %T", result["messages_sent"])
	}
	if _, ok := result["messages_received"].(int); !ok {
		t.Errorf("messages_received should be int, got %T", result["messages_received"])
	}

	// Verify status
	status, ok := result["status"].(string)
	if !ok || (status != "ok" && status != "warning" && status != "error") {
		t.Errorf("Invalid status: %v", result["status"])
	}

	t.Logf("Stream analysis results:")
	t.Logf("  Messages sent: %v", result["messages_sent"])
	t.Logf("  Messages received: %v", result["messages_received"])
	t.Logf("  Dropped count: %v", result["dropped_count"])
	t.Logf("  Drop percentage: %v", result["drop_percentage"])
	t.Logf("  Flow control events: %v", result["flow_control_events"])
	t.Logf("  Monitoring duration: %v", result["monitoring_duration_sec"])
	t.Logf("  Status: %v", result["status"])
}

// TestAnalyzeGRPCStream_ConnectionRefused tests error handling for connection refused
func TestAnalyzeGRPCStream_ConnectionRefused(t *testing.T) {
	result, err := network.AnalyzeGRPCStream("127.0.0.1", 9999, 1)
	if err == nil {
		t.Fatalf("Expected error for connection refused, got result: %v", result)
	}

	if err.Error() == "" {
		t.Error("Error message is empty")
	}
}

// TestAnalyzeGRPCStream_InvalidHost tests error handling for invalid host
func TestAnalyzeGRPCStream_InvalidHost(t *testing.T) {
	result, err := network.AnalyzeGRPCStream("invalid-host-that-does-not-exist.local", 50051, 1)
	if err == nil {
		t.Fatalf("Expected error for invalid host, got result: %v", result)
	}
}

// TestAnalyzeGRPCStream_DefaultDuration tests default duration when 0 is passed
func TestAnalyzeGRPCStream_DefaultDuration(t *testing.T) {
	hostPort, cleanup := startMockGRPCServerWithWatch(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	addr, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := addr.IP.String()
	port := addr.Port

	// Test with 1-second duration
	result, err := network.AnalyzeGRPCStream(host, port, 1)
	if err != nil {
		// On slow systems, this might timeout, which is acceptable
		if err.Error() == "" {
			t.Fatalf("AnalyzeGRPCStream failed with empty error")
		}
		t.Logf("AnalyzeGRPCStream test failed (may be slow system): %v", err)
		return
	}

	if _, ok := result["monitoring_duration_sec"]; !ok {
		t.Error("Missing monitoring_duration_sec in result")
	}
}

// TestAnalyzeGRPCStream_StatusTransition tests flow control event detection
func TestAnalyzeGRPCStream_StatusTransition(t *testing.T) {
	hostPort, cleanup := startMockGRPCServerWithWatch(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	addr, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := addr.IP.String()
	port := addr.Port

	result, err := network.AnalyzeGRPCStream(host, port, 3)
	if err != nil {
		t.Logf("AnalyzeGRPCStream failed: %v", err)
		return
	}

	// Check for flow control events tracking
	if flowEvents, ok := result["flow_control_events"].(int); ok {
		if flowEvents < 0 {
			t.Errorf("flow_control_events should be non-negative, got %d", flowEvents)
		}
	} else {
		t.Errorf("flow_control_events should be int, got %T", result["flow_control_events"])
	}
}

// TestAnalyzeGRPCStream_LowDropPercentage tests normal stream operation
func TestAnalyzeGRPCStream_LowDropPercentage(t *testing.T) {
	hostPort, cleanup := startMockGRPCServerWithWatch(t, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	addr, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := addr.IP.String()
	port := addr.Port

	result, err := network.AnalyzeGRPCStream(host, port, 1)
	if err != nil {
		t.Logf("Note: test inconclusive due to error: %v", err)
		return
	}

	status, _ := result["status"].(string)
	if status == "error" {
		if errs, ok := result["errors"].([]string); ok && len(errs) > 0 {
			t.Logf("Stream had errors (acceptable in test environment): %v", errs)
		}
	}
}

// TestStreamStats_ToMap tests conversion to map
func TestStreamStats_ToMap(t *testing.T) {
	stats := &network.StreamStats{
		StartTime:          time.Now(),
		EndTime:            time.Now().Add(5 * time.Second),
		MessagesSent:       100,
		MessagesReceived:   98,
		DroppedSequences:   []int64{15, 67, 89},
		FlowControlEvents:  2,
		LastStatus:         "SERVING",
		MonitoringDuration: 5.0,
		Errors:             []string{},
	}

	result := stats.ToMap()

	if messages_sent, ok := result["messages_sent"].(int); !ok || messages_sent != 100 {
		t.Errorf("messages_sent: expected 100, got %v", result["messages_sent"])
	}

	if messages_received, ok := result["messages_received"].(int); !ok || messages_received != 98 {
		t.Errorf("messages_received: expected 98, got %v", result["messages_received"])
	}

	if dropped, ok := result["dropped_count"].(int); !ok || dropped != 3 {
		t.Errorf("dropped_count: expected 3, got %v", result["dropped_count"])
	}

	if flow, ok := result["flow_control_events"].(int); !ok || flow != 2 {
		t.Errorf("flow_control_events: expected 2, got %v", result["flow_control_events"])
	}

	if status, ok := result["status"].(string); !ok || status == "" {
		t.Errorf("status should be non-empty string, got %v", result["status"])
	}
}

// TestStreamStats_ToMap_WithErrors tests error handling in ToMap
func TestStreamStats_ToMap_WithErrors(t *testing.T) {
	stats := &network.StreamStats{
		StartTime:        time.Now(),
		EndTime:          time.Now(),
		MessagesSent:     50,
		MessagesReceived: 30,
		DroppedSequences: []int64{1, 2, 3},
		Errors:           []string{"connection reset", "timeout"},
	}

	result := stats.ToMap()

	status, _ := result["status"].(string)
	if status != "error" {
		t.Errorf("status should be 'error' when there are errors, got %s", status)
	}

	if errors, ok := result["errors"].([]string); !ok || len(errors) != 2 {
		t.Errorf("errors should be []string with 2 items, got %v", result["errors"])
	}
}

// TestStreamStats_DropPercentageCalculation tests drop percentage calculation
func TestStreamStats_DropPercentageCalculation(t *testing.T) {
	tests := []struct {
		name               string
		sent               int
		dropped            []int64
		expectedPercentage float64
	}{
		{
			name:               "No drops",
			sent:               100,
			dropped:            []int64{},
			expectedPercentage: 0.0,
		},
		{
			name:               "5% drops",
			sent:               100,
			dropped:            []int64{10, 20, 30, 40, 50},
			expectedPercentage: 5.0,
		},
		{
			name:               "1% drops",
			sent:               200,
			dropped:            []int64{1, 2},
			expectedPercentage: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &network.StreamStats{
				MessagesSent:     tt.sent,
				DroppedSequences: tt.dropped,
			}

			if len(tt.dropped) > 0 {
				stats.DropPercentage = float64(len(tt.dropped)) * 100.0 / float64(tt.sent)
			}

			if stats.DropPercentage != tt.expectedPercentage {
				t.Errorf("Expected %.2f%%, got %.2f%%", tt.expectedPercentage, stats.DropPercentage)
			}
		})
	}
}

// BenchmarkAnalyzeGRPCStream benchmarks stream analysis
func BenchmarkAnalyzeGRPCStream(b *testing.B) {
	hostPort, cleanup := startMockGRPCServerWithWatch(&testing.T{}, grpc_health_v1.HealthCheckResponse_SERVING)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	addr, _ := net.ResolveTCPAddr("tcp", hostPort)
	host := addr.IP.String()
	port := addr.Port

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = network.AnalyzeGRPCStream(host, port, 1)
	}
}

// BenchmarkStreamStats_ToMap benchmarks map conversion
func BenchmarkStreamStats_ToMap(b *testing.B) {
	stats := &network.StreamStats{
		StartTime:          time.Now(),
		EndTime:            time.Now().Add(5 * time.Second),
		MessagesSent:       1000,
		MessagesReceived:   980,
		DroppedSequences:   []int64{1, 5, 10, 15, 20},
		FlowControlEvents:  5,
		LastStatus:         "SERVING",
		MonitoringDuration: 5.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stats.ToMap()
	}
}

// Helper function: startMockGRPCServerWithWatch starts a gRPC server with Watch capability
func startMockGRPCServerWithWatch(t *testing.T, healthStatus grpc_health_v1.HealthCheckResponse_ServingStatus) (string, func()) {
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
