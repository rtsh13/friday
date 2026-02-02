package system

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// BufferStats holds network buffer statistics and recommendations
type BufferStats struct {
	RMemMax              int      `json:"rmem_max"`
	WMemMax              int      `json:"wmem_max"`
	TCPRMemMin           int      `json:"tcp_rmem_min"`
	TCPRMemDefault       int      `json:"tcp_rmem_default"`
	TCPRMemMax           int      `json:"tcp_rmem_max"`
	TCPWMemMin           int      `json:"tcp_wmem_min"`
	TCPWMemDefault       int      `json:"tcp_wmem_default"`
	TCPWMemMax           int      `json:"tcp_wmem_max"`
	RecommendedRMemMax   int      `json:"recommended_rmem_max"`
	RecommendedWMemMax   int      `json:"recommended_wmem_max"`
	RecommendedTCPRMemMax int     `json:"recommended_tcp_rmem_max"`
	RecommendedTCPWMemMax int     `json:"recommended_tcp_wmem_max"`
	Warnings             []string `json:"warnings"`
	Recommendations      []string `json:"recommendations"`
}

// InspectNetworkBuffers reads and analyzes Linux kernel network buffer settings
func InspectNetworkBuffers() (map[string]interface{}, error) {
	stats := &BufferStats{
		Warnings:        []string{},
		Recommendations: []string{},
	}

	// Read core buffer settings
	if val, err := readProcValue("/proc/sys/net/core/rmem_max"); err == nil {
		stats.RMemMax = val
	} else {
		return nil, fmt.Errorf("failed to read rmem_max: %w", err)
	}

	if val, err := readProcValue("/proc/sys/net/core/wmem_max"); err == nil {
		stats.WMemMax = val
	} else {
		return nil, fmt.Errorf("failed to read wmem_max: %w", err)
	}

	// Read TCP-specific buffer settings (3-tuple: min, default, max)
	if vals, err := readProcTuple("/proc/sys/net/ipv4/tcp_rmem"); err == nil {
		if len(vals) >= 3 {
			stats.TCPRMemMin = vals[0]
			stats.TCPRMemDefault = vals[1]
			stats.TCPRMemMax = vals[2]
		}
	} else {
		return nil, fmt.Errorf("failed to read tcp_rmem: %w", err)
	}

	if vals, err := readProcTuple("/proc/sys/net/ipv4/tcp_wmem"); err == nil {
		if len(vals) >= 3 {
			stats.TCPWMemMin = vals[0]
			stats.TCPWMemDefault = vals[1]
			stats.TCPWMemMax = vals[2]
		}
	} else {
		return nil, fmt.Errorf("failed to read tcp_wmem: %w", err)
	}

	// Set recommended values (optimized for high-bandwidth networks)
	// For 10Gbps networks: ~85MB for rmem_max, ~32MB for wmem_max
	stats.RecommendedRMemMax = 128 * 1024 * 1024  // 128MB
	stats.RecommendedWMemMax = 128 * 1024 * 1024  // 128MB
	stats.RecommendedTCPRMemMax = 64 * 1024 * 1024 // 64MB
	stats.RecommendedTCPWMemMax = 64 * 1024 * 1024 // 64MB

	// Analyze and generate warnings/recommendations
	if stats.RMemMax < stats.RecommendedRMemMax {
		warning := fmt.Sprintf("rmem_max is too low (%d bytes vs recommended %d bytes) - may cause packet drops on high-bandwidth connections",
			stats.RMemMax, stats.RecommendedRMemMax)
		stats.Warnings = append(stats.Warnings, warning)
		stats.Recommendations = append(stats.Recommendations,
			fmt.Sprintf("Increase rmem_max: sysctl -w net.core.rmem_max=%d", stats.RecommendedRMemMax))
	}

	if stats.WMemMax < stats.RecommendedWMemMax {
		warning := fmt.Sprintf("wmem_max is too low (%d bytes vs recommended %d bytes) - may limit send buffer for high-bandwidth connections",
			stats.WMemMax, stats.RecommendedWMemMax)
		stats.Warnings = append(stats.Warnings, warning)
		stats.Recommendations = append(stats.Recommendations,
			fmt.Sprintf("Increase wmem_max: sysctl -w net.core.wmem_max=%d", stats.RecommendedWMemMax))
	}

	if stats.TCPRMemMax < stats.RecommendedTCPRMemMax {
		warning := fmt.Sprintf("tcp_rmem max is too low (%d bytes vs recommended %d bytes) - limits TCP receive buffer",
			stats.TCPRMemMax, stats.RecommendedTCPRMemMax)
		stats.Warnings = append(stats.Warnings, warning)
		stats.Recommendations = append(stats.Recommendations,
			fmt.Sprintf("Increase tcp_rmem: sysctl -w 'net.ipv4.tcp_rmem=%d %d %d'",
				stats.TCPRMemMin, stats.TCPRMemDefault, stats.RecommendedTCPRMemMax))
	}

	if stats.TCPWMemMax < stats.RecommendedTCPWMemMax {
		warning := fmt.Sprintf("tcp_wmem max is too low (%d bytes vs recommended %d bytes) - limits TCP send buffer",
			stats.TCPWMemMax, stats.RecommendedTCPWMemMax)
		stats.Warnings = append(stats.Warnings, warning)
		stats.Recommendations = append(stats.Recommendations,
			fmt.Sprintf("Increase tcp_wmem: sysctl -w 'net.ipv4.tcp_wmem=%d %d %d'",
				stats.TCPWMemMin, stats.TCPWMemDefault, stats.RecommendedTCPWMemMax))
	}

	// Return as map for consistency with other functions
	result := map[string]interface{}{
		"rmem_max":                  stats.RMemMax,
		"wmem_max":                  stats.WMemMax,
		"tcp_rmem_min":              stats.TCPRMemMin,
		"tcp_rmem_default":          stats.TCPRMemDefault,
		"tcp_rmem_max":              stats.TCPRMemMax,
		"tcp_wmem_min":              stats.TCPWMemMin,
		"tcp_wmem_default":          stats.TCPWMemDefault,
		"tcp_wmem_max":              stats.TCPWMemMax,
		"recommended_rmem_max":      stats.RecommendedRMemMax,
		"recommended_wmem_max":      stats.RecommendedWMemMax,
		"recommended_tcp_rmem_max":  stats.RecommendedTCPRMemMax,
		"recommended_tcp_wmem_max":  stats.RecommendedTCPWMemMax,
		"warnings":                  stats.Warnings,
		"recommendations":           stats.Recommendations,
		"status":                    "ok",
	}

	if len(stats.Warnings) > 0 {
		result["status"] = "warning"
	}

	return result, nil
}

// ReadProcValue reads a single integer value from a /proc file (exported for testing)
func ReadProcValue(path string) (int, error) {
	return readProcValue(path)
}

// readProcValue reads a single integer value from a /proc file
func readProcValue(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("cannot read %s: %w", path, err)
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return 0, fmt.Errorf("cannot parse value from %s: %w", path, err)
	}

	return val, nil
}

// ReadProcTuple reads multiple space-separated integers from a /proc file (exported for testing)
func ReadProcTuple(path string) ([]int, error) {
	return readProcTuple(path)
}

// readProcTuple reads multiple space-separated integers from a /proc file
func readProcTuple(path string) ([]int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	parts := strings.Fields(strings.TrimSpace(string(content)))
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty content from %s", path)
	}

	var values []int
	for _, part := range parts {
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("cannot parse value '%s' from %s: %w", part, path, err)
		}
		values = append(values, val)
	}

	return values, nil
}