package storedcounter

import (
	"encoding/binary"

	"github.com/ipfs/go-datastore"
)

// StoredCounter is a counter that persists to a datastore as it increments
type StoredCounter struct {
	ds   datastore.Datastore
	name datastore.Key
}

// New returns a new StoredCounter for the given datastore and key
func New(ds datastore.Datastore, name datastore.Key) *StoredCounter {
	return &StoredCounter{ds, name}
}

// Next returns the next counter value, updating it on disk in the process
// if no counter is present, it creates one and returns a 0 value
func (sc *StoredCounter) Next() (uint64, error) {
	has, err := sc.ds.Has(sc.name)
	if err != nil {
		return 0, err
	}

	var next uint64 = 0
	if has {
		curBytes, err := sc.ds.Get(sc.name)
		if err != nil {
			return 0, err
		}
		cur, _ := binary.Uvarint(curBytes)
		next = cur + 1
	}
	buf := make([]byte, binary.MaxVarintLen64)
	size := binary.PutUvarint(buf, next)

	return next, sc.ds.Put(sc.name, buf[:size])
}
