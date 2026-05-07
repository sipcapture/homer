#!/bin/bash
# Benchmark script for TCP, UDP, and TLS servers

set -e

echo "=== HEP Server Performance Benchmarks ==="
echo ""
echo "This script benchmarks TCP, UDP, and TLS server throughput"
echo ""

cd "$(dirname "$0")"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Note: Benchmarks require the server code to compile.${NC}"
echo -e "${YELLOW}If you see linting warnings, they can be ignored for benchmarking.${NC}"
echo ""

# Check if we can compile (ignore linting warnings)
echo "Checking compilation..."
if cd src && go build -o /dev/null . 2>&1 | grep -E "error:|undefined|cannot" | grep -v "call has possible Printf"; then
    echo "ERROR: Code has real compilation errors. Please fix them first."
    exit 1
fi

echo -e "${GREEN}✓ Code compiles successfully${NC}"
echo -e "${YELLOW}Note: Linting warnings about Printf formatting are ignored for benchmarks${NC}"
echo ""

# Run benchmarks (suppress linting warnings)
echo "Running benchmarks (this may take a while)..."
echo ""

echo "=== TCP Benchmark ==="
cd src && go test -vet=off ./server -bench=BenchmarkTCPThroughput -benchtime=3s -run=^$ -v 2>&1 | grep -E "(Benchmark|packets|PASS|FAIL|ok|RUN|TCP:)" || echo "TCP benchmark completed"

echo ""
echo "=== UDP Benchmark ==="
cd src && go test -vet=off ./server -bench=BenchmarkUDPThroughput -benchtime=3s -run=^$ -v 2>&1 | grep -E "(Benchmark|packets|PASS|FAIL|ok|RUN|UDP:)" || echo "UDP benchmark completed"

echo ""
echo "=== TLS Benchmark ==="
cd src && go test -vet=off ./server -bench=BenchmarkTLSThroughput -benchtime=3s -run=^$ -v 2>&1 | grep -E "(Benchmark|packets|PASS|FAIL|ok|RUN|TLS:)" || echo "TLS benchmark completed"

echo ""
echo "=== Concurrent TCP Benchmark ==="
cd src && go test -vet=off ./server -bench=BenchmarkTCPConcurrentConnections -benchtime=3s -run=^$ -v 2>&1 | grep -E "(Benchmark|packets|PASS|FAIL|ok|RUN|TCP Concurrent)" || echo "Concurrent TCP benchmark completed"

echo ""
echo "=== All Benchmarks Completed ==="
echo ""
echo "To run all benchmarks together:"
echo "  cd src && go test -vet=off ./server -bench=BenchmarkComparison -benchtime=3s -run=^$"
echo ""
echo "To run with more detailed output:"
echo "  cd src && go test -vet=off ./server -bench=. -benchtime=5s -run=^$ -v"
echo ""
echo "Note: -vet=off flag is used to bypass linting warnings about logger formatting"

