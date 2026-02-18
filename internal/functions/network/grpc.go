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

// CheckGRPCHealth connects to a gRPC server and checks its health status.
//
// Bug 7 fix: replaced deprecated grpc.DialContext (with grpc.WithBlock) with
// grpc.NewClient. Connections are now established lazily; any connectivity
// error surfaces at the RPC call level instead of the dial step.
func CheckGRPCHealth(host string, port int, timeout int) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 5
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	startTime := time.Now()

	target := fmt.Sprintf("%s:%d", host, port)
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", target, err)
	}
	defer conn.Close()

	client := grpc_health_v1.NewHealthClient(conn)

	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "", // empty service name checks overall server health
	})
	if err != nil {
		return nil, fmt.Errorf("gRPC health check RPC failed: %w", err)
	}

	latencyMs := time.Since(startTime).Milliseconds()

	statusStr := resp.Status.String()
	switch resp.Status {
	case grpc_health_v1.HealthCheckResponse_SERVING:
		statusStr = "SERVING"
	case grpc_health_v1.HealthCheckResponse_NOT_SERVING:
		statusStr = "NOT_SERVING"
	case grpc_health_v1.HealthCheckResponse_UNKNOWN:
		statusStr = "UNKNOWN"
	case grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN:
		statusStr = "SERVICE_UNKNOWN"
	}

	return map[string]interface{}{
		"host":       host,
		"port":       port,
		"status":     statusStr,
		"latency_ms": latencyMs,
	}, nil
}

// AnalyzeGRPCStream monitors a gRPC health-watch stream for the specified
// duration and returns message-level statistics.
//
// Bug 4 fix: sequence tracking was split across the goroutine (incrementing
// its own counter and writing to stats.SequenceNumbers) and the main loop
// (incrementing lastSeq independently), causing the two to diverge when
// messages were buffered in msgChan after the stop signal. All sequence
// tracking now lives solely in the main select loop, eliminating the race.
//
// Bug 6 fix: stream.CloseSend() is a no-op on a server-streaming RPC (Watch)
// because the client does not send messages; the server controls the stream.
// The stream is now terminated by cancelling the context, which causes
// stream.Recv() to return a context-cancelled error that the goroutine
// treats as a clean exit.
//
// Bug 7 fix: uses grpc.NewClient instead of deprecated grpc.DialContext.
func AnalyzeGRPCStream(host string, port int, duration int) (map[string]interface{}, error) {
	if duration <= 0 {
		duration = 10
	}

	target := fmt.Sprintf("%s:%d", host, port)

	// The context lifetime covers the monitoring window plus a small buffer.
	// We hold a reference to cancel so we can stop the stream (Bug 6 fix).
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(duration+5)*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", target, err)
	}
	defer conn.Close()

	client := grpc_health_v1.NewHealthClient(conn)

	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "", // empty service name watches overall server health
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start gRPC health watch stream: %w", err)
	}

	// Bug 5 fix: Host and Port are stored in StreamStats so ToMap() can
	// return the actual values instead of hardcoded empty string / zero.
	stats := &StreamStats{
		Host:              host,
		Port:              port,
		StartTime:         time.Now(),
		EndTime:           time.Now(),
		MessagesSent:      1, // the initial Watch request counts as one sent message
		MessagesReceived:  0,
		SequenceNumbers:   make(map[int64]bool),
		DroppedSequences:  []int64{},
		FlowControlEvents: 0,
		Errors:            []string{},
	}

	stopChan := make(chan struct{})
	time.AfterFunc(time.Duration(duration)*time.Second, func() {
		close(stopChan)
	})

	msgChan := make(chan *grpc_health_v1.HealthCheckResponse, 100)
	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	// Goroutine: read from the stream and forward messages to the main loop.
	// Bug 4 fix: the goroutine no longer maintains its own sequence counter
	// or writes to stats.SequenceNumbers. It only forwards raw messages.
	// Bug 6 fix: a ctx.Err() check distinguishes an intentional cancel (clean
	// stop) from a real network error so we don't report a spurious error.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				if ctx.Err() != nil {
					// Context was cancelled by us to stop the stream — not an error.
					return
				}
				errChan <- err
				return
			}
			msgChan <- resp
		}
	}()

	// Main monitoring loop.
	// Bug 4 fix: lastSeq is the single source of truth for sequence numbers.
	// Each received message increments lastSeq and records that sequence in
	// the map here, so the map and lastSeq are always in sync.
	lastSeq := int64(0)
	receiveCount := 0

	for {
		select {
		case <-stopChan:
			stats.EndTime = time.Now()

			// Bug 6 fix: cancel the context to signal stream.Recv() to return,
			// which unblocks the goroutine cleanly. CloseSend() is removed.
			cancel()
			wg.Wait()

			// Detect gaps in the sequence space.
			if lastSeq > 0 {
				for i := int64(1); i <= lastSeq; i++ {
					if !stats.SequenceNumbers[i] {
						stats.DroppedSequences = append(stats.DroppedSequences, i)
					}
				}
			}

			stats.MessagesReceived = receiveCount
			if stats.MessagesSent > 0 {
				stats.DropPercentage = float64(len(stats.DroppedSequences)) * 100.0 / float64(stats.MessagesSent)
			}
			stats.MonitoringDuration = stats.EndTime.Sub(stats.StartTime).Seconds()

			return stats.ToMap(), nil

		case resp := <-msgChan:
			receiveCount++
			lastSeq++
			// Bug 4 fix: sequence number recorded here, in the same select case
			// that increments lastSeq, so they are always equal.
			stats.SequenceNumbers[lastSeq] = true

			if stats.LastStatus != "" && stats.LastStatus != resp.Status.String() {
				stats.FlowControlEvents++
			}
			stats.LastStatus = resp.Status.String()

		case err := <-errChan:
			stats.Errors = append(stats.Errors, err.Error())
			stats.EndTime = time.Now()

			stats.MessagesReceived = receiveCount
			if stats.MessagesSent > 0 {
				stats.DropPercentage = float64(len(stats.DroppedSequences)) * 100.0 / float64(stats.MessagesSent)
			}
			stats.MonitoringDuration = stats.EndTime.Sub(stats.StartTime).Seconds()

			return stats.ToMap(), nil
		}
	}
}

// StreamStats holds statistics about a monitored gRPC stream.
type StreamStats struct {
	// Bug 5 fix: Host and Port added so ToMap() can return the actual values.
	Host               string
	Port               int
	StartTime          time.Time
	EndTime            time.Time
	MessagesSent       int
	MessagesReceived   int
	DroppedCount       int
	DropPercentage     float64
	SequenceNumbers    map[int64]bool
	DroppedSequences   []int64
	FlowControlEvents  int
	LastStatus         string
	MonitoringDuration float64
	Errors             []string
}

// ToMap converts StreamStats to a map for JSON serialization.
func (s *StreamStats) ToMap() map[string]interface{} {
	// Bug 5 fix: use s.Host and s.Port instead of hardcoded "" and 0.
	result := map[string]interface{}{
		"host":                    s.Host,
		"port":                    s.Port,
		"messages_sent":           s.MessagesSent,
		"messages_received":       s.MessagesReceived,
		"dropped_count":           len(s.DroppedSequences),
		"drop_percentage":         fmt.Sprintf("%.2f", s.DropPercentage),
		"flow_control_events":     s.FlowControlEvents,
		"monitoring_duration_sec": fmt.Sprintf("%.2f", s.MonitoringDuration),
		"status":                  "ok",
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
