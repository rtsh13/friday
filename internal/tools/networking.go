// Package tools provides built-in networking tools for cliche.
package tools

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/stratos/cliche/pkg/models"
)

// ============================================================================
// Ping Tool
// ============================================================================

// PingTool checks host reachability via ICMP ping.
type PingTool struct{}

func NewPingTool() *PingTool { return &PingTool{} }

func (p *PingTool) Name() string { return "ping" }

func (p *PingTool) Description() string {
	return "Send ICMP ping to check if a host is reachable. Returns latency and packet loss info."
}

func (p *PingTool) Parameters() []Parameter {
	return []Parameter{
		{Name: "host", Type: "string", Description: "Hostname or IP address to ping", Required: true},
		{Name: "count", Type: "string", Description: "Number of ping packets", Required: false, Default: "3"},
	}
}

func (p *PingTool) Execute(ctx context.Context, params map[string]string) models.ToolResult {
	host := params["host"]
	count := params["count"]

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", count, host)
	} else {
		cmd = exec.CommandContext(ctx, "ping", "-c", count, host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return models.ToolResult{
			Success: false,
			Output:  string(output),
			Error:   fmt.Sprintf("Host %s is unreachable: %v", host, err),
		}
	}

	// Parse and summarize output
	summary := parsePingOutput(string(output))
	return models.ToolResult{
		Success: true,
		Output:  summary,
	}
}

func parsePingOutput(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Keep statistics lines
		if strings.Contains(line, "packets") ||
			strings.Contains(line, "rtt") ||
			strings.Contains(line, "round-trip") ||
			strings.Contains(line, "avg") ||
			strings.Contains(line, "loss") {
			result = append(result, line)
		}
	}

	if len(result) == 0 {
		return output // Return full output if parsing fails
	}
	return strings.Join(result, "\n")
}

// ============================================================================
// DNS Lookup Tool
// ============================================================================

// DNSLookupTool queries DNS records.
type DNSLookupTool struct{}

func NewDNSLookupTool() *DNSLookupTool { return &DNSLookupTool{} }

func (d *DNSLookupTool) Name() string { return "dns-lookup" }

func (d *DNSLookupTool) Description() string {
	return "Query DNS records for a domain. Returns A, AAAA, CNAME, MX, and TXT records."
}

func (d *DNSLookupTool) Parameters() []Parameter {
	return []Parameter{
		{Name: "domain", Type: "string", Description: "Domain name to look up", Required: true},
		{Name: "type", Type: "string", Description: "Record type: all, A, AAAA, MX, TXT, CNAME", Required: false, Default: "all"},
	}
}

func (d *DNSLookupTool) Execute(ctx context.Context, params map[string]string) models.ToolResult {
	domain := params["domain"]
	recordType := strings.ToUpper(params["type"])

	var results []string

	// A records
	if recordType == "ALL" || recordType == "A" {
		ips, err := net.LookupIP(domain)
		if err == nil {
			for _, ip := range ips {
				if ip.To4() != nil {
					results = append(results, fmt.Sprintf("A: %s", ip))
				}
			}
		}
	}

	// AAAA records
	if recordType == "ALL" || recordType == "AAAA" {
		ips, err := net.LookupIP(domain)
		if err == nil {
			for _, ip := range ips {
				if ip.To4() == nil {
					results = append(results, fmt.Sprintf("AAAA: %s", ip))
				}
			}
		}
	}

	// CNAME
	if recordType == "ALL" || recordType == "CNAME" {
		cname, err := net.LookupCNAME(domain)
		if err == nil && cname != domain+"." {
			results = append(results, fmt.Sprintf("CNAME: %s", cname))
		}
	}

	// MX records
	if recordType == "ALL" || recordType == "MX" {
		mxs, err := net.LookupMX(domain)
		if err == nil {
			for _, mx := range mxs {
				results = append(results, fmt.Sprintf("MX: %s (priority %d)", mx.Host, mx.Pref))
			}
		}
	}

	// TXT records
	if recordType == "ALL" || recordType == "TXT" {
		txts, err := net.LookupTXT(domain)
		if err == nil {
			for _, txt := range txts {
				// Truncate long TXT records
				if len(txt) > 100 {
					txt = txt[:100] + "..."
				}
				results = append(results, fmt.Sprintf("TXT: %s", txt))
			}
		}
	}

	if len(results) == 0 {
		return models.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("No DNS records found for %s", domain),
		}
	}

	return models.ToolResult{
		Success: true,
		Output:  strings.Join(results, "\n"),
	}
}

