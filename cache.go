package cache

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"os"
	"path"
	"sync"
	"time"
)

type Cache[T any] struct {
	Identifier string
	Expire     bool
	Expiry     time.Time
	lock       sync.Mutex
}

var (
	Location string
)

// New creates a new Cache instance or attempts to load an existing one from disk.
// It initializes the cache with the given identifier and expiry duration.
// If a cache file exists, it tries to read the header and existing cache metadata.
// It returns a pointer to the Cache or nil if an error occurs during file operations or decoding.
func New[T any](id string, expiry time.Duration) *Cache[T] {
	c := &Cache[T]{
		Identifier: id,
		Expire:     expiry != 0,
		Expiry:     time.Now().Add(expiry),
	}
	//make a slice for the offset
	hb := make([]byte, 4)
	//open the file, creating if not exist
	file, err := os.OpenFile(path.Join(Location, c.Identifier), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil
	}
	defer file.Close()
	//try reading the first 4 bytes
	_, err = file.Read(hb)
	if err != nil {
		//if we cant read because we reach end of file return new cache
		return c
	}
	//create a uint32
	hlen := binary.LittleEndian.Uint32(hb)
	cb := make([]byte, hlen)
	//seek to after the offset
	_, err = file.Seek(4, 0)
	if err != nil {
		return c
	}
	//read until the end of the offset
	_, err = file.Read(cb)
	if err != nil {
		return c
	}
	//put the bytes into a buffer
	cbytes := bytes.NewBuffer(cb)
	//decode those bytes into cache
	err = gob.NewDecoder(cbytes).Decode(c)
	if err != nil {
		return nil
	}
	return c
}

// Set saves the provided slice of items to the cache file associated with the Cache instance.
// It ensures thread-safe access by acquiring a lock.
// It returns an error if the file cannot be opened or if encoding fails.
func (c *Cache[T]) Set(items *[]T) (err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	//create the buffer containing cache
	cb := bytes.NewBuffer([]byte{})
	//put the encoded cache into the buffer
	err = gob.NewEncoder(cb).Encode(c)
	if err != nil {
		return err
	}
	//get the length of the buffer
	hl := uint32(cb.Len())
	//make a barray to hold the uint32
	hlb := make([]byte, 4)
	//put the uint32 in bytes
	binary.LittleEndian.PutUint32(hlb, hl)
	//open file
	file, err := os.OpenFile(path.Join(Location, c.Identifier), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	//truncate the file in case we are reusing it
	file.Truncate(0)
	//write the length of cache to header
	_, err = file.Write(hlb)
	if err != nil {
		return err
	}
	//seek to after the length header
	_, err = file.Seek(4, 0)
	if err != nil {
		return err
	}
	//write cache bytes to file
	_, err = file.ReadFrom(cb)
	if err != nil {
		return err
	}
	//seek to length of cache hb.Len() relative to current seek
	_, err = file.Seek(int64(cb.Len()), 1)
	if err != nil {
		return err
	}
	//now we can write the actual contents of the slice passed in
	err = gob.NewEncoder(file).Encode(items)
	if err != nil {
		return err
	}
	return nil
}

// Get retrieves the items from the cache file.
// It ensures thread-safe access by acquiring a lock.
// If the cache has expired and is configured to expire, the file is removed and nil is returned.
// It returns a pointer to the slice of items or nil if the cache is expired, the file
// cannot be opened, or decoding fails.
func (c *Cache[T]) Get() (items *[]T) {
	c.lock.Lock()
	defer c.lock.Unlock()
	//assign items
	items = &[]T{}
	//check for expiry
	if c.Expiry.Before(time.Now()) && c.Expire {
		os.Remove(path.Join(Location, c.Identifier))
		return nil
	}
	//open the file
	file, err := os.OpenFile(path.Join(Location, c.Identifier), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil
	}
	defer file.Close()
	//read the offset to read from into hb
	hb := make([]byte, 4)
	_, err = file.Read(hb)
	if err != nil {
		return nil
	}
	//seek to after the header
	_, err = file.Seek(4, 0)
	if err != nil {
		return nil
	}
	//set the seek to the offset 1 adds it to the above header seek
	offset := binary.LittleEndian.Uint32(hb)
	_, err = file.Seek(int64(offset), 1)
	if err != nil {
		return nil
	}
	//decode from the file
	err = gob.NewDecoder(file).Decode(items)
	if err != nil {
		return nil
	}
	return items
}
