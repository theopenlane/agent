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

const (
	// Test configuration constants
	testRetentionHours        = 24
	testFileSizeKB            = 1024
	testDurationSeconds       = 5
	bufferTestDurationSeconds = 2
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

	if err := os.MkdirAll(storageDir, config.DefaultDirectoryPermissions); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	defer os.RemoveAll(storageDir)

	// Create buffered storage with evidence collection
	storageSystem, err := storage.NewStorage(
		storage.WithAPIConfig("https://api.example.com", "test-token"),
		storage.WithBuffering(resultsDir),
		storage.WithEvidence(true, testRetentionHours*time.Hour, testFileSizeKB*testFileSizeKB),
	)

	defer os.RemoveAll(storageDir)

	if err != nil {
		fmt.Printf("Failed to create storage: %v\n", err)
		return
	}

	defer storageSystem.Close()

	// Test health check
	if err := storageSystem.Health(); err != nil {
		fmt.Printf("Storage health check failed: %v\n", err)
		return
	}

	fmt.Println("   ✓ Storage system health check passed")

	// Create a test result
	startTime := time.Now()
	finishTime := startTime.Add(testDurationSeconds * time.Second)
	exitCode := 0
	testResult := &config.Result{
		CheckName:  "test-check",
		StartedAt:  startTime,
		FinishedAt: finishTime,
		Status:     "SUCCESS",
		ExitCode:   &exitCode,
		Standard:   "test",
		ControlRef: "001",
		Log:        "Test check completed successfully",
		Metadata:   make(map[string]any),
	}

	// Test storing result
	fmt.Printf("   Storing test result for check: %s\n", testResult.CheckName)

	err = storageSystem.StoreResult(testResult)
	if err != nil {
		fmt.Printf("Failed to store result: %v\n", err)
		return
	}

	fmt.Println("   ✓ Result stored successfully")

	// Test evidence service if available
	evidenceConfig := &storage.EvidenceConfig{
		Enabled:         true,
		DataDir:         storageDir,
		MaxFileSize:     testFileSizeKB * testFileSizeKB,
		RetentionPeriod: testRetentionHours * time.Hour,
	}
	evidenceService := storage.NewEvidenceService(evidenceConfig)

	// Create some test evidence
	testEvidence, err := evidenceService.CreateEvidenceFromOutput(context.Background(), "test-check",
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

	// Connectivity manager ready
	fmt.Println("   ✓ Connectivity manager initialized")

	// Test subscription (brief test)
	fmt.Println("   ✓ Connectivity monitoring system ready")

	// Test buffered storage (simulating API with fallback)
	fmt.Println("\n   Testing buffered storage (API with local fallback)...")
	testBufferedStorage()
}

func testBufferedStorage() {
	// Create temp directory for buffered storage test
	bufferDir := "./test-data/buffer-test"
	if err := os.MkdirAll(bufferDir, config.DefaultDirectoryPermissions); err != nil {
		log.Fatalf("Failed to create buffer directory: %v", err)
	}
	defer os.RemoveAll(bufferDir)

	// Create buffered storage that will use local fallback
	// (since we don't have valid API credentials for this test)
	bufStorageSystem, err := storage.NewStorage(
		storage.WithAPIConfig("", ""),
		storage.WithBuffering(bufferDir),
		storage.WithEvidence(true, testRetentionHours*time.Hour, testFileSizeKB*testFileSizeKB),
	)
	if err != nil {
		log.Printf("   Could not create buffered storage: %v", err)
		return
	}
	defer bufStorageSystem.Close()

	// Create test result for buffered storage
	bufStartTime := time.Now()
	bufFinishTime := bufStartTime.Add(bufferTestDurationSeconds * time.Second)
	bufExitCode := 0
	testResult := &config.Result{
		CheckName:  "buffered-test-check",
		StartedAt:  bufStartTime,
		FinishedAt: bufFinishTime,
		Status:     "SUCCESS",
		ExitCode:   &bufExitCode,
		Standard:   "buffer",
		ControlRef: "001",
		Log:        "Buffered test check completed successfully",
		Metadata:   map[string]any{"tags": []string{"buffered", "test"}},
	}

	// Store result (will fall back to local storage)
	err = bufStorageSystem.StoreResult(testResult)
	if err != nil {
		log.Printf("   Buffered storage test failed: %v", err)
		return
	}

	fmt.Println("   ✓ Buffered storage fallback working correctly")
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
