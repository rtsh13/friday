package network

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// CheckGRPCHealth connects to a gRPC server and checks its health status
func CheckGRPCHealth(host string, port int, timeout int) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 5 // default 5 seconds
	}

	// Create connection context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Measure total latency
	startTime := time.Now()

	// Connect to gRPC server (insecure for now - can be enhanced)
	target := fmt.Sprintf("%s:%d", host, port)
	conn, err := grpc.DialContext(ctx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // wait for connection to be ready
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", target, err)
	}
	defer conn.Close()

	// Create health check client
	client := grpc_health_v1.NewHealthClient(conn)

	// Call health check RPC with service name empty (default service)
	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "", // empty service name checks overall server health
	})
	if err != nil {
		return nil, fmt.Errorf("gRPC health check RPC failed: %w", err)
	}

	// Calculate latency
	latencyMs := time.Since(startTime).Milliseconds()

	// Map gRPC health status to string
	statusStr := resp.Status.String()
	if resp.Status == grpc_health_v1.HealthCheckResponse_SERVING {
		statusStr = "SERVING"
	} else if resp.Status == grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		statusStr = "NOT_SERVING"
	} else if resp.Status == grpc_health_v1.HealthCheckResponse_UNKNOWN {
		statusStr = "UNKNOWN"
	} else if resp.Status == grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN {
		statusStr = "SERVICE_UNKNOWN"
	}

	return map[string]interface{}{
		"host":       host,
		"port":       port,
		"status":     statusStr,
		"latency_ms": latencyMs,
	}, nil
}

func AnalyzeGRPCStream(host string, port int, duration int) (map[string]interface{}, error) {
	if duration <= 0 {
		duration = 10 // default 10 seconds
	}

	target := fmt.Sprintf("%s:%d", host, port)

	// Create connection context with timeout (slightly longer than monitoring duration)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(duration+5)*time.Second)
	defer cancel()

	// Connect to gRPC server
	conn, err := grpc.DialContext(ctx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", target, err)
	}
	defer conn.Close()

	// Create health check client
	client := grpc_health_v1.NewHealthClient(conn)

	// Watch for health check stream changes
	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "", // empty service name watches overall server health
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start gRPC health watch stream: %w", err)
	}

	// Monitor the stream
	stats := &StreamStats{
		StartTime:        time.Now(),
		EndTime:          time.Now(),
		MessagesSent:     1,  // At least one watch request
		MessagesReceived: 0,
		SequenceNumbers:  make(map[int64]bool),
		DroppedSequences: []int64{},
		FlowControlEvents: 0,
		Errors:           []string{},
	}

	// Channel to stop monitoring after duration
	stopChan := make(chan bool, 1)
	time.AfterFunc(time.Duration(duration)*time.Second, func() {
		stopChan <- true
	})

	// Channel to collect stream messages
	msgChan := make(chan *grpc_health_v1.HealthCheckResponse, 100)
	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	// Goroutine to receive stream messages
	wg.Add(1)
	go func() {
		defer wg.Done()
		sequence := int64(0)
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				errChan <- err
				return
			}

			sequence++
			msgChan <- resp
			stats.SequenceNumbers[sequence] = true
		}
	}()

	// Main monitoring loop
	lastSeq := int64(0)
	receiveCount := 0

	for {
		select {
		case <-stopChan:
			// Stop monitoring after duration
			stats.EndTime = time.Now()
			stream.CloseSend()
			wg.Wait()

			// Detect dropped sequences
			if lastSeq > 0 {
				for i := int64(1); i <= lastSeq; i++ {
					if !stats.SequenceNumbers[i] {
						stats.DroppedSequences = append(stats.DroppedSequences, i)
					}
				}
			}

			// Calculate statistics
			stats.MessagesReceived = receiveCount
			if stats.MessagesSent > 0 {
				stats.DropPercentage = float64(len(stats.DroppedSequences)) * 100.0 / float64(stats.MessagesSent)
			}
			stats.MonitoringDuration = stats.EndTime.Sub(stats.StartTime).Seconds()

			return stats.ToMap(), nil

		case resp := <-msgChan:
			receiveCount++
			lastSeq++

			// Detect flow control events (when status changes)
			if stats.LastStatus != "" && stats.LastStatus != resp.Status.String() {
				stats.FlowControlEvents++
			}
			stats.LastStatus = resp.Status.String()

		case err := <-errChan:
			stats.Errors = append(stats.Errors, err.Error())
			stats.EndTime = time.Now()

			// Calculate statistics with partial data
			stats.MessagesReceived = receiveCount
			if stats.MessagesSent > 0 {
				stats.DropPercentage = float64(len(stats.DroppedSequences)) * 100.0 / float64(stats.MessagesSent)
			}
			stats.MonitoringDuration = stats.EndTime.Sub(stats.StartTime).Seconds()

			return stats.ToMap(), nil
		}
	}
}

// StreamStats holds statistics about a monitored gRPC stream
type StreamStats struct {
	StartTime         time.Time
	EndTime           time.Time
	MessagesSent      int
	MessagesReceived  int
	DroppedCount      int
	DropPercentage    float64
	SequenceNumbers   map[int64]bool
	DroppedSequences  []int64
	FlowControlEvents int
	LastStatus        string
	MonitoringDuration float64
	Errors            []string
}

// ToMap converts StreamStats to a map for JSON serialization
func (s *StreamStats) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"host":                   "",
		"port":                   0,
		"messages_sent":          s.MessagesSent,
		"messages_received":      s.MessagesReceived,
		"dropped_count":          len(s.DroppedSequences),
		"drop_percentage":        fmt.Sprintf("%.2f", s.DropPercentage),
		"flow_control_events":    s.FlowControlEvents,
		"monitoring_duration_sec": fmt.Sprintf("%.2f", s.MonitoringDuration),
		"status":                 "ok",
	}

	if len(s.Errors) > 0 {
		result["status"] = "error"
		result["errors"] = s.Errors
	}

	if s.DropPercentage > 1.0 {
		result["status"] = "warning"
	}

	return result
}