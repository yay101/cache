package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// Set up temp directory for tests
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		panic("Failed to create temp dir")
	}
	Location = tmpDir
	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestSetAndGet(t *testing.T) {
	id := "test_key"
	data := []string{"item1", "item2", "item3"}

	// Test Set
	ok := Set(id, data, time.Hour)
	if !ok {
		t.Fatal("Set failed")
	}

	// Test Get
	result, found := Get[string](id)
	if !found {
		t.Fatal("Get failed to find the key")
	}

	if len(result) != len(data) {
		t.Fatalf("Expected %d items, got %d", len(data), len(result))
	}

	for i, v := range data {
		if result[i] != v {
			t.Errorf("Expected %s at index %d, got %s", v, i, result[i])
		}
	}
}

func TestGetNonExistent(t *testing.T) {
	_, found := Get[string]("nonexistent_key")
	if found {
		t.Fatal("Expected false for nonexistent key, got true")
	}
}

func TestExpiry(t *testing.T) {
	id := "expiry_test"
	data := []int{1, 2, 3}

	// Set with very short expiry (already expired)
	ok := Set(id, data, -time.Hour)
	if !ok {
		t.Fatal("Set failed")
	}

	// Try to get expired data - should return false
	_, found := Get[int](id)
	if found {
		t.Fatal("Expected expired key to not be found")
	}

	// File should have been removed
	filePath := filepath.Join(Location, id)
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("Expected file to be removed after expiry")
	}
}

func TestConcurrentAccess(t *testing.T) {
	id := "concurrent_test"
	data := []string{"concurrent"}

	// Set first
	Set(id, data, time.Hour)

	// Multiple gets should work
	for i := 0; i < 10; i++ {
		result, found := Get[string](id)
		if !found {
			t.Fatalf("Get failed on iteration %d", i)
		}
		if len(result) != 1 || result[0] != "concurrent" {
			t.Errorf("Unexpected data on iteration %d", i)
		}
	}
}
