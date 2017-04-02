/*
Copyright 2017 Luke Granger-Brown

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package blte

import (
	"compress/zlib"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
)

var (
	ErrBadMagic = fmt.Errorf("blte: header had bad magic")
)

type chunkInfo struct {
	compressedSize   uint32
	decompressedSize uint32
	checksum         [16]byte
}

type hashingReader struct {
	r io.Reader

	Hash hash.Hash
}

func (r *hashingReader) Read(b []byte) (int, error) {
	n, err := r.r.Read(b)
	r.Hash.Write(b[:n]) // error never returned
	return n, err
}

// ReadByte returns the next byte from the input.
//
// This is implemented mostly to avoid overreading from compress/zlib.
func (r *hashingReader) ReadByte() (byte, error) {
	if br, ok := r.r.(io.ByteReader); ok {
		b, err := br.ReadByte()
		r.Hash.Write([]byte{b}) // error never returned
		return b, err
	}

	buf := make([]byte, 1)
	var n int
	var err error
	for {
		n, err = r.r.Read(buf)
		if n == 1 {
			r.Hash.Write(buf) // error never returned
			return buf[0], nil
		}
		if err != nil {
			return 0, err
		}
	}
	panic("should never get here")
}

type Reader struct {
	r io.Reader

	seenHeader bool

	flags      uint8
	chunkCount uint32
	chunks     []chunkInfo

	currentChunk       uint32
	remainingChunkData []byte
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read(b []byte) (int, error) {
	if err := r.readHeader(); err != nil {
		return 0, err
	}

	// if we have remaining decompressed chunk data, just read that
	if r.remainingChunkData != nil {
		n := copy(b, r.remainingChunkData)
		r.remainingChunkData = r.remainingChunkData[n:]
		if len(r.remainingChunkData) == 0 {
			r.remainingChunkData = nil
		}
		return n, nil
	}

	// read the chunk compression byte, and checksum and decompress the data
	r.currentChunk++
	if err := r.readChunk(); err != nil {
		return 0, err
	}

	n := copy(b, r.remainingChunkData)
	r.remainingChunkData = r.remainingChunkData[n:]
	if len(r.remainingChunkData) == 0 {
		r.remainingChunkData = nil
	}
	return n, nil
}

func (r *Reader) readHeader() error {
	if r.seenHeader {
		return nil
	}
	r.seenHeader = true

	buf, err := readBytes(r.r, 8)
	if err != nil {
		return err
	}
	if buf[0] != 'B' || buf[1] != 'L' || buf[2] != 'T' || buf[3] != 'E' {
		return ErrBadMagic
	}
	hdrLen := binary.BigEndian.Uint32(buf[4:])
	if hdrLen == 0 {
		// no chunk info, just data!
		return nil
	}

	hdrLen -= 8 // already seen bits of the header

	buf, err = readBytes(r.r, 4) // ChunkInfo
	if err != nil {
		return err
	}
	hdrLen -= 4
	r.flags = buf[0]
	buf[0] = 0x00 // wowdev.wiki says this is a uint24, so treat as uint32
	r.chunkCount = binary.BigEndian.Uint32(buf[:4])

	chunks := make([]chunkInfo, r.chunkCount)
	for n := uint32(0); n < r.chunkCount; n++ {
		buf, err = readBytes(r.r, 24) // ChunkInfoEntry
		if err != nil {
			return err
		}
		hdrLen -= 24

		chunks[n] = chunkInfo{
			compressedSize:   binary.BigEndian.Uint32(buf[0:4]),
			decompressedSize: binary.BigEndian.Uint32(buf[4:8]),
		}
		for x := 0; x < 16; x++ {
			chunks[n].checksum[x] = buf[8+x]
		}
	}
	r.chunks = chunks

	if hdrLen != 0 {
		return fmt.Errorf("blte: header is not same as expected length: read %d bytes too many", -hdrLen)
	}

	return r.readChunk()
}

func (r *Reader) readChunk() error {
	var hr io.Reader = r.r
	var hhr *hashingReader
	if r.chunks != nil {
		// if this isn't a single chunk file, we'll want to check the hash
		if r.currentChunk >= uint32(len(r.chunks)) {
			return io.EOF
		}
		hhr = &hashingReader{
			r: &io.LimitedReader{
				R: r.r,
				N: int64(r.chunks[r.currentChunk].compressedSize),
			},
			Hash: md5.New(),
		}
		hr = hhr
	}

	// read the chunk byte
	cms, err := readBytes(hr, 1)
	if err != nil {
		return err
	}
	cm := cms[0]

	// construct the reader
	var rr io.Reader
	switch cm {
	case 'N':
		rr = hr
	case 'Z':
		rr, err = zlib.NewReader(hr)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("blte: unsupported compression method %v", cm)
	}

	// read the whole thing
	r.remainingChunkData, err = ioutil.ReadAll(rr)
	if err != nil {
		return err
	}

	// if we have a hashingReader, check the hash
	if hhr != nil {
		hash := hhr.Hash.Sum(nil)
		match := true
		for n := 0; n < len(hash); n++ {
			if hash[n] != r.chunks[r.currentChunk].checksum[n] {
				match = false
			}
		}
		if !match {
			return fmt.Errorf("blte: checksum mismatch in chunk %d: calculated %x, header said %x", r.currentChunk, hash, r.chunks[r.currentChunk].checksum)
		}
	}

	return nil
}

func readBytes(r io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
