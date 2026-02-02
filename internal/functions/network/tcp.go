package network

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// TCPStats holds parsed TCP connection statistics
type TCPStats struct {
	State                 string
	Port                  int
	Interface             string
	Retransmits           int
	SendQueueBytes        int
	RecvQueueBytes        int
	RecommendedBufferSize int
	Latency               float64 // RTT in milliseconds
}

// CheckTCPHealth analyzes TCP connection health using ss command
func CheckTCPHealth(iface string, port int) (map[string]interface{}, error) {
	// Execute ss command to get TCP stats for specific port
	stats, err := parseTCPStats(port)
	if err != nil {
		return nil, err
	}

	// Calculate recommended buffer size based on bandwidth-delay product
	// Formula: Buffer = RTT (ms) * Bandwidth (Mbps) / 8
	// Conservative estimate: assume 100Mbps if RTT available, else default to 6MB
	recommendedBuffer := calculateRecommendedBuffer(stats.Latency)

	return map[string]interface{}{
		"state":                   stats.State,
		"port":                    port,
		"interface":               iface,
		"retransmits":             stats.Retransmits,
		"send_queue_bytes":        stats.SendQueueBytes,
		"recv_queue_bytes":        stats.RecvQueueBytes,
		"rtt_ms":                  stats.Latency,
		"recommended_buffer_size": recommendedBuffer,
	}, nil
}

// ParseTCPStats executes ss command and parses the output (exported for testing)
func ParseTCPStats(port int) (*TCPStats, error) {
	return parseTCPStats(port)
}

// parseTCPStats executes ss command and parses the output
func parseTCPStats(port int) (*TCPStats, error) {
	// Build ss command: ss -ti sport = :PORT
	// -t: TCP sockets
	// -i: show TCP info
	cmd := exec.Command("ss", "-ti", fmt.Sprintf("sport = :%d", port))

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute ss: %w", err)
	}

	output := out.String()
	if output == "" {
		return nil, fmt.Errorf("no TCP connection found on port %d", port)
	}

	return parseSSOutput(output, port)
}

// parseSSOutput parses the output from ss command
// Output format:
// State    Recv-Q Send-Q Local Address:Port Peer Address:Port
// ESTAB    0      10     127.0.0.1:50051  127.0.0.1:54321
//          cubic wscale:7,7 rto:204 rtt:0.5/0.25 send 167.7Mbps rcv_space:29200

// ParseSSOutput parses the ss command output (exported for testing)
func ParseSSOutput(output string, port int) (*TCPStats, error) {
	return parseSSOutput(output, port)
}

func parseSSOutput(output string, port int) (*TCPStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	stats := &TCPStats{
		Port:        port,
		State:       "UNKNOWN",
		Retransmits: 0,
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "State") {
			continue
		}

		// Parse state line (e.g., "ESTAB 0 10 127.0.0.1:50051 127.0.0.1:54321")
		if strings.HasPrefix(line, "ESTAB") || strings.HasPrefix(line, "LISTEN") ||
			strings.HasPrefix(line, "TIME-WAIT") || strings.HasPrefix(line, "CLOSE-WAIT") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				stats.State = fields[0]
				// Parse recv queue (Recv-Q)
				if recvQ, err := strconv.Atoi(fields[1]); err == nil {
					stats.RecvQueueBytes = recvQ
				}
				// Parse send queue (Send-Q)
				if sendQ, err := strconv.Atoi(fields[2]); err == nil {
					stats.SendQueueBytes = sendQ
				}
			}
		}

		// Parse TCP info line (contains rtt, retransmits, etc.)
		// Example: "cubic wscale:7,7 rto:204 rtt:0.5/0.25 retrans:5 send 167.7Mbps rcv_space:29200"
		if strings.Contains(line, "rtt:") || strings.Contains(line, "retrans:") {
			// Extract RTT
			rttRegex := regexp.MustCompile(`rtt:([0-9.]+)`)
			if matches := rttRegex.FindStringSubmatch(line); len(matches) > 1 {
				if rtt, err := strconv.ParseFloat(matches[1], 64); err == nil {
					stats.Latency = rtt // in milliseconds
				}
			}

			// Extract retransmits
			retransRegex := regexp.MustCompile(`retrans:(\d+)`)
			if matches := retransRegex.FindStringSubmatch(line); len(matches) > 1 {
				if retrans, err := strconv.Atoi(matches[1]); err == nil {
					stats.Retransmits = retrans
				}
			}

			// Note: rcv_space is kernel's estimate of recommended receive buffer
			// but it's not the actual queue size, so we don't override RecvQueueBytes
		}
	}

	if stats.State == "UNKNOWN" {
		return nil, fmt.Errorf("could not parse connection state from ss output")
	}

	return stats, nil
}

// calculateRecommendedBuffer computes recommended buffer size
// Formula: RTT (seconds) * Bandwidth (bits/sec) / 8 (to get bytes)
// Conservative: assume 1Gbps if RTT is very low, scale down for higher RTT
// CalculateRecommendedBuffer calculates recommended buffer size based on RTT
func CalculateRecommendedBuffer(rttMS float64) int {
	return calculateRecommendedBuffer(rttMS)
}

func calculateRecommendedBuffer(rttMS float64) int {
	// Default 6MB if no RTT available
	if rttMS <= 0 {
		return 6 * 1024 * 1024
	}

	// Convert RTT from ms to seconds
	rttSec := rttMS / 1000.0

	// Estimate bandwidth based on typical scenarios
	// Low RTT (< 1ms): assume fast local network, 10Gbps
	// Medium RTT (1-10ms): assume 1Gbps
	// High RTT (> 10ms): assume 100Mbps (WAN)
	var bandwidthBps float64
	switch {
	case rttMS < 1:
		bandwidthBps = 10 * 1e9 // 10Gbps
	case rttMS < 10:
		bandwidthBps = 1e9 // 1Gbps
	default:
		bandwidthBps = 100e6 // 100Mbps
	}

	// BDP = RTT * Bandwidth / 8 (to convert bits to bytes)
	bdp := int(rttSec * bandwidthBps / 8)

	// Cap at reasonable maximum (64MB)
	if bdp > 64*1024*1024 {
		bdp = 64 * 1024 * 1024
	}

	return bdp
}