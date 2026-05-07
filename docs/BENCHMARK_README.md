# Running Benchmarks

## Linter Issue

Benchmarks may fail to compile due to strict linter settings that block compilation when there are warnings about formatting in logger calls.

## Solutions

### Option 1: Temporarily disable vet for tests

Create a `.golangci.yml` file in the project root:

```yaml
linters-settings:
  govet:
    check-shadowing: false
    enable-all: true
    disable:
      - printf
```

### Option 2: Use a separate package for benchmarks

Create a separate `benchmarks/` directory and move benchmarks there.

### Option 3: Fix logger calls

Fix all logger calls in `server.go`, `tcp.go`, `udp.go`, `tls.go` files to match the format.

### Option 4: Run via workaround

```bash
# Compile tests manually
go test -c ./server -o /tmp/server_test

# Run the compiled test
/tmp/server_test -test.bench=BenchmarkTCPThroughput -test.benchtime=3s
```

## Current Status

Benchmarks are created and ready to use, but require resolving the linter issue to run.

Files:
- `server/benchmark_test.go` - main benchmarks
- `run_benchmarks.sh` - benchmark runner script
- `docs/BENCHMARKS.md` - documentation
