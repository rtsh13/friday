// Package network provides network diagnostic functions.
package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// Ping
// ============================================================================

// PingResult holds the result of a ping operation.
type PingResult struct {
	Reachable         bool    `json:"reachable"`
	PacketsSent       int     `json:"packets_sent"`
	PacketsReceived   int     `json:"packets_received"`
	PacketLossPercent float64 `json:"packet_loss_percent"`
	MinLatencyMs      float64 `json:"min_latency_ms"`
	AvgLatencyMs      float64 `json:"avg_latency_ms"`
	MaxLatencyMs      float64 `json:"max_latency_ms"`
	RawOutput         string  `json:"raw_output"`
}

// Ping sends ICMP ping packets to a host.
func Ping(host string, count int) (*PingResult, error) {
	if count <= 0 {
		count = 3
	}
	if count > 20 {
		count = 20
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(count*5)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	countStr := strconv.Itoa(count)

	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", countStr, host)
	} else {
		cmd = exec.CommandContext(ctx, "ping", "-c", countStr, host)
	}

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	result := &PingResult{
		PacketsSent: count,
		RawOutput:   outputStr,
	}

	if err != nil {
		result.Reachable = false
		result.PacketsReceived = 0
		result.PacketLossPercent = 100
		return result, nil // Return result, not error - ping failure is a valid result
	}

	// Parse output
	result.Reachable = true
	parsePingOutput(outputStr, result)

	return result, nil
}

// parsePingOutput extracts statistics from ping output.
func parsePingOutput(output string, result *PingResult) {
	// Try to extract packet stats
	// Linux/macOS: "3 packets transmitted, 3 received, 0% packet loss"
	// Windows: "Packets: Sent = 3, Received = 3, Lost = 0 (0% loss)"

	lossRegex := regexp.MustCompile(`(\d+)%\s*(?:packet\s*)?loss`)
	if matches := lossRegex.FindStringSubmatch(output); len(matches) > 1 {
		if loss, err := strconv.ParseFloat(matches[1], 64); err == nil {
			result.PacketLossPercent = loss
			result.PacketsReceived = result.PacketsSent - int(float64(result.PacketsSent)*loss/100)
		}
	}

	// Try to extract RTT stats
	// Linux/macOS: "rtt min/avg/max/mdev = 0.045/0.062/0.079/0.014 ms"
	// Windows: "Minimum = 1ms, Maximum = 2ms, Average = 1ms"

	if runtime.GOOS == "windows" {
		minRegex := regexp.MustCompile(`Minimum\s*=\s*(\d+)`)
		maxRegex := regexp.MustCompile(`Maximum\s*=\s*(\d+)`)
		avgRegex := regexp.MustCompile(`Average\s*=\s*(\d+)`)

		if m := minRegex.FindStringSubmatch(output); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				result.MinLatencyMs = v
			}
		}
		if m := maxRegex.FindStringSubmatch(output); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				result.MaxLatencyMs = v
			}
		}
		if m := avgRegex.FindStringSubmatch(output); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				result.AvgLatencyMs = v
			}
		}
	} else {
		rttRegex := regexp.MustCompile(`(?:rtt|round-trip)\s+min/avg/max(?:/mdev)?\s*=\s*([0-9.]+)/([0-9.]+)/([0-9.]+)`)
		if m := rttRegex.FindStringSubmatch(output); len(m) > 3 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				result.MinLatencyMs = v
			}
			if v, err := strconv.ParseFloat(m[2], 64); err == nil {
				result.AvgLatencyMs = v
			}
			if v, err := strconv.ParseFloat(m[3], 64); err == nil {
				result.MaxLatencyMs = v
			}
		}
	}

	// Determine reachability from loss
	if result.PacketLossPercent >= 100 {
		result.Reachable = false
	}
}

// ============================================================================
// DNS Lookup
// ============================================================================

// DNSRecord represents a single DNS record.
type DNSRecord struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// DNSResult holds the result of a DNS lookup.
type DNSResult struct {
	Records     []DNSRecord `json:"records"`
	RecordCount int         `json:"record_count"`
}

