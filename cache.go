// Package cache provides a simple file-based caching solution with expiry support.
// It stores data on disk with a fixed 160-byte header containing identifier, expiry flag, and timestamp.
package cache

import (
	"encoding/binary"
	"encoding/gob"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxIDLen = 128
	// Header layout (160 bytes):
	//   - Bytes 0-127: Identifier (up to 128 characters)
	//   - Byte 128: Expire flag (0 or 1)
	//   - Bytes 129-152: Expiry timestamp (24 bytes for time.Time)
	//   - Bytes 153-156: Identifier length (uint32)
	//   - Bytes 157-159: Padding (unused)
	headerSize = 160
)

// Cache represents the header structure stored at the beginning of each cache file.
type Cache struct {
	Identifier string    // The cache key identifier (max 128 characters)
	Expire     bool      // Whether the cache entry has an expiry time
	Expiry     time.Time // When the cache entry expires
}

// Location is the directory where cache files are stored.
// This must be set before using the cache functions.
var Location string

// locks maps cache identifiers to their corresponding mutex for thread-safe access.
var locks = make(map[string]*sync.Mutex)

// locksMu protects the locks map itself.
var locksMu sync.Mutex

// Get retrieves cached data for the given identifier.
// It uses a fixed 160-byte header to store metadata.
//
// Parameters:
//   - id: The cache key identifier
//
// Returns:
//   - data: The cached data of type T
//   - ok: true if data was found and is not expired, false otherwise
//
// The function checks the header for:
//   - Valid identifier match
//   - Expiry time (removes expired files and returns false)
func Get[T any](id string) (data T, ok bool) {
	// Initialize lock for this identifier if it doesn't exist
	locksMu.Lock()
	if locks[id] == nil {
		locks[id] = &sync.Mutex{}
	}
	mu := locks[id]
	locksMu.Unlock()
	mu.Lock()
	defer mu.Unlock()

	// Open the cache file for reading
	file, err := os.OpenFile(path.Join(Location, id), os.O_RDONLY, 0644)
	if err != nil {
		return data, false
	}
	defer file.Close()

	return readCache[T](file, id, true)
}

// GetReader retrieves cached data from any io.Reader using the cache header format.
// It does not perform file-based expiry cleanup or locking.
//
// Parameters:
//   - r: The reader to read from (must support the cache header format)
//   - id: The expected identifier to validate against
//
// Returns:
//   - data: The cached data of type T
//   - ok: true if data was successfully read and not expired, false otherwise
func GetReader[T any](r io.Reader, id string) (data T, ok bool) {
	return readCache[T](r, id, false)
}

// readCache reads a cache entry from any io.Reader.
// If removeExpired is true, expired files will be deleted from disk.
func readCache[T any](r io.Reader, id string, removeExpired bool) (data T, ok bool) {
	// Read the fixed header
	header := make([]byte, headerSize)
	_, err := io.ReadFull(r, header)
	if err != nil {
		return data, false
	}

	// Extract identifier from bytes 0-127
	storedID := string(header[0:maxIDLen])

	// Read actual identifier length from bytes 153-156
	idLen := binary.LittleEndian.Uint32(header[153:157])
	if idLen > maxIDLen {
		idLen = maxIDLen
	}
	storedID = storedID[:idLen]

	// Verify the identifier matches
	if storedID != id {
		return data, false
	}

	// Extract expire flag from byte 128
	expire := header[128] == 1

	// Extract expiry timestamp from bytes 129-152
	sec := int64(binary.LittleEndian.Uint64(header[129:137]))
	nsec := int32(binary.LittleEndian.Uint32(header[137:141]))
	expiry := time.Unix(sec, int64(nsec))

	// Check if the cache has expired
	if expire && expiry.Before(time.Now()) {
		if removeExpired {
			os.Remove(path.Join(Location, id))
		}
		return data, false
	}

	// Decode the remaining data using gob
	err = gob.NewDecoder(r).Decode(&data)
	if err != nil {
		return data, false
	}

	return data, true
}

