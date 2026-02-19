package network

import (
	"fmt"
	"testing"

	"github.com/friday/internal/functions/network"
)

func TestParseSSOutput_EstablishedConnection(t *testing.T) {
	// Real ss output for an ESTABLISHED connection
	ssOutput := `State    Recv-Q Send-Q Local Address:Port  Peer Address:Port Process
ESTAB    0      10     127.0.0.1:50051      127.0.0.1:54321   users:(("app",pid=1234,fd=5))
         cubic wscale:7,7 rto:204 rtt:0.5/0.25 retrans:5 send 167.7Mbps rcv_space:29200`

	stats, err := network.ParseSSOutput(ssOutput, 50051)
	if err != nil {
		t.Fatalf("parseSSOutput failed: %v", err)
	}

	tests := []struct {
		name     string
		expected interface{}
		actual   interface{}
	}{
		{"State", "ESTAB", stats.State},
		{"Port", 50051, stats.Port},
		{"RecvQueueBytes", 0, stats.RecvQueueBytes},
		{"SendQueueBytes", 10, stats.SendQueueBytes},
		{"Retransmits", 5, stats.Retransmits},
		{"Latency", 0.5, stats.Latency},
	}

	for _, tt := range tests {
		if tt.expected != tt.actual {
			t.Errorf("%s: expected %v, got %v", tt.name, tt.expected, tt.actual)
		}
	}
}

func TestParseSSOutput_HighRetransmits(t *testing.T) {
	// Connection with high retransmit count (network issues)
	ssOutput := `State    Recv-Q Send-Q Local Address:Port  Peer Address:Port
ESTAB    50     150    10.0.0.1:50051      10.0.0.2:54321
         cubic wscale:7,7 rto:302 rtt:2.5/1.2 retrans:47 send 50.2Mbps rcv_space:14600`

	stats, err := network.ParseSSOutput(ssOutput, 50051)
	if err != nil {
		t.Fatalf("parseSSOutput failed: %v", err)
	}

	if stats.Retransmits != 47 {
		t.Errorf("Retransmits: expected 47, got %d", stats.Retransmits)
	}
	if stats.SendQueueBytes != 150 {
		t.Errorf("SendQueueBytes: expected 150, got %d", stats.SendQueueBytes)
	}
	if stats.RecvQueueBytes != 50 {
		t.Errorf("RecvQueueBytes: expected 50, got %d", stats.RecvQueueBytes)
	}
}

func TestParseSSOutput_ListeningSocket(t *testing.T) {
	// LISTEN state socket
	ssOutput := `State    Recv-Q Send-Q Local Address:Port  Peer Address:Port
LISTEN   0      128    0.0.0.0:50051        0.0.0.0:*`

	stats, err := network.ParseSSOutput(ssOutput, 50051)
	if err != nil {
		t.Fatalf("parseSSOutput failed: %v", err)
	}

	if stats.State != "LISTEN" {
		t.Errorf("State: expected LISTEN, got %s", stats.State)
	}
}

func TestParseSSOutput_TimeWaitState(t *testing.T) {
	// TIME-WAIT state (connection closing)
	ssOutput := `State    Recv-Q Send-Q Local Address:Port  Peer Address:Port
TIME-WAIT 0      0     127.0.0.1:50051     127.0.0.1:54321`

	stats, err := network.ParseSSOutput(ssOutput, 50051)
	if err != nil {
		t.Fatalf("parseSSOutput failed: %v", err)
	}

	if stats.State != "TIME-WAIT" {
		t.Errorf("State: expected TIME-WAIT, got %s", stats.State)
	}
}

func TestParseSSOutput_InvalidInput(t *testing.T) {
	ssOutput := "invalid output"

	_, err := network.ParseSSOutput(ssOutput, 50051)
	if err == nil {
		t.Fatal("parseSSOutput should fail on invalid output")
	}
}

func TestCalculateRecommendedBuffer(t *testing.T) {
	tests := []struct {
		name        string
		rttMS       float64
		minExpected int
		maxExpected int
		description string
	}{
		{
			name:        "ZeroRTT",
			rttMS:       0,
			minExpected: 6 * 1024 * 1024,
			maxExpected: 6 * 1024 * 1024,
			description: "should return default 6MB for zero RTT",
		},
		{
			name:        "LowRTT",
			rttMS:       0.5,
			minExpected: 625000, // 0.5ms @ 10Gbps: 0.0005 * 1e10 / 8 = 625000
			maxExpected: 625000,
			description: "low RTT should estimate high bandwidth (10Gbps)",
		},
		{
			name:        "MediumRTT",
			rttMS:       5,
			minExpected: 625000, // 5ms @ 1Gbps: 0.005 * 1e9 / 8 = 625000
			maxExpected: 625000,
			description: "medium RTT should assume 1Gbps",
		},
		{
			name:        "HighRTT",
			rttMS:       50,
			minExpected: 625000, // 50ms @ 100Mbps: 0.05 * 1e8 / 8 = 625000
			maxExpected: 625000,
			description: "high RTT should assume 100Mbps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := network.CalculateRecommendedBuffer(tt.rttMS)
			if result < tt.minExpected || result > tt.maxExpected {
				t.Errorf("%s: expected between %d and %d, got %d",
					tt.description, tt.minExpected, tt.maxExpected, result)
			}
		})
	}
}

