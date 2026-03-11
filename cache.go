// Package cache provides a simple file-based caching solution with expiry support.
// It stores data on disk with a fixed 48-byte header containing identifier, expiry flag, and timestamp.
package cache

import (
	"encoding/binary"
	"encoding/gob"
	"os"
	"path"
	"sync"
	"time"
)

// Cache represents the header structure stored at the beginning of each cache file.
// The struct is 48 bytes when serialized:
//   - Bytes 0-15: Identifier (up to 16 characters)
//   - Byte 16: Expire flag (0 or 1)
//   - Bytes 17-40: Expiry timestamp (24 bytes for time.Time)
//   - Bytes 41-44: Identifier length (uint32)
//   - Bytes 45-47: Padding (unused)
type Cache struct {
	Identifier string    // The cache key identifier (max 16 characters)
	Expire     bool      // Whether the cache entry has an expiry time
	Expiry     time.Time // When the cache entry expires
}

// Location is the directory where cache files are stored.
// This must be set before using the cache functions.
var Location string

// locks maps cache identifiers to their corresponding mutex for thread-safe access.
var locks = make(map[string]*sync.Mutex)

// Get retrieves cached data for the given identifier.
// It uses a fixed 48-byte header to store metadata.
//
// Parameters:
//   - id: The cache key identifier
//
// Returns:
//   - data: The cached data as a slice of type T
//   - ok: true if data was found and is not expired, false otherwise
//
// The function checks the header for:
//   - Valid identifier match
//   - Expiry time (removes expired files and returns false)
func Get[T any](id string) (data []T, ok bool) {
	// Initialize lock for this identifier if it doesn't exist
	if locks[id] == nil {
		locks[id] = &sync.Mutex{}
	}
	locks[id].Lock()
	defer locks[id].Unlock()

	// Open the cache file for reading
	file, err := os.OpenFile(path.Join(Location, id), os.O_RDONLY, 0644)
	if err != nil {
		return nil, false
	}
	defer file.Close()

	// Read the fixed 48-byte header
	header := make([]byte, 48)
	_, err = file.Read(header)
	if err != nil {
		return nil, false
	}

	// Extract identifier from bytes 0-15
	storedID := string(header[0:16])

	// Read actual identifier length from bytes 41-44
	idLen := binary.LittleEndian.Uint32(header[41:45])
	if idLen > 16 {
		idLen = 16
	}
	storedID = storedID[:idLen]

	// Verify the identifier matches
	if storedID != id {
		return nil, false
	}

	// Extract expire flag from byte 16
	expire := header[16] == 1

	// Extract expiry timestamp from bytes 17-40
	// time.Time layout: sec (int64) + nsec (int32) + loc (pointer)
	sec := int64(binary.LittleEndian.Uint64(header[17:25]))
	nsec := int32(binary.LittleEndian.Uint32(header[25:29]))
	expiry := time.Unix(sec, int64(nsec))

	// Check if the cache has expired
	if expire && expiry.Before(time.Now()) {
		os.Remove(path.Join(Location, id))
		return nil, false
	}

	// Decode the remaining data using gob
	err = gob.NewDecoder(file).Decode(&data)
	if err != nil {
		return nil, false
	}

	return data, true
}

// Set stores data in the cache with an optional expiry duration.
// It uses a fixed 48-byte header for metadata and gob encoding for the data.
//
// Parameters:
//   - id: The cache key identifier (max 16 characters)
//   - data: The data to cache (any slice type)
//   - expiry: Duration until the cache expires. Use 0 for no expiry.
//
// Returns:
//   - ok: true if the data was successfully stored, false on error
//
// The file format is:
//   - Bytes 0-47: Fixed 48-byte header
//   - Bytes 48+: Gob-encoded data
func Set[T any](id string, data []T, expiry time.Duration) (ok bool) {
	// Initialize lock for this identifier if it doesn't exist
	if locks[id] == nil {
		locks[id] = &sync.Mutex{}
	}
	locks[id].Lock()
	defer locks[id].Unlock()

	// Create the fixed 48-byte header
	header := make([]byte, 48)

	// Bytes 0-15: Store identifier (up to 16 characters)
	idBytes := []byte(id)
	copy(header[0:16], idBytes[:min(len(idBytes), 16)])

	// Byte 16: Store expire flag (1 if expiry is set, 0 otherwise)
	if expiry != 0 {
		header[16] = 1
	}

	// Bytes 17-40: Store expiry timestamp
	// Calculate expiry time and extract seconds and nanoseconds
	expiryTime := time.Now().Add(expiry)
	sec := expiryTime.Unix()
	nsec := expiryTime.Nanosecond()

	// Write sec (int64) to bytes 17-24
	binary.LittleEndian.PutUint64(header[17:25], uint64(sec))
	// Write nsec (int32) to bytes 25-28
	binary.LittleEndian.PutUint32(header[25:29], uint32(nsec))
	// loc pointer is left as nil (0) for UTC

	// Bytes 41-44: Store identifier length
	binary.LittleEndian.PutUint32(header[41:45], uint32(min(len(idBytes), 16)))

	// Open or create the cache file
	file, err := os.OpenFile(path.Join(Location, id), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false
	}
	defer file.Close()

	// Write the fixed 48-byte header
	_, err = file.Write(header)
	if err != nil {
		return false
	}

	// Encode and write the data using gob
	err = gob.NewEncoder(file).Encode(data)
	if err != nil {
		return false
	}

	return true
}
