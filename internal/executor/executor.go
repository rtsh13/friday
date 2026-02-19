// Package executor handles function execution and dispatching.
package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/friday/internal/functions/debugging"
	"github.com/friday/internal/functions/network"
	"github.com/friday/internal/functions/system"
	"github.com/friday/internal/types"
	"go.uber.org/zap"
)

// Executor dispatches function calls to their implementations.
type Executor struct {
	logger *zap.Logger
}

// NewExecutor creates a new function executor.
func NewExecutor(logger *zap.Logger) *Executor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Executor{
		logger: logger,
	}
}

// Execute runs a function call and returns the JSON result.
func (e *Executor) Execute(fn types.FunctionCall) (string, error) {
	e.logger.Info("Executing function",
		zap.String("name", fn.Name),
		zap.Any("params", fn.Params))

	switch fn.Name {
	// ==================== Basic Network Tools ====================
	case "ping":
		return e.executePing(fn.Params)

	case "dns_lookup":
		return e.executeDNSLookup(fn.Params)

	case "port_scan":
		return e.executePortScan(fn.Params)

	case "http_request":
		return e.executeHTTPRequest(fn.Params)

	case "traceroute":
		return e.executeTraceroute(fn.Params)

	case "netinfo":
		return e.executeNetInfo(fn.Params)

	// ==================== TCP/gRPC Tools ====================
	case "check_tcp_health":
		return e.executeCheckTCPHealth(fn.Params)

	case "check_grpc_health":
		return e.executeCheckGRPCHealth(fn.Params)

	case "analyze_grpc_stream":
		return e.executeAnalyzeGRPCStream(fn.Params)

	// ==================== System Tools ====================
	case "inspect_network_buffers":
		return e.executeInspectNetworkBuffers(fn.Params)

	case "execute_sysctl_command":
		return e.executeExecuteSysctl(fn.Params)

	case "restore_sysctl_value":
		return e.executeRestoreSysctlValue(fn.Params)

	// ==================== Debugging Tools (Placeholder) ====================
	case "analyze_core_dump":
		return e.executeAnalyzeCoreDump(fn.Params)

	default:
		return "", fmt.Errorf("unknown function: %s", fn.Name)
	}
}

// ============================================================================
// Parameter Helpers
// ============================================================================

