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

type Cache struct {
	Identifier string
	Expire     bool
	Expiry     time.Time
}

var (
	Location string
	locks    = make(map[string]*sync.Mutex)
)

func Get[T any](id string) (data []T, ok bool) {
	//check lock exists, create otherwise
	if locks[id] == nil {
		locks[id] = &sync.Mutex{}
	}
	locks[id].Lock()
	defer locks[id].Unlock()

	//open the file
	file, err := os.OpenFile(path.Join(Location, id), os.O_RDONLY, 0644)
	if err != nil {
		return nil, false
	}
	defer file.Close()

	//read the header length (first 4 bytes)
	hb := make([]byte, 4)
	_, err = file.Read(hb)
	if err != nil {
		return nil, false
	}

	//create a uint32 for header length
	hlen := binary.LittleEndian.Uint32(hb)

	//read the header bytes
	cb := make([]byte, hlen)
	_, err = file.Read(cb)
	if err != nil {
		return nil, false
	}

	//decode the header
	header := &Cache{}
	cbytes := bytes.NewBuffer(cb)
	err = gob.NewDecoder(cbytes).Decode(header)
	if err != nil {
		return nil, false
	}

	//check expiry, return nil, false if expired
	if header.Expire && header.Expiry.Before(time.Now()) {
		os.Remove(path.Join(Location, id))
		return nil, false
	}

	//read the identifier and make sure it matches id
	if header.Identifier != id {
		return nil, false
	}

	//gob decode the data into data and return data and true
	err = gob.NewDecoder(file).Decode(&data)
	if err != nil {
		return nil, false
	}

	return data, true
}

func Set[T any](id string, data []T, expiry time.Duration) (ok bool) {
	//check lock exists, create otherwise
	if locks[id] == nil {
		locks[id] = &sync.Mutex{}
	}
	locks[id].Lock()
	defer locks[id].Unlock()

	//create the header
	header := &Cache{
		Identifier: id,
		Expire:     expiry != 0,
		Expiry:     time.Now().Add(expiry),
	}

	//gob encode the header
	hb := bytes.NewBuffer([]byte{})
	err := gob.NewEncoder(hb).Encode(header)
	if err != nil {
		return false
	}

	//get header length
	hl := uint32(hb.Len())
	hlb := make([]byte, 4)
	binary.LittleEndian.PutUint32(hlb, hl)

	//open file
	file, err := os.OpenFile(path.Join(Location, id), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false
	}
	defer file.Close()

	//write header length
	_, err = file.Write(hlb)
	if err != nil {
		return false
	}

	//write header bytes
	_, err = file.Write(hb.Bytes())
	if err != nil {
		return false
	}

	//gob encode the data
	err = gob.NewEncoder(file).Encode(data)
	if err != nil {
		return false
	}

	//gob encode the data true
	return true
}
