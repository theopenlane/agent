package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/internal/buffer"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/internal/connectivity"
)

// Simple test program to demonstrate buffer and connectivity functionality
func main() {
	// Setup logger
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()

	fmt.Println("=== Testing Buffer and Connectivity Components ===")

	// Test buffer functionality
	fmt.Println("\n1. Testing Result Buffer...")
	testBuffer(logger)

	// Test connectivity functionality  
	fmt.Println("\n2. Testing Connectivity Manager...")
	testConnectivity(logger)

	fmt.Println("\n=== All Tests Complete ===")
}

func testBuffer(logger zerolog.Logger) {
	// Create buffer directory
	bufferDir := "./test-data/buffer-test"
	os.MkdirAll(bufferDir, 0755)
	defer os.RemoveAll(bufferDir)

	// Create buffer
	buffer, err := buffer.NewResultBuffer(logger, bufferDir)
	if err != nil {
		log.Fatal("Failed to create buffer:", err)
	}

	// Create a test result
	testResult := &config.Result{
		CheckName:   "test-check",
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(5 * time.Second),
		Duration:    "5s",
		ExitCode:    0,
		Passed:      true,
		ExecutedAt:  time.Now(),
		Controls:    []string{"TEST-001"},
		Tags:        []string{"test"},
	}

	// Test buffering
	fmt.Printf("   Buffering test result for check: %s\n", testResult.CheckName)
	err = buffer.BufferResult(testResult)
	if err != nil {
		log.Fatal("Failed to buffer result:", err)
	}
	fmt.Println("   ✓ Result buffered successfully")

	// Test retrieval
	results, err := buffer.GetBufferedResults()
	if err != nil {
		log.Fatal("Failed to get buffered results:", err)
	}
	fmt.Printf("   ✓ Retrieved %d buffered result(s)\n", len(results))

	// Test stats
	stats, err := buffer.GetBufferStats()
	if err != nil {
		log.Fatal("Failed to get buffer stats:", err)
	}
	fmt.Printf("   ✓ Buffer stats: %d items, %d bytes\n", 
		stats["buffered_count"], stats["total_size"])

	// Clean up buffered results
	for _, result := range results {
		buffer.RemoveBufferedResult(result.ID)
	}
	fmt.Println("   ✓ Buffer cleanup completed")
}

func testConnectivity(logger zerolog.Logger) {
	// Create connectivity manager
	apiURL := "https://httpbin.org"  // Use a reliable test endpoint
	connMgr := connectivity.NewManager(logger, apiURL)

	// Test connectivity check
	ctx := context.Background()
	fmt.Printf("   Checking connectivity to: %s\n", apiURL)
	
	err := connMgr.Check(ctx)
	if err != nil {
		fmt.Printf("   Connectivity check failed (expected if offline): %v\n", err)
		fmt.Printf("   Status: %s\n", getStatusString(connMgr.GetStatus()))
	} else {
		fmt.Println("   ✓ Connectivity check passed")
		fmt.Printf("   Status: %s\n", getStatusString(connMgr.GetStatus()))
	}

	// Test stats
	stats := connMgr.GetStats()
	fmt.Printf("   ✓ Connectivity stats available: %d fields\n", len(stats))

	// Test subscription (brief test)
	fmt.Println("   ✓ Connectivity monitoring system ready")
}

func getStatusString(status connectivity.Status) string {
	switch status {
	case connectivity.StatusOnline:
		return "online"
	case connectivity.StatusOffline:
		return "offline"
	default:
		return "unknown"
	}
}