// TestCalculateRecommendedBuffer_Specific tests specific buffer calculations
func TestCalculateRecommendedBuffer_Specific(t *testing.T) {
	tests := []struct {
		rttMS    float64
		expected int
	}{
		// BDP formula: RTT_seconds * Bandwidth_bps / 8
		// All these calculate to roughly 625KB because the bandwidth is scaled inversely with RTT
		// 0.5ms @ 10Gbps: 0.0005 * 1e10 / 8 = 625000 bytes
		{0.5, 625000},

		// 5ms @ 1Gbps: 0.005 * 1e9 / 8 = 625000 bytes
		{5, 625000},

		// 50ms @ 100Mbps: 0.05 * 1e8 / 8 = 625000 bytes
		{50, 625000},
	}

	for _, tt := range tests {
		result := network.CalculateRecommendedBuffer(tt.rttMS)
		if result != tt.expected {
			t.Errorf("RTT %.1fms: expected %d, got %d",
				tt.rttMS, tt.expected, result)
		}
	}
}

// BenchmarkParseSSOutput benchmarks the parsing function
func BenchmarkParseSSOutput(b *testing.B) {
	ssOutput := `State    Recv-Q Send-Q Local Address:Port  Peer Address:Port
ESTAB    0      10     127.0.0.1:50051      127.0.0.1:54321
         cubic wscale:7,7 rto:204 rtt:0.5/0.25 retrans:5 send 167.7Mbps rcv_space:29200`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = network.ParseSSOutput(ssOutput, 50051)
	}
}

// BenchmarkCalculateRecommendedBuffer benchmarks buffer calculation
func BenchmarkCalculateRecommendedBuffer(b *testing.B) {
	rttValues := []float64{0.5, 5, 50, 100}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, rtt := range rttValues {
			_ = network.CalculateRecommendedBuffer(rtt)
		}
	}
}

// TestCheckTCPHealth_OutputStructure tests the main function
func TestCheckTCPHealth_OutputStructure(t *testing.T) {
	result, err := network.CheckTCPHealth("eth0", 50051)

	// On systems without ss or no active connections, this may fail
	// That's expected for cross-platform testing
	if err != nil {
		t.Logf("CheckTCPHealth failed (expected on non-Linux): %v", err)
		return
	}

	// Verify output structure
	expectedKeys := []string{"state", "port", "interface", "retransmits", "send_queue_bytes", "recv_queue_bytes", "recommended_buffer_size"}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("Missing key in output: %s", key)
		}
	}

	// Verify types
	if _, ok := result["port"].(int); !ok {
		t.Errorf("port should be int, got %T", result["port"])
	}
	if _, ok := result["state"].(string); !ok {
		t.Errorf("state should be string, got %T", result["state"])
	}
}

// TestParseSSOutput_VariousStates tests parsing of different TCP states
func TestParseSSOutput_VariousStates(t *testing.T) {
	states := []string{
		"ESTAB",
		"LISTEN",
		"TIME-WAIT",
		"CLOSE-WAIT",
	}

	for _, state := range states {
		t.Run(fmt.Sprintf("State_%s", state), func(t *testing.T) {
			ssOutput := fmt.Sprintf(`State    Recv-Q Send-Q Local Address:Port  Peer Address:Port
%s 0      0     127.0.0.1:50051     127.0.0.1:54321`, state)

			stats, err := network.ParseSSOutput(ssOutput, 50051)
			if err != nil {
				t.Fatalf("parseSSOutput failed for %s: %v", state, err)
			}

			if stats.State != state {
				t.Errorf("Expected state %s, got %s", state, stats.State)
			}
		})
	}
}

// TestParseSSOutput_EdgeCase_EmptyQueues tests empty send/recv queues
func TestParseSSOutput_EdgeCase_EmptyQueues(t *testing.T) {
	ssOutput := `State    Recv-Q Send-Q Local Address:Port  Peer Address:Port
ESTAB    0      0      127.0.0.1:50051      127.0.0.1:54321
         cubic wscale:7,7 rto:204 rtt:0.1/0.05 retrans:0`

	stats, err := network.ParseSSOutput(ssOutput, 50051)
	if err != nil {
		t.Fatalf("parseSSOutput failed: %v", err)
	}

	if stats.SendQueueBytes != 0 || stats.RecvQueueBytes != 0 || stats.Retransmits != 0 {
		t.Errorf("Expected all zeros for healthy connection, got retrans=%d, send=%d, recv=%d",
			stats.Retransmits, stats.SendQueueBytes, stats.RecvQueueBytes)
	}
}
