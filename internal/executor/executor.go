package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/ashutoshrp06/telemetry-debugger/internal/functions/network"
	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	"go.uber.org/zap"
)

type Executor struct {
	logger *zap.Logger
}

func NewExecutor(logger *zap.Logger) *Executor {
	return &Executor{
		logger: logger,
	}
}

func (e *Executor) Execute(fn types.FunctionCall) (string, error) {
	e.logger.Info("Executing function",
		zap.String("name", fn.Name),
		zap.Any("params", fn.Params))

	// helper to get string parameter
	getString := func(key string, required bool, def string) (string, error) {
		v, ok := fn.Params[key]
		if !ok {
			if required {
				return "", errors.New("missing required parameter: " + key)
			}
			return def, nil
		}
		switch t := v.(type) {
		case string:
			return t, nil
		default:
			// attempt to stringify
			return fmt.Sprintf("%v", v), nil
		}
	}

	// helper to get int parameter
	getInt := func(key string, required bool, def int) (int, error) {
		v, ok := fn.Params[key]
		if !ok {
			if required {
				return 0, errors.New("missing required parameter: " + key)
			}
			return def, nil
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

	switch fn.Name {
	case "check_tcp_health":
		iface, err := getString("interface", true, "")
		if err != nil {
			return "", err
		}
		port, err := getInt("port", true, 0)
		if err != nil {
			return "", err
		}

		// Call the real implementation
		stats, err := network.CheckTCPHealth(iface, port)
		if err != nil {
			return "", err
		}

		b, err := json.Marshal(stats)
		if err != nil {
			return "", err
		}
		return string(b), nil

	case "check_grpc_health":
		host, err := getString("host", false, "localhost")
		if err != nil {
			return "", err
		}
		port, err := getInt("port", true, 0)
		if err != nil {
			return "", err
		}
		timeout, err := getInt("timeout", false, 5)
		if err != nil {
			return "", err
		}

		// Call the real implementation
		stats, err := network.CheckGRPCHealth(host, port, timeout)
		if err != nil {
			return "", err
		}

		b, err := json.Marshal(stats)
		if err != nil {
			return "", err
		}
		return string(b), nil

	default:
		return "", fmt.Errorf("unknown function: %s", fn.Name)
	}
}