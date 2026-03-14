# Cache Package

A simple file-based caching solution with expiry support for Go applications.

## Features

- **File-based storage**: Cache data is persisted to disk
- **Fixed header**: 160-byte header for efficient metadata storage
- **Expiry support**: Optional time-based expiration
- **Thread-safe**: Mutex-based locking per cache key
- **Generic support**: Works with any data type using Go generics

## Installation

```bash
go get github.com/yourusername/cache
```

## Quick Start

```go/var/www/cache/example.go#L1-20
package main

import (
    "time"
    "github.com/yourusername/cache"
)

func main() {
    // Set the cache directory
    cache.Location = "/tmp/mycache"

    // Store data with 1 hour expiry
    ok := Set("user:123", []string{"data1", "data2"}, time.Hour)
    if !ok {
        // Handle error
    }

    // Retrieve data
    data, found := Get[string]("user:123")
    if found {
        // Use data
        println(data[0]) // outputs: data1
    }

    // Store data without expiry
    Set("config", []int{1, 2, 3}, 0)
}
```

## API

### Variables

- `Location string` - Directory where cache files are stored. Must be set before using Get/Set.

### Functions

#### Get[T any](id string) (data []T, ok bool)

Retrieves cached data for the given identifier.

**Parameters:**
- `id` - The cache key identifier (max 128 characters)

**Returns:**
- `data` - The cached data as a slice of type T
- `ok` - true if data was found and is not expired, false otherwise

**Behavior:**
- Returns `false` if the key doesn't exist
- Returns `false` and removes the file if the cache has expired
- Thread-safe with per-key locking

#### Set[T any](id string, data []T, expiry time.Duration) (ok bool)

Stores data in the cache with an optional expiry duration.

**Parameters:**
- `id` - The cache key identifier (max 128 characters)
- `data` - The data to cache (any slice type)
- `expiry` - Duration until the cache expires. Use `0` for no expiry.

**Returns:**
- `ok` - true if the data was successfully stored, false on error

### Cache Struct

The `Cache` struct represents the header metadata:

```go/var/www/cache/cache_struct.go#L1-8
type Cache struct {
    Identifier string    // The cache key (max 128 chars)
    Expire     bool      // Whether the entry has expiry
    Expiry     time.Time // When the entry expires
}
```

## File Format

Each cache file consists of:

1. **Fixed 160-byte header:**
   - Bytes 0-127: Identifier (up to 128 characters)
   - Byte 128: Expire flag (0 or 1)
   - Bytes 129-152: Expiry timestamp (time.Time)
   - Bytes 153-156: Identifier length
   - Bytes 157-159: Padding

2. **Gob-encoded data:** The remaining bytes contain the cached data encoded using Go's `encoding/gob` package.

## Thread Safety

The package uses per-key mutex locks to ensure thread-safe access. Each unique cache identifier has its own lock, allowing concurrent access to different keys while serializing access to the same key.

## Limitations

- Maximum identifier length: 128 characters
- Data must be sliceable (use `[]T` for single items)
- Identifier must be unique within the cache directory

## Example: Using with Custom Types

```go/var/www/cache/custom_type.go#L1-25
package main

import (
    "time"
    "cache"
)

type User struct {
    ID   int
    Name string
}

func main() {
    cache.Location = "/tmp/cache"

    // Store custom struct
    users := []User{
        {ID: 1, Name: "Alice"},
        {ID: 2, Name: "Bob"},
    }
    Set("users", users, time.Hour)

    // Retrieve
    result, ok := Get[User]("users")
    if ok {
        for _, u := range result {
            println(u.Name)
        }
    }
}