// Set stores data in the cache with an optional expiry duration.
// It uses a fixed 160-byte header for metadata and gob encoding for the data.
//
// Parameters:
//   - id: The cache key identifier (max 128 characters)
//   - data: The data to cache (any type)
//   - expiry: Duration until the cache expires. Use 0 for no expiry.
//
// Returns:
//   - ok: true if the data was successfully stored, false on error
//
// The file format is:
//   - Bytes 0-159: Fixed header
//   - Bytes 160+: Gob-encoded data
func Set[T any](id string, data T, expiry time.Duration) (ok bool) {
	// Initialize lock for this identifier if it doesn't exist
	locksMu.Lock()
	if locks[id] == nil {
		locks[id] = &sync.Mutex{}
	}
	mu := locks[id]
	locksMu.Unlock()
	mu.Lock()
	defer mu.Unlock()

	// Open or create the cache file
	file, err := os.OpenFile(path.Join(Location, id), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false
	}
	defer file.Close()

	return writeCache(file, id, data, expiry)
}

// SetWriter stores cache data to any io.Writer using the cache header format.
// It does not perform file-based locking.
//
// Parameters:
//   - w: The writer to write to
//   - id: The cache key identifier (max 128 characters)
//   - data: The data to cache (any type)
//   - expiry: Duration until the cache expires. Use 0 for no expiry.
//
// Returns:
//   - ok: true if the data was successfully written, false on error
func SetWriter[T any](w io.Writer, id string, data T, expiry time.Duration) (ok bool) {
	return writeCache(w, id, data, expiry)
}

// writeCache writes a cache entry to any io.Writer.
func writeCache[T any](w io.Writer, id string, data T, expiry time.Duration) bool {
	// Create the fixed header
	header := make([]byte, headerSize)

	// Bytes 0-127: Store identifier (up to 128 characters)
	idBytes := []byte(id)
	idLen := len(idBytes)
	if idLen > maxIDLen {
		idLen = maxIDLen
	}
	copy(header[0:maxIDLen], idBytes[:idLen])

	// Byte 128: Store expire flag (1 if expiry is set, 0 otherwise)
	if expiry != 0 {
		header[128] = 1
	}

	// Bytes 129-152: Store expiry timestamp
	expiryTime := time.Now().Add(expiry)
	sec := expiryTime.Unix()
	nsec := expiryTime.Nanosecond()

	binary.LittleEndian.PutUint64(header[129:137], uint64(sec))
	binary.LittleEndian.PutUint32(header[137:141], uint32(nsec))

	// Bytes 153-156: Store identifier length
	binary.LittleEndian.PutUint32(header[153:157], uint32(idLen))

	// Write the fixed header
	_, err := w.Write(header)
	if err != nil {
		return false
	}

	// Encode and write the data using gob
	err = gob.NewEncoder(w).Encode(data)
	if err != nil {
		return false
	}

	return true
}

// scanExpired scans the cache directory and removes any entries that have expired.
// It uses a per-file lock to avoid racing with Get/Set on the same identifier.
func scanExpired() {
	entries, err := os.ReadDir(Location)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		id := entry.Name()

		// Acquire the per-identifier lock, creating it if needed.
		locksMu.Lock()
		if locks[id] == nil {
			locks[id] = &sync.Mutex{}
		}
		mu := locks[id]
		locksMu.Unlock()
		mu.Lock()

		file, err := os.OpenFile(filepath.Join(Location, id), os.O_RDONLY, 0644)
		if err != nil {
			mu.Unlock()
			continue
		}

		header := make([]byte, headerSize)
		_, err = io.ReadFull(file, header)
		file.Close()
		mu.Unlock()
		if err != nil {
			continue
		}

		expire := header[128] == 1
		if !expire {
			continue
		}

		sec := int64(binary.LittleEndian.Uint64(header[129:137]))
		nsec := int32(binary.LittleEndian.Uint32(header[137:141]))
		expiry := time.Unix(sec, int64(nsec))

		if expiry.Before(time.Now()) {
			// Re-acquire lock for removal to avoid racing with Get.
			mu.Lock()
			os.Remove(filepath.Join(Location, id))
			mu.Unlock()
		}
	}
}

// Reaper starts a background goroutine that scans the cache directory every
// minute and deletes expired entries. It returns immediately and runs for
// the lifetime of the process. It should be called after Location is set.
func Reaper() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			scanExpired()
		}
	}()
}
