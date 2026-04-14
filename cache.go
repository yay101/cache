// Package cache provides a simple file-based caching solution with expiry support.
// It stores data on disk with a fixed 160-byte header containing identifier, expiry flag, and timestamp.
package cache

import (
	"encoding/binary"
	"encoding/gob"
	"os"
	"path"
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
	if locks[id] == nil {
		locks[id] = &sync.Mutex{}
	}
	locks[id].Lock()
	defer locks[id].Unlock()

	// Open the cache file for reading
	file, err := os.OpenFile(path.Join(Location, id), os.O_RDONLY, 0644)
	if err != nil {
		return data, false
	}
	defer file.Close()

	// Read the fixed header
	header := make([]byte, headerSize)
	_, err = file.Read(header)
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
	// time.Time layout: sec (int64) + nsec (int32) + loc (pointer)
	sec := int64(binary.LittleEndian.Uint64(header[129:137]))
	nsec := int32(binary.LittleEndian.Uint32(header[137:141]))
	expiry := time.Unix(sec, int64(nsec))

	// Check if the cache has expired
	if expire && expiry.Before(time.Now()) {
		os.Remove(path.Join(Location, id))
		return data, false
	}

	// Decode the remaining data using gob
	err = gob.NewDecoder(file).Decode(&data)
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
	if locks[id] == nil {
		locks[id] = &sync.Mutex{}
	}
	locks[id].Lock()
	defer locks[id].Unlock()

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
	// Calculate expiry time and extract seconds and nanoseconds
	expiryTime := time.Now().Add(expiry)
	sec := expiryTime.Unix()
	nsec := expiryTime.Nanosecond()

	// Write sec (int64) to bytes 129-136
	binary.LittleEndian.PutUint64(header[129:137], uint64(sec))
	// Write nsec (int32) to bytes 137-140
	binary.LittleEndian.PutUint32(header[137:141], uint32(nsec))
	// loc pointer is left as nil (0) for UTC

	// Bytes 153-156: Store identifier length
	binary.LittleEndian.PutUint32(header[153:157], uint32(idLen))

	// Open or create the cache file
	file, err := os.OpenFile(path.Join(Location, id), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false
	}
	defer file.Close()

	// Write the fixed header
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