func getString(params map[string]interface{}, key string, required bool, defaultVal string) (string, error) {
	v, ok := params[key]
	if !ok {
		if required {
			return "", errors.New("missing required parameter: " + key)
		}
		return defaultVal, nil
	}
	switch t := v.(type) {
	case string:
		return t, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func getInt(params map[string]interface{}, key string, required bool, defaultVal int) (int, error) {
	v, ok := params[key]
	if !ok {
		if required {
			return 0, errors.New("missing required parameter: " + key)
		}
		return defaultVal, nil
	}
	switch t := v.(type) {
	case int:
		return t, nil
	case int64:
		return int(t), nil
	case float64:
		return int(t), nil
	case float32:
		return int(t), nil
	case string:
		i, err := strconv.Atoi(t)
		if err != nil {
			return 0, fmt.Errorf("invalid integer for %s: %v", key, err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unsupported type for int param %s: %T", key, v)
	}
}

func getBool(params map[string]interface{}, key string, required bool, defaultVal bool) (bool, error) {
	v, ok := params[key]
	if !ok {
		if required {
			return false, errors.New("missing required parameter: " + key)
		}
		return defaultVal, nil
	}
	switch t := v.(type) {
	case bool:
		return t, nil
	case string:
		return strings.ToLower(t) == "true" || t == "1", nil
	default:
		return false, fmt.Errorf("unsupported type for bool param %s: %T", key, v)
	}
}

// ============================================================================
// Basic Network Tool Implementations
// ============================================================================

func (e *Executor) executePing(params map[string]interface{}) (string, error) {
	host, err := getString(params, "host", true, "")
	if err != nil {
		return "", err
	}
	count, err := getInt(params, "count", false, 3)
	if err != nil {
		return "", err
	}

	result, err := network.Ping(host, count)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

func (e *Executor) executeDNSLookup(params map[string]interface{}) (string, error) {
	domain, err := getString(params, "domain", true, "")
	if err != nil {
		return "", err
	}
	recordType, err := getString(params, "record_type", false, "all")
	if err != nil {
		return "", err
	}

	result, err := network.DNSLookup(domain, recordType)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

func (e *Executor) executePortScan(params map[string]interface{}) (string, error) {
	host, err := getString(params, "host", true, "")
	if err != nil {
		return "", err
	}
	ports, err := getString(params, "ports", false, "common")
	if err != nil {
		return "", err
	}

	result, err := network.PortScan(host, ports)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

func (e *Executor) executeHTTPRequest(params map[string]interface{}) (string, error) {
	url, err := getString(params, "url", true, "")
	if err != nil {
		return "", err
	}
	method, err := getString(params, "method", false, "GET")
	if err != nil {
		return "", err
	}

	result, err := network.HTTPRequest(url, method)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

func (e *Executor) executeTraceroute(params map[string]interface{}) (string, error) {
	host, err := getString(params, "host", true, "")
	if err != nil {
		return "", err
	}
	maxHops, err := getInt(params, "max_hops", false, 15)
	if err != nil {
		return "", err
	}

	result, err := network.Traceroute(host, maxHops)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

func (e *Executor) executeNetInfo(params map[string]interface{}) (string, error) {
	iface, err := getString(params, "interface", false, "all")
	if err != nil {
		return "", err
	}

	result, err := network.NetInfo(iface)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

// ============================================================================
// TCP/gRPC Tool Implementations
// ============================================================================

func (e *Executor) executeCheckTCPHealth(params map[string]interface{}) (string, error) {
	iface, err := getString(params, "interface", true, "")
	if err != nil {
		return "", err
	}
	port, err := getInt(params, "port", true, 0)
	if err != nil {
		return "", err
	}

	result, err := network.CheckTCPHealth(iface, port)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

func (e *Executor) executeCheckGRPCHealth(params map[string]interface{}) (string, error) {
	host, err := getString(params, "host", false, "localhost")
	if err != nil {
		return "", err
	}
	port, err := getInt(params, "port", true, 0)
	if err != nil {
		return "", err
	}
	timeout, err := getInt(params, "timeout", false, 5)
	if err != nil {
		return "", err
	}

	result, err := network.CheckGRPCHealth(host, port, timeout)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

func (e *Executor) executeAnalyzeGRPCStream(params map[string]interface{}) (string, error) {
	host, err := getString(params, "host", false, "localhost")
	if err != nil {
		return "", err
	}
	port, err := getInt(params, "port", true, 0)
	if err != nil {
		return "", err
	}
	duration, err := getInt(params, "duration", false, 10)
	if err != nil {
		return "", err
	}

	result, err := network.AnalyzeGRPCStream(host, port, duration)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

// ============================================================================
// System Tool Implementations
// ============================================================================

func (e *Executor) executeInspectNetworkBuffers(params map[string]interface{}) (string, error) {
	result, err := system.InspectNetworkBuffers()
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

func (e *Executor) executeExecuteSysctl(params map[string]interface{}) (string, error) {
	parameter, err := getString(params, "parameter", true, "")
	if err != nil {
		return "", err
	}
	value, err := getString(params, "value", true, "")
	if err != nil {
		return "", err
	}
	persist, err := getBool(params, "persist", false, false)
	if err != nil {
		return "", err
	}

	// Bug 1 fix: dry-run validates inputs only; does not execute the real sysctl command.
	isDryRun, _ := getBool(params, "__dry_run", false, false)
	if isDryRun {
		return toJSON(map[string]interface{}{
			"parameter": parameter,
			"value":     value,
			"persist":   persist,
			"dry_run":   true,
			"success":   true,
		})
	}

	result, err := system.ExecuteSysctl(parameter, value, persist)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

// executeRestoreSysctlValue restores a sysctl parameter to a previous value.
// Used internally by the transaction rollback mechanism.
func (e *Executor) executeRestoreSysctlValue(params map[string]interface{}) (string, error) {
	parameter, err := getString(params, "parameter", true, "")
	if err != nil {
		return "", err
	}
	value, err := getString(params, "value", true, "")
	if err != nil {
		return "", err
	}

	if err := system.RestoreSysctlValue(parameter, value); err != nil {
		return "", err
	}

	return toJSON(map[string]interface{}{
		"parameter":      parameter,
		"restored_value": value,
		"success":        true,
	})
}

// ============================================================================
// Debugging Tool Implementations (Placeholder)
// ============================================================================

func (e *Executor) executeAnalyzeCoreDump(params map[string]interface{}) (string, error) {
	corePath, err := getString(params, "core_path", true, "")
	if err != nil {
		return "", err
	}
	binaryPath, err := getString(params, "binary_path", false, "")
	if err != nil {
		return "", err
	}

	// Import from debugging package
	result, err := debugging.AnalyzeCoreDump(corePath, binaryPath)
	if err != nil {
		return "", err
	}

	return toJSON(result)
}

// ============================================================================
// Utilities
// ============================================================================

func toJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(b), nil
}
