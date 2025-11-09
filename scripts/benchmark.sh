#!/bin/bash

# Benchmark script for comparing algorithms
# Usage: ./benchmark.sh

set -e

echo "==================================="
echo "Rate Limiter Benchmark Suite"
echo "==================================="

# Run Go benchmarks
echo "Running Go benchmarks..."
go test -bench=. -benchmem -benchtime=10s ./tests/benchmark/ | tee benchmark-results.txt

echo ""
echo "==================================="
echo "Benchmark Complete"
echo "==================================="

# Extract key metrics
echo "Key Metrics:"
echo "-------------"
grep "BenchmarkTokenBucket" benchmark-results.txt | head -1
grep "BenchmarkSlidingWindow" benchmark-results.txt | head -1
grep "BenchmarkFixedWindow" benchmark-results.txt | head -1

echo ""
echo "Results saved to: benchmark-results.txt"