// ============================================================================
// Port Scan Tool
// ============================================================================

// PortScanTool checks if TCP ports are open.
type PortScanTool struct{}

func NewPortScanTool() *PortScanTool { return &PortScanTool{} }

func (p *PortScanTool) Name() string { return "port-scan" }

func (p *PortScanTool) Description() string {
	return "Check if TCP ports are open on a host. Useful for checking service availability."
}

func (p *PortScanTool) Parameters() []Parameter {
	return []Parameter{
		{Name: "host", Type: "string", Description: "Hostname or IP to scan", Required: true},
		{Name: "ports", Type: "string", Description: "Comma-separated ports (e.g., '22,80,443') or 'common' for common ports", Required: false, Default: "common"},
	}
}

func (p *PortScanTool) Execute(ctx context.Context, params map[string]string) models.ToolResult {
	host := params["host"]
	portsParam := params["ports"]

	var ports []int
	if portsParam == "common" {
		ports = []int{22, 80, 443, 3000, 3306, 5432, 6379, 8080, 8443, 27017}
	} else {
		for _, ps := range strings.Split(portsParam, ",") {
			ps = strings.TrimSpace(ps)
			if port, err := strconv.Atoi(ps); err == nil && port > 0 && port < 65536 {
				ports = append(ports, port)
			}
		}
	}

	if len(ports) == 0 {
		return models.ToolResult{
			Success: false,
			Error:   "No valid ports specified",
		}
	}

	var results []string
	var openCount int

	for _, port := range ports {
		addr := fmt.Sprintf("%s:%d", host, port)
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			results = append(results, fmt.Sprintf("Port %d: CLOSED", port))
		} else {
			conn.Close()
			results = append(results, fmt.Sprintf("Port %d: OPEN", port))
			openCount++
		}
	}

	summary := fmt.Sprintf("Scanned %d ports: %d open, %d closed\n\n%s",
		len(ports), openCount, len(ports)-openCount, strings.Join(results, "\n"))

	return models.ToolResult{
		Success: true,
		Output:  summary,
	}
}

// ============================================================================
// HTTP Request Tool
// ============================================================================

// HTTPTool makes HTTP requests and returns response info.
type HTTPTool struct{}

func NewHTTPTool() *HTTPTool { return &HTTPTool{} }

func (h *HTTPTool) Name() string { return "http" }

func (h *HTTPTool) Description() string {
	return "Make an HTTP/HTTPS request and return status code, headers, and response time. Useful for checking web service health."
}

func (h *HTTPTool) Parameters() []Parameter {
	return []Parameter{
		{Name: "url", Type: "string", Description: "URL to request (include http:// or https://)", Required: true},
		{Name: "method", Type: "string", Description: "HTTP method", Required: false, Default: "GET", Enum: []string{"GET", "HEAD", "POST"}},
	}
}

func (h *HTTPTool) Execute(ctx context.Context, params map[string]string) models.ToolResult {
	url := params["url"]
	method := strings.ToUpper(params["method"])

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

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return models.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Invalid request: %v", err),
		}
	}

	req.Header.Set("User-Agent", "cliche/1.0")

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return models.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	var results []string
	results = append(results, fmt.Sprintf("Status: %s", resp.Status))
	results = append(results, fmt.Sprintf("Response Time: %dms", elapsed.Milliseconds()))
	results = append(results, fmt.Sprintf("Protocol: %s", resp.Proto))

	// Key headers
	interestingHeaders := []string{"Content-Type", "Server", "X-Powered-By", "Location", "Cache-Control"}
	for _, h := range interestingHeaders {
		if v := resp.Header.Get(h); v != "" {
			results = append(results, fmt.Sprintf("%s: %s", h, v))
		}
	}

	// Determine success
	success := resp.StatusCode >= 200 && resp.StatusCode < 400

	return models.ToolResult{
		Success: success,
		Output:  strings.Join(results, "\n"),
	}
}

// ============================================================================
// Traceroute Tool
// ============================================================================

