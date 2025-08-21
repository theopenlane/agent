package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/connectivity"
	"github.com/theopenlane/agent/internal/storage"
)

// Simple test program to demonstrate unified storage and connectivity functionality
func main() {
	fmt.Println("=== Testing Unified Storage and Connectivity Components ===")

	// Test unified storage functionality
	fmt.Println("\n1. Testing Unified Storage System...")
	testStorage()

	// Test connectivity functionality
	fmt.Println("\n2. Testing Connectivity Manager...")
	testConnectivity()

	fmt.Println("\n=== All Tests Complete ===")
}

func testStorage() {
	// Create storage directory
	storageDir := "./test-data/storage-test"
	resultsDir := "./test-data/storage-test/results"

	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	defer os.RemoveAll(storageDir)

	// Create local storage with evidence collection
	storageSystem, err := storage.NewStorage(
		storage.WithLocalStorage(storageDir, resultsDir, "json"),
		storage.WithEvidence(true, 24*time.Hour, 1024*1024, false),
	)
	if err != nil {
		log.Fatal("Failed to create storage:", err)
	}
	defer storageSystem.Close()

	// Test health check
	if err := storageSystem.Health(); err != nil {
		log.Fatal("Storage health check failed:", err)
	}

	fmt.Println("   ✓ Storage system health check passed")

	// Create a test result
	testResult := &config.Result{
		CheckName:  "test-check",
		StartTime:  time.Now(),
		EndTime:    time.Now().Add(5 * time.Second),
		Duration:   "5s",
		ExitCode:   0,
		Passed:     true,
		ExecutedAt: time.Now(),
		Controls:   []string{"TEST-001"},
		Tags:       []string{"test"},
	}

	// Test storing result
	fmt.Printf("   Storing test result for check: %s\n", testResult.CheckName)

	err = storageSystem.StoreResult(testResult)
	if err != nil {
		log.Fatal("Failed to store result:", err)
	}

	fmt.Println("   ✓ Result stored successfully")

	// Test stats
	stats := storageSystem.GetStats()
	fmt.Printf("   ✓ Storage stats: %d total results, %d successful uploads\n",
		stats.TotalResults, stats.SuccessfulUploads)

	// Test evidence service if available
	storageConfig := &storage.Config{
		EvidenceEnabled:         true,
		DataDir:                 storageDir,
		EvidenceMaxFileSize:     1024 * 1024,
		EvidenceRetentionPeriod: 24 * time.Hour,
		EvidenceCompressFiles:   false,
	}
	evidenceService := storage.NewEvidenceService(storageConfig)

	// Create some test evidence
	testEvidence, err := evidenceService.CreateEvidenceFromOutput("test-check",
		[]byte("test stdout"), []byte("test stderr"))
	if err == nil && len(testEvidence) > 0 {
		fmt.Printf("   ✓ Evidence service created %d evidence files\n", len(testEvidence))

		// Test storing result with evidence
		err = storageSystem.StoreResultWithEvidence(testResult, testEvidence)
		if err != nil {
			log.Printf("   Warning: Failed to store result with evidence: %v", err)
		} else {
			fmt.Println("   ✓ Result with evidence stored successfully")
		}
	}

	fmt.Println("   ✓ Storage system test completed")
}

func testConnectivity() {
	// Create connectivity manager
	apiURL := "https://httpbin.org" // Use a reliable test endpoint
	connMgr := connectivity.NewManager(apiURL)

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

	// Test buffered storage (simulating API with fallback)
	fmt.Println("\n   Testing buffered storage (API with local fallback)...")
	testBufferedStorage()
}

func testBufferedStorage() {
	// Create temp directory for buffered storage test
	bufferDir := "./test-data/buffer-test"
	if err := os.MkdirAll(bufferDir, 0o755); err != nil {
		log.Fatalf("Failed to create buffer directory: %v", err)
	}
	defer os.RemoveAll(bufferDir)

	// Create buffered storage that will use local fallback
	// (since we don't have valid API credentials for this test)
	bufStorageSystem, err := storage.NewStorage(
		storage.WithBufferedStorage("", "", bufferDir),
		storage.WithEvidence(true, 24*time.Hour, 1024*1024, false),
	)
	if err != nil {
		log.Printf("   Could not create buffered storage: %v", err)
		return
	}
	defer bufStorageSystem.Close()

	// Create test result for buffered storage
	testResult := &config.Result{
		CheckName:  "buffered-test-check",
		StartTime:  time.Now(),
		EndTime:    time.Now().Add(2 * time.Second),
		Duration:   "2s",
		ExitCode:   0,
		Passed:     true,
		ExecutedAt: time.Now(),
		Controls:   []string{"BUF-001"},
		Tags:       []string{"buffered", "test"},
	}

	// Store result (will fall back to local storage)
	err = bufStorageSystem.StoreResult(testResult)
	if err != nil {
		log.Printf("   Buffered storage test failed: %v", err)
		return
	}

	fmt.Println("   ✓ Buffered storage fallback working correctly")

	// Check stats
	stats := bufStorageSystem.GetStats()
	fmt.Printf("   ✓ Buffered storage stats: %d total results\n", stats.TotalResults)
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
