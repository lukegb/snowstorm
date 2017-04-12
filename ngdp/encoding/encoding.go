/*
Copyright 2017 Luke Granger-Brown

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the Licensm.
You may obtain a copy of the License at

     http://www.apachm.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the Licensm.
*/

package encoding

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"sort"

	"github.com/lukegb/snowstorm/ngdp"
)

type hash [16]byte

// Error constants
var (
	ErrBadMagic           = fmt.Errorf("encoding: bad magic")
	ErrBadHashSize        = fmt.Errorf("encoding: bad hash size in header")
	ErrUnknownContentHash = fmt.Errorf("encoding: unknown content hash")
	ErrTooManyCDNHashes   = fmt.Errorf("encoding: multiple CDN hashes listed")
)

type mapEntry struct {
	contentHash ngdp.ContentHash
	cdnHashes   []ngdp.CDNHash
}

// A Mapper converts file content hashes into their corresponding CDN hashes.
type Mapper struct {
	keys []mapEntry
}

// NewMapper creates a new Mapper from a provided encoding file.
//
// The encoding file should not be in BLTE format - it should already have been decoded.
func NewMapper(r io.Reader) (*Mapper, error) {
	m := &Mapper{}
	if err := m.init(r); err != nil {
		return nil, err
	}
	return m, nil
}

type header struct {
	hashSizeA  uint8
	hashSizeB  uint8
	flagsA     uint16
	flagsB     uint16
	sizeA      uint32
	sizeB      uint32
	stringSize uint32
}

func (m *Mapper) readHeader(r io.Reader) (*header, error) {
	buf := make([]byte, 22)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	if buf[0] != 'E' || buf[1] != 'N' {
		return nil, ErrBadMagic
	}

	var h header
	hashSizeA := buf[3]
	hashSizeB := buf[4]
	if hashSizeA != 0x10 || hashSizeB != 0x10 {
		return nil, ErrBadHashSize
	}
	h.flagsA = binary.BigEndian.Uint16(buf[0x5:0x7])
	h.flagsB = binary.BigEndian.Uint16(buf[0x7:0x9])
	h.sizeA = binary.BigEndian.Uint32(buf[0x9:0x0d])
	h.sizeB = binary.BigEndian.Uint32(buf[0x0d:0x11])
	h.stringSize = binary.BigEndian.Uint32(buf[0x12:0x16])

	return &h, nil
}

func sliceToHash(b []byte) hash {
	var x [16]byte
	for n := 0; n < 16; n++ {
		x[n] = b[n]
	}
	return x
}

// ToCDNHash converts a content hash into a single CDN hash.
//
// It is possible for a single content hash to map to multiple CDN hashes. In this case, an error is thrown - the semantics of what multiple CDN hashes means is currently unclear.
func (m *Mapper) ToCDNHash(contentHash ngdp.ContentHash) (ngdp.CDNHash, error) {
	i := sort.Search(len(m.keys), func(n int) bool {
		return !m.keys[n].contentHash.Less(contentHash)
	})
	if i >= len(m.keys) || !m.keys[i].contentHash.Equal(contentHash) {
		return ngdp.CDNHash{}, ErrUnknownContentHash
	}
	x := m.keys[i]
	if len(x.cdnHashes) != 1 {
		return ngdp.CDNHash{}, ErrTooManyCDNHashes
	}
	return x.cdnHashes[0], nil
}

func (m *Mapper) init(r io.Reader) error {
	h, err := m.readHeader(r)
	if err != nil {
		return fmt.Errorf("encoding: reading header: %v", err)
	}

	// Skip over the layout string table; we don't need it
	if _, err := io.CopyN(ioutil.Discard, r, int64(h.stringSize)); err != nil {
		return fmt.Errorf("encoding: skipping layout string table: %v", err)
	}

	// Read key table index
	keyEntryHashes := make([][16]byte, h.sizeA)
	buf := make([]byte, 32)
	for n := uint32(0); n < h.sizeA; n++ {
		if _, err := io.ReadFull(r, buf); err != nil {
			return fmt.Errorf("encoding: reading %d entry in key table index: %v", n, err)
		}
		for x := 0; x < 16; x++ {
			keyEntryHashes[n][x] = buf[0x10+x]
		}
	}

	var slc []mapEntry

	// Read key table entries
	buf = make([]byte, 4096)
	for n := uint32(0); n < h.sizeA; n++ {
		if _, err := io.ReadFull(r, buf); err != nil {
			return fmt.Errorf("encoding: reading %d entry in key table: %v", n, err)
		}
		h := md5.Sum(buf)
		match := true
		for x := 0; x < 16; x++ {
			if h[x] != keyEntryHashes[n][x] {
				match = false
			}
		}
		if !match {
			return fmt.Errorf("encoding: key table entry %d hash mismatch: want %x, got %x", keyEntryHashes[n], h)
		}

		keybuf := buf
		for {
			cdnKeyCount := binary.LittleEndian.Uint16(keybuf[0x0:0x2])
			if cdnKeyCount == 0x0 {
				break
			}
			contentHash := ngdp.ContentHash(sliceToHash(keybuf[0x06:0x16]))
			keybuf = keybuf[0x16:]
			cdnKeys := make([]ngdp.CDNHash, cdnKeyCount)
			for x := uint16(0); x < cdnKeyCount; x++ {
				cdnKeys[x] = ngdp.CDNHash(sliceToHash(keybuf[:0x10]))
				keybuf = keybuf[0x10:]
			}

			slc = append(slc, mapEntry{
				contentHash: contentHash,
				cdnHashes:   cdnKeys,
			})
		}
	}

	m.keys = make([]mapEntry, len(slc))
	copy(m.keys, slc)

	// Skip over layout table index and entries
	if _, err := io.CopyN(ioutil.Discard, r, int64(h.sizeB*32)); err != nil {
		return fmt.Errorf("encoding: skipping layout table index: %v", err)
	}
	if _, err := io.CopyN(ioutil.Discard, r, int64(h.sizeB*4096)); err != nil {
		return fmt.Errorf("encoding: skipping layout table entries: %v", err)
	}
	// TODO(lukegb): also skip over the layout string that describes this file at the end

	return nil
}