// TracerouteTool traces the network path to a host.
type TracerouteTool struct{}

func NewTracerouteTool() *TracerouteTool { return &TracerouteTool{} }

func (t *TracerouteTool) Name() string { return "traceroute" }

func (t *TracerouteTool) Description() string {
	return "Trace the network path to a host. Shows each hop with latency. Useful for finding where connectivity breaks."
}

func (t *TracerouteTool) Parameters() []Parameter {
	return []Parameter{
		{Name: "host", Type: "string", Description: "Hostname or IP to trace", Required: true},
		{Name: "max_hops", Type: "string", Description: "Maximum number of hops", Required: false, Default: "15"},
	}
}

func (t *TracerouteTool) Execute(ctx context.Context, params map[string]string) models.ToolResult {
	host := params["host"]
	maxHops := params["max_hops"]

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "tracert", "-h", maxHops, host)
	} else if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "traceroute", "-m", maxHops, host)
	} else {
		cmd = exec.CommandContext(ctx, "traceroute", "-m", maxHops, "-w", "2", host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Traceroute often exits non-zero even on partial success
		if len(output) > 0 {
			return models.ToolResult{
				Success: true,
				Output:  string(output),
			}
		}
		return models.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Traceroute failed: %v", err),
		}
	}

	return models.ToolResult{
		Success: true,
		Output:  string(output),
	}
}

// ============================================================================
// Network Info Tool
// ============================================================================

// NetInfoTool returns local network interface information.
type NetInfoTool struct{}

func NewNetInfoTool() *NetInfoTool { return &NetInfoTool{} }

func (n *NetInfoTool) Name() string { return "netinfo" }

func (n *NetInfoTool) Description() string {
	return "Get local network interface information including IP addresses, MAC addresses, and interface status."
}

func (n *NetInfoTool) Parameters() []Parameter {
	return []Parameter{
		{Name: "interface", Type: "string", Description: "Specific interface name, or 'all'", Required: false, Default: "all"},
	}
}

func (n *NetInfoTool) Execute(ctx context.Context, params map[string]string) models.ToolResult {
	filterIface := params["interface"]

	ifaces, err := net.Interfaces()
	if err != nil {
		return models.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to get interfaces: %v", err),
		}
	}

	var results []string

	for _, iface := range ifaces {
		if filterIface != "all" && iface.Name != filterIface {
			continue
		}

		// Skip loopback and down interfaces unless specifically requested
		if filterIface == "all" {
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			if iface.Flags&net.FlagUp == 0 {
				continue
			}
		}

		var ifaceInfo []string
		ifaceInfo = append(ifaceInfo, fmt.Sprintf("Interface: %s", iface.Name))
		ifaceInfo = append(ifaceInfo, fmt.Sprintf("  MAC: %s", iface.HardwareAddr))
		ifaceInfo = append(ifaceInfo, fmt.Sprintf("  MTU: %d", iface.MTU))

		status := []string{}
		if iface.Flags&net.FlagUp != 0 {
			status = append(status, "UP")
		}
		if iface.Flags&net.FlagBroadcast != 0 {
			status = append(status, "BROADCAST")
		}
		if iface.Flags&net.FlagMulticast != 0 {
			status = append(status, "MULTICAST")
		}
		ifaceInfo = append(ifaceInfo, fmt.Sprintf("  Flags: %s", strings.Join(status, ", ")))

		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ifaceInfo = append(ifaceInfo, fmt.Sprintf("  Address: %s", addr.String()))
		}

		results = append(results, strings.Join(ifaceInfo, "\n"))
	}

	if len(results) == 0 {
		return models.ToolResult{
			Success: false,
			Error:   "No matching interfaces found",
		}
	}

	return models.ToolResult{
		Success: true,
		Output:  strings.Join(results, "\n\n"),
	}
}

// ============================================================================
// Register All Networking Tools
// ============================================================================

// RegisterNetworkingTools registers all networking tools with the registry.
func RegisterNetworkingTools(r *Registry) {
	r.MustRegister(NewPingTool())
	r.MustRegister(NewDNSLookupTool())
	r.MustRegister(NewPortScanTool())
	r.MustRegister(NewHTTPTool())
	r.MustRegister(NewTracerouteTool())
	r.MustRegister(NewNetInfoTool())
}
