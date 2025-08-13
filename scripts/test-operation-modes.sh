#!/bin/bash

# Test script for all operation modes
set -e

echo "=== Testing Operation Modes ==="

# Clean up any previous test data
rm -rf ./test-data/data
rm -rf ./test-data/results
rm -rf ./test-data/buffer

echo ""
echo "1. Testing STANDALONE mode (completely independent)"
echo "================================================="

# Create test directories
mkdir -p ./test-data/data ./test-data/results ./test-data/buffer

# Test configuration validation
echo "   Validating standalone configuration..."
./openlane-agent config validate --config ./test-data/standalone-config.yaml
echo "   ✓ Standalone configuration is valid"

echo "   Key features of standalone mode:"
echo "   - No API registration or connectivity required"
echo "   - Results stored locally in ./test-data/results/"
echo "   - Evidence files copied to ./test-data/results/evidence/"
echo "   - Configurable output formats: json, yaml, csv"
echo "   - Perfect for air-gapped environments"

echo ""
echo "2. Testing BUFFERED mode (API with local fallback)"
echo "=================================================="

# Test configuration validation
echo "   Validating buffered configuration..."
./openlane-agent config validate --config ./test-data/buffered-config.yaml
echo "   ✓ Buffered configuration is valid"

echo "   Key features of buffered mode:"
echo "   - Primary: API connectivity to Openlane platform"
echo "   - Fallback: Local buffering when API unavailable"
echo "   - Auto-sync when connectivity restored"
echo "   - Configurable retry limits and retention"
echo "   - Buffer directory: ./test-data/buffer/"

echo ""
echo "3. Testing NORMAL mode (standard API operation)"
echo "==============================================="

# Test configuration validation
echo "   Validating normal configuration..."
./openlane-agent config validate --config ./test-data/normal-config.yaml
echo "   ✓ Normal configuration is valid"

echo "   Key features of normal mode:"
echo "   - Standard API connectivity to Openlane platform"
echo "   - Real-time result transmission"
echo "   - No local buffering or fallback"
echo "   - Requires consistent network connectivity"
echo "   - Most efficient for cloud environments"

echo ""
echo "4. Storage Interface Architecture"
echo "================================="

echo "   The agent now uses pluggable storage interfaces:"
echo "   - ResultStorage: Handles compliance check results"
echo "   - EvidenceStorage: Handles evidence file storage"
echo "   - StorageManager: Coordinates both storage types"
echo ""
echo "   Storage implementations by mode:"
echo "   - Standalone: LocalFileStorage + LocalEvidenceStorage"
echo "   - Buffered: BufferedAPIStorage (API + local buffer)"
echo "   - Normal: APIResultStorage + APIEvidenceStorage"

echo ""
echo "5. Example Usage"
echo "==============="

echo "   To run in standalone mode:"
echo "   ./openlane-agent start --config ./test-data/standalone-config.yaml --no-daemon"
echo ""
echo "   To run in buffered mode:"
echo "   ./openlane-agent start --config ./test-data/buffered-config.yaml --no-daemon"
echo ""
echo "   To run in normal mode:"
echo "   ./openlane-agent start --config ./test-data/normal-config.yaml --no-daemon"

echo ""
echo "=== Operation Modes Test Complete ==="
echo ""
echo "All three operation modes are now available:"
echo "1. Standalone - Complete independence, local storage only"
echo "2. Buffered - API with intelligent local fallback"
echo "3. Normal - Standard API operation"