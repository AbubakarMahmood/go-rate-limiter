#!/bin/bash

# Load testing script using Vegeta
# Usage: ./load-test.sh [duration] [rate]

set -e

DURATION=${1:-30s}
RATE=${2:-1000}
TARGET_URL=${TARGET_URL:-http://localhost:8080}
OUTPUT_DIR="./load-test-results"

echo "==================================="
echo "Rate Limiter Load Test"
echo "==================================="
echo "Target URL: $TARGET_URL"
echo "Duration: $DURATION"
echo "Rate: $RATE req/sec"
echo "==================================="

# Create output directory
mkdir -p $OUTPUT_DIR

# Check if vegeta is installed
if ! command -v vegeta &> /dev/null; then
    echo "Error: vegeta is not installed"
    echo "Install with: go install github.com/tsenart/vegeta@latest"
    exit 1
fi

# Generate test targets
cat > $OUTPUT_DIR/targets.txt << EOF
POST $TARGET_URL/v1/check
Content-Type: application/json

{
  "resource": "api.users.create",
  "identifier": "user-123",
  "algorithm": "token_bucket"
}

POST $TARGET_URL/v1/check
Content-Type: application/json

{
  "resource": "api.posts.create",
  "identifier": "user-456",
  "algorithm": "sliding_window"
}

POST $TARGET_URL/v1/check
Content-Type: application/json

{
  "resource": "api.comments.create",
  "identifier": "user-789",
  "algorithm": "fixed_window"
}
EOF

echo "Running load test..."

# Run vegeta attack
vegeta attack \
  -targets=$OUTPUT_DIR/targets.txt \
  -duration=$DURATION \
  -rate=$RATE \
  -workers=10 \
  > $OUTPUT_DIR/results.bin

echo "Load test complete. Generating reports..."

# Generate text report
vegeta report $OUTPUT_DIR/results.bin > $OUTPUT_DIR/report.txt

# Generate HTML plot
vegeta plot $OUTPUT_DIR/results.bin > $OUTPUT_DIR/plot.html

# Generate latency histogram
vegeta report -type='hist[0,1ms,5ms,10ms,50ms,100ms,500ms,1s,5s]' $OUTPUT_DIR/results.bin > $OUTPUT_DIR/histogram.txt

# Display results
echo ""
echo "==================================="
echo "Load Test Results"
echo "==================================="
cat $OUTPUT_DIR/report.txt

echo ""
echo "==================================="
echo "Latency Histogram"
echo "==================================="
cat $OUTPUT_DIR/histogram.txt

echo ""
echo "==================================="
echo "Results saved to: $OUTPUT_DIR"
echo "View HTML plot: open $OUTPUT_DIR/plot.html"
echo "==================================="
