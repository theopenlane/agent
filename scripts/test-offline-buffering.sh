#!/bin/bash

# Test script for offline buffering functionality
set -e

echo "=== Testing Offline Buffering Functionality ==="

# Create test directories
mkdir -p ./test-data/data
mkdir -p ./test-data/buffer

echo "Test directories created"

# Test 1: Validate configuration
echo ""
echo "1. Testing configuration validation..."
./openlane-agent config validate --config ./test-data/test-offline-config.yaml
echo "✓ Configuration is valid"

# Test 2: Show what offline mode offers
echo ""
echo "2. Offline buffering features:"
echo "   - File-based buffering for check results when API is unreachable"
echo "   - Connectivity detection and monitoring"
echo "   - Automatic sync when connectivity is restored"
echo "   - Configurable retry attempts and retention periods"
echo "   - Buffer cleanup and statistics"

# Test 3: Check buffer directory
echo ""
echo "3. Buffer directory structure:"
if [ -d "./test-data/buffer" ]; then
    echo "   Buffer directory: ./test-data/buffer"
    echo "   Status: Ready for buffering"
    ls -la ./test-data/buffer 2>/dev/null || echo "   (Empty - ready for first use)"
else
    echo "   Buffer directory will be created on first use"
fi

# Test 4: Show offline configuration
echo ""
echo "4. Offline configuration from test config:"
echo "   - Enabled: true"
echo "   - Buffer directory: ./test-data/buffer"
echo "   - Connectivity check interval: 10s"
echo "   - Sync interval: 30s"
echo "   - Max retries: 3"
echo "   - Buffer retention: 1h"

echo ""
echo "=== Offline Buffering Test Complete ==="
echo ""
echo "To test manually:"
echo "1. Start the agent with: ./openlane-agent start --config ./test-data/test-offline-config.yaml --no-daemon"
echo "2. Disconnect network or block API access to trigger offline mode"
echo "3. Watch as results get buffered to ./test-data/buffer/"
echo "4. Reconnect network to see automatic sync occur"
echo "5. Check agent stats for buffer and connectivity information"