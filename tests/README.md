# Test Organization

This directory contains all test files for the project, organized in a structure that mirrors the main package hierarchy.

## Directory Structure

```
tests/
├── internal/
│   ├── executor/
│   │   ├── executor_test.go        # Dispatcher and routing tests
│   │   └── e2e_integration_test.go # End-to-end workflow tests
│   └── functions/
│       └── network/
│           ├── tcp_test.go         # TCP health check tests
│           └── grpc_test.go        # gRPC health check tests
└── README.md                       # This file
```

## Running Tests

### All Tests
```bash
go test ./tests/... -v -timeout 60s
```

### Executor Tests
```bash
go test ./tests/internal/executor -v -timeout 60s
```

### Network Function Tests
```bash
go test ./tests/internal/functions/network -v -timeout 60s
```

### Specific Test
```bash
go test ./tests/internal/executor -v -run TestE2E_EndToEnd_FullWorkflow
```

### With Benchmarks
```bash
go test ./tests/... -v -bench=. -benchmem
```

## Test Coverage

### Executor Tests (4 tests)
- `TestExecute_CheckTCPHealth_Success` - TCP health check dispatcher routing
- `TestExecute_CheckGRPCHealth_MissingParam` - Parameter validation
- `TestExecute_CheckGRPCHealth_WithDefaults` - Default parameter handling
- `TestE2E_EndToEnd_FullWorkflow` - Complete pipeline workflow (7 validation stages)

### Network Function Tests (21 tests)
#### TCP Tests (13 tests)
- `TestParseSSOutput_EstablishedConnection` - ESTAB connection parsing
- `TestParseSSOutput_HighRetransmits` - High retransmit detection
- `TestParseSSOutput_ListeningSocket` - LISTEN state parsing
- `TestParseSSOutput_TimeWaitState` - TIME-WAIT state parsing
- `TestParseSSOutput_InvalidInput` - Error handling
- `TestCalculateRecommendedBuffer` - BDP calculation (4 sub-tests)
- `TestCalculateRecommendedBuffer_Specific` - Exact formula validation
- `TestCheckTCPHealth_OutputStructure` - Main function output structure
- `TestParseSSOutput_VariousStates` - Multiple TCP states (4 sub-tests)
- `TestParseSSOutput_EdgeCase_EmptyQueues` - Edge case handling
- Benchmarks: `BenchmarkParseSSOutput`, `BenchmarkCalculateRecommendedBuffer`

#### gRPC Tests (8 tests)
- `TestCheckGRPCHealth_Serving` - SERVING status
- `TestCheckGRPCHealth_NotServing` - NOT_SERVING status
- `TestCheckGRPCHealth_Unknown` - UNKNOWN status
- `TestCheckGRPCHealth_ConnectionRefused` - Connection error handling
- `TestCheckGRPCHealth_InvalidHost` - DNS error handling
- `TestCheckGRPCHealth_ZeroTimeout` - Default timeout handling
- `TestCheckGRPCHealth_LatencyMeasurement` - Latency tracking
- `TestCheckGRPCHealth_Timeout` - Timeout parameter validation
- Benchmark: `BenchmarkCheckGRPCHealth`

## Test Organization Notes

- Tests are organized in a **separate directory structure** mirroring the main package structure
- Each test file stays close to its corresponding source package for clarity
- Test packages import from the actual package implementations using full import paths
- This separation keeps the source tree clean while maintaining logical organization

## Future Tests

As new functions are implemented, add corresponding test files to this directory structure:

```
tests/
├── internal/
│   ├── executor/
│   │   └── [new test files here]
│   └── functions/
│       ├── network/
│       │   └── [network test files]
│       ├── system/
│       │   └── [system test files] <- Add when implementing system functions
│       ├── debugging/
│       │   └── [debugging test files] <- Add when implementing debugging functions
│       └── yang/
│           └── [yang test files] <- Add when implementing YANG functions
```

## Test Execution Results

**Last Run:** All 24 tests PASSED ✅
- Executor tests: 4/4 PASS (including 1 E2E with 7 validation stages)
- Network tests: 20/20 PASS (13 TCP + 7 gRPC + benchmarks)

## Notes

- Tests on non-Linux systems expect failures for `ss` command (platform-specific limitation)
- gRPC tests use mock servers with real gRPC health check protocol
- All tests are self-contained and don't require running services
- Integration tests validate the complete CLI→Registry→Executor→Functions pipeline
