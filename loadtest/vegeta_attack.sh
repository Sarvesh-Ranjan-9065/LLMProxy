#!/bin/bash

# LLMProxy Load Test Script using vegeta
# Install: go install github.com/tsenart/vegeta@latest

set -e

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
DURATION="${DURATION:-30s}"
RATE="${RATE:-100}"

echo "╔══════════════════════════════════════════╗"
echo "║        LLMProxy Load Test                ║"
echo "╚══════════════════════════════════════════╝"
echo ""
echo "Target:   $PROXY_URL"
echo "Rate:     $RATE req/sec"
echo "Duration: $DURATION"
echo ""

# Check if vegeta is installed
if ! command -v vegeta &> /dev/null; then
    echo "vegeta not found. Install with: go install github.com/tsenart/vegeta@latest"
    echo ""
    echo "Falling back to curl-based load test..."
    echo ""
    
    # Simple curl-based load test
    TOTAL=100
    SUCCESS=0
    FAIL=0
    RATE_LIMITED=0
    
    echo "Sending $TOTAL requests..."
    START=$(date +%s%N)
    
    for i in $(seq 1 $TOTAL); do
        STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
            -X POST "$PROXY_URL/v1/chat/completions" \
            -H "Content-Type: application/json" \
            -H "X-API-Key: test-key" \
            -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Load test request '$i'"}]}')
        
        if [ "$STATUS" = "200" ]; then
            SUCCESS=$((SUCCESS + 1))
        elif [ "$STATUS" = "429" ]; then
            RATE_LIMITED=$((RATE_LIMITED + 1))
        else
            FAIL=$((FAIL + 1))
        fi
        
        if [ $((i % 10)) -eq 0 ]; then
            echo "  Progress: $i/$TOTAL (200: $SUCCESS, 429: $RATE_LIMITED, errors: $FAIL)"
        fi
    done
    
    END=$(date +%s%N)
    ELAPSED=$(( (END - START) / 1000000 ))
    
    echo ""
    echo "═══════════════════════════════════════"
    echo "Results:"
    echo "  Total:        $TOTAL"
    echo "  Success:      $SUCCESS"
    echo "  Rate Limited: $RATE_LIMITED"
    echo "  Errors:       $FAIL"
    echo "  Time:         ${ELAPSED}ms"
    echo "  Throughput:   $(echo "scale=2; $TOTAL * 1000 / $ELAPSED" | bc) req/sec"
    echo "═══════════════════════════════════════"
    exit 0
fi

# Create temporary target file
BODY='{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello, how are you?"}]}'
cat <<EOF > /tmp/vegeta-target.txt
POST ${PROXY_URL}/v1/chat/completions
Content-Type: application/json
X-API-Key: test-key

@/tmp/vegeta-body.json
EOF

echo "$BODY" > /tmp/vegeta-body.json

echo "Starting load test..."
echo ""

# Run vegeta attack
vegeta attack \
    -targets=/tmp/vegeta-target.txt \
    -rate=$RATE \
    -duration=$DURATION \
    -timeout=30s \
    | tee /tmp/vegeta-results.bin \
    | vegeta report

echo ""
echo "Generating HTML report..."
cat /tmp/vegeta-results.bin | vegeta report -type=json > loadtest/report.json 2>/dev/null || true
cat /tmp/vegeta-results.bin | vegeta plot > loadtest/report.html 2>/dev/null || true
echo "Report saved to loadtest/report.html"

# Cleanup
rm -f /tmp/vegeta-target.txt /tmp/vegeta-body.json /tmp/vegeta-results.bin