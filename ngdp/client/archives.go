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

package client

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"io"

	"golang.org/x/sync/errgroup"

	"github.com/lukegb/snowstorm/ngdp"
)

const (
	archiveConcurrentIndexFetches = 20
	archiveIndexChunkSize         = 4096
	archiveEntriesPerChunk        = 170
)

type archiveIndexEntry struct {
	archive *ngdp.CDNHash
	size    uint32
	offset  uint32
}

func (ade archiveIndexEntry) asArchiveEntry() ArchiveEntry {
	return ArchiveEntry{
		Archive: *ade.archive,
		Size:    ade.size,
		Offset:  ade.offset,
	}
}

// An ArchiveMapper maps file CDN hashes to their location within the set of archives.
type ArchiveMapper struct {
	m map[ngdp.CDNHash]archiveIndexEntry
}

// An ArchiveEntry contains the location of a given file within the archive set.
type ArchiveEntry struct {
	Archive ngdp.CDNHash
	Size    uint32
	Offset  uint32
}

// Map takes a CDNHash of a desired file and returns the CDNHash of the containing archive, as well as the size and offset within the archive.
//
// If the file does not exist in any known archives, then ok will be false.
func (e *ArchiveMapper) Map(in ngdp.CDNHash) (entry ArchiveEntry, ok bool) {
	ade, ok := e.m[in]
	if !ok {
		return ArchiveEntry{}, false
	}

	return ade.asArchiveEntry(), true
}

func buildArchiveMap(ctx context.Context, llc *LowLevelClient, cdnInfo ngdp.CDNInfo, archiveHash ngdp.CDNHash) (map[ngdp.CDNHash]archiveIndexEntry, error) {
	// Retrieve the archive index.
	resp, err := llc.get(ctx, cdnInfo, ngdp.ContentTypeData, archiveHash, ".index")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	chunk := make([]byte, archiveIndexChunkSize)
	m := make(map[ngdp.CDNHash]archiveIndexEntry)
	for {
		// Read each chunk, one at a time.
		if _, err := io.ReadFull(resp.Body, chunk); err != nil {
			if err == io.ErrUnexpectedEOF || err == io.EOF {
				// We've reached the end of this archive.
				break
			}
			return nil, err
		}

	ChunkLoop:
		// Parse out each archive entry.
		for n := 0; n < archiveEntriesPerChunk; n++ {
			entry := chunk[n*0x18 : (n+1)*0x18]

			// Check if this entry contains any data.
			isAllZeros := true
			for x := 0; x < 0x18; x++ {
				if entry[x] != 0 {
					isAllZeros = false
					break
				}
			}
			if isAllZeros {
				// This entry has no data; read next chunk.
				break ChunkLoop
			}

			var cdnHash ngdp.CDNHash
			for n := 0; n < md5.Size; n++ {
				cdnHash[n] = entry[n]
			}
			size := binary.BigEndian.Uint32(entry[0x10:0x14])
			offset := binary.BigEndian.Uint32(entry[0x14:0x18])

			m[cdnHash] = archiveIndexEntry{
				archive: &archiveHash,
				size:    size,
				offset:  offset,
			}
		}
	}
	return m, nil
}

// NewArchiveMapper creates a new archive mapper from the provided set of archives.
func (llc *LowLevelClient) NewArchiveMapper(ctx context.Context, cdnInfo ngdp.CDNInfo, archives []ngdp.CDNHash) (*ArchiveMapper, error) {
	// Calculate required worker count.
	workerCount := archiveConcurrentIndexFetches
	if workerCount > len(archives) {
		workerCount = len(archives)
	}

	workChan := make(chan ngdp.CDNHash)
	resultChan := make(chan map[ngdp.CDNHash]archiveIndexEntry)
	g, ctx := errgroup.WithContext(ctx)

	// Enqueue work into workChan.
	g.Go(func() error {
		defer close(workChan)
		for _, archiveHash := range archives {
			select {
			case workChan <- archiveHash:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	// Fetch the archive indices.
	for n := 0; n < workerCount; n++ {
		g.Go(func() error {
			for archiveHash := range workChan {
				m, err := buildArchiveMap(ctx, llc, cdnInfo, archiveHash)
				if err != nil {
					return err
				}

				select {
				case resultChan <- m:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}

	// Signal main goroutine when all workers have finished.
	go func() {
		g.Wait()
		close(resultChan)
	}()

	// Process results.
	m := make(map[ngdp.CDNHash]archiveIndexEntry)
	for miniMap := range resultChan {
		for k, v := range miniMap {
			m[k] = v
		}
	}

	// Check if there was an error.
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &ArchiveMapper{m}, nil
}