// DNSLookup queries DNS records for a domain.
func DNSLookup(domain string, recordType string) (*DNSResult, error) {
	recordType = strings.ToUpper(recordType)
	if recordType == "" {
		recordType = "ALL"
	}

	result := &DNSResult{
		Records: make([]DNSRecord, 0),
	}

	// A records
	if recordType == "ALL" || recordType == "A" {
		ips, err := net.LookupIP(domain)
		if err == nil {
			for _, ip := range ips {
				if ip.To4() != nil {
					result.Records = append(result.Records, DNSRecord{
						Type:  "A",
						Value: ip.String(),
					})
				}
			}
		}
	}

	// AAAA records
	if recordType == "ALL" || recordType == "AAAA" {
		ips, err := net.LookupIP(domain)
		if err == nil {
			for _, ip := range ips {
				if ip.To4() == nil && ip.To16() != nil {
					result.Records = append(result.Records, DNSRecord{
						Type:  "AAAA",
						Value: ip.String(),
					})
				}
			}
		}
	}

	// CNAME
	if recordType == "ALL" || recordType == "CNAME" {
		cname, err := net.LookupCNAME(domain)
		if err == nil && cname != "" && cname != domain+"." {
			result.Records = append(result.Records, DNSRecord{
				Type:  "CNAME",
				Value: strings.TrimSuffix(cname, "."),
			})
		}
	}

	// MX records
	if recordType == "ALL" || recordType == "MX" {
		mxs, err := net.LookupMX(domain)
		if err == nil {
			for _, mx := range mxs {
				result.Records = append(result.Records, DNSRecord{
					Type:  "MX",
					Value: fmt.Sprintf("%s (priority %d)", strings.TrimSuffix(mx.Host, "."), mx.Pref),
				})
			}
		}
	}

	// TXT records
	if recordType == "ALL" || recordType == "TXT" {
		txts, err := net.LookupTXT(domain)
		if err == nil {
			for _, txt := range txts {
				value := txt
				if len(value) > 100 {
					value = value[:100] + "..."
				}
				result.Records = append(result.Records, DNSRecord{
					Type:  "TXT",
					Value: value,
				})
			}
		}
	}

	result.RecordCount = len(result.Records)

	if result.RecordCount == 0 {
		return nil, fmt.Errorf("no DNS records found for %s", domain)
	}

	return result, nil
}

// ============================================================================
// Port Scan
// ============================================================================

// PortScanResult holds the result of a port scan.
type PortScanResult struct {
	OpenPorts    []int `json:"open_ports"`
	ClosedPorts  []int `json:"closed_ports"`
	TotalScanned int   `json:"total_scanned"`
	OpenCount    int   `json:"open_count"`
}

// CommonPorts is a list of commonly used ports.
var CommonPorts = []int{22, 80, 443, 3000, 3306, 5432, 6379, 8000, 8080, 8443, 9000, 27017}

// PortScan checks if TCP ports are open on a host.
func PortScan(host string, portsParam string) (*PortScanResult, error) {
	var ports []int

	if portsParam == "" || portsParam == "common" {
		ports = CommonPorts
	} else {
		for _, ps := range strings.Split(portsParam, ",") {
			ps = strings.TrimSpace(ps)
			if port, err := strconv.Atoi(ps); err == nil && port > 0 && port < 65536 {
				ports = append(ports, port)
			}
		}
	}

	if len(ports) == 0 {
		return nil, fmt.Errorf("no valid ports specified")
	}

	result := &PortScanResult{
		OpenPorts:    make([]int, 0),
		ClosedPorts:  make([]int, 0),
		TotalScanned: len(ports),
	}

	timeout := 2 * time.Second

	for _, port := range ports {
		addr := fmt.Sprintf("%s:%d", host, port)
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			result.ClosedPorts = append(result.ClosedPorts, port)
		} else {
			conn.Close()
			result.OpenPorts = append(result.OpenPorts, port)
		}
	}

	result.OpenCount = len(result.OpenPorts)

	return result, nil
}

// ============================================================================
// HTTP Request
// ============================================================================

// HTTPResult holds the result of an HTTP request.
type HTTPResult struct {
	StatusCode     int               `json:"status_code"`
	StatusText     string            `json:"status_text"`
	ResponseTimeMs int64             `json:"response_time_ms"`
	Headers        map[string]string `json:"headers"`
	Protocol       string            `json:"protocol"`
	Success        bool              `json:"success"`
}

// HTTPRequest makes an HTTP/HTTPS request and returns response info.
func HTTPRequest(url string, method string) (*HTTPResult, error) {
	method = strings.ToUpper(method)
	if method == "" {
		method = "GET"
	}

	// Ensure URL has scheme
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	req.Header.Set("User-Agent", "telemetry-debugger/1.0")

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	result := &HTTPResult{
		StatusCode:     resp.StatusCode,
		StatusText:     resp.Status,
		ResponseTimeMs: elapsed.Milliseconds(),
		Protocol:       resp.Proto,
		Headers:        make(map[string]string),
		Success:        resp.StatusCode >= 200 && resp.StatusCode < 400,
	}

	// Extract interesting headers
	interestingHeaders := []string{
		"Content-Type", "Server", "X-Powered-By",
		"Location", "Cache-Control", "Content-Length",
	}
	for _, h := range interestingHeaders {
		if v := resp.Header.Get(h); v != "" {
			result.Headers[h] = v
		}
	}

	return result, nil
}

// ============================================================================
// Traceroute
// ============================================================================

// TracerouteResult holds the result of a traceroute.
type TracerouteResult struct {
	Hops               []string `json:"hops"`
	DestinationReached bool     `json:"destination_reached"`
	TotalHops          int      `json:"total_hops"`
	RawOutput          string   `json:"raw_output"`
}

// Traceroute traces the network path to a host.
func Traceroute(host string, maxHops int) (*TracerouteResult, error) {
	if maxHops <= 0 {
		maxHops = 15
	}
	if maxHops > 64 {
		maxHops = 64
	}

	maxHopsStr := strconv.Itoa(maxHops)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "tracert", "-h", maxHopsStr, host)
	} else if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "traceroute", "-m", maxHopsStr, host)
	} else {
		cmd = exec.CommandContext(ctx, "traceroute", "-m", maxHopsStr, "-w", "2", host)
	}

	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	result := &TracerouteResult{
		Hops:      make([]string, 0),
		RawOutput: outputStr,
	}

	// Parse hops from output
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip header lines
		if strings.HasPrefix(line, "traceroute") || strings.HasPrefix(line, "Tracing") {
			continue
		}

		// Check if line starts with a hop number
		if len(line) > 0 && (line[0] >= '0' && line[0] <= '9' || line[0] == ' ') {
			result.Hops = append(result.Hops, line)
		}
	}

	result.TotalHops = len(result.Hops)
	result.DestinationReached = strings.Contains(outputStr, host) &&
		!strings.Contains(outputStr, "* * *")

	return result, nil
}

// ============================================================================
// Network Info
// ============================================================================

// InterfaceInfo holds information about a network interface.
type InterfaceInfo struct {
	Name       string   `json:"name"`
	MAC        string   `json:"mac"`
	MTU        int      `json:"mtu"`
	Flags      []string `json:"flags"`
	Addresses  []string `json:"addresses"`
	IsUp       bool     `json:"is_up"`
	IsLoopback bool     `json:"is_loopback"`
}

// NetInfoResult holds the result of network info query.
type NetInfoResult struct {
	Interfaces     []InterfaceInfo `json:"interfaces"`
	InterfaceCount int             `json:"interface_count"`
}

// NetInfo retrieves local network interface information.
func NetInfo(filterInterface string) (*NetInfoResult, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
	}

	result := &NetInfoResult{
		Interfaces: make([]InterfaceInfo, 0),
	}

	for _, iface := range ifaces {
		// Apply filter
		if filterInterface != "" && filterInterface != "all" && iface.Name != filterInterface {
			continue
		}

		// Skip loopback and down interfaces for "all" query
		if filterInterface == "" || filterInterface == "all" {
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			if iface.Flags&net.FlagUp == 0 {
				continue
			}
		}

		info := InterfaceInfo{
			Name:       iface.Name,
			MAC:        iface.HardwareAddr.String(),
			MTU:        iface.MTU,
			Flags:      make([]string, 0),
			Addresses:  make([]string, 0),
			IsUp:       iface.Flags&net.FlagUp != 0,
			IsLoopback: iface.Flags&net.FlagLoopback != 0,
		}

		// Parse flags
		if iface.Flags&net.FlagUp != 0 {
			info.Flags = append(info.Flags, "UP")
		}
		if iface.Flags&net.FlagBroadcast != 0 {
			info.Flags = append(info.Flags, "BROADCAST")
		}
		if iface.Flags&net.FlagMulticast != 0 {
			info.Flags = append(info.Flags, "MULTICAST")
		}
		if iface.Flags&net.FlagLoopback != 0 {
			info.Flags = append(info.Flags, "LOOPBACK")
		}

		// Get addresses
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			info.Addresses = append(info.Addresses, addr.String())
		}

		result.Interfaces = append(result.Interfaces, info)
	}

	result.InterfaceCount = len(result.Interfaces)

	if result.InterfaceCount == 0 {
		return nil, fmt.Errorf("no matching interfaces found")
	}

	return result, nil
}
