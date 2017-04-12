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
	"fmt"
	"io"
	"net/http"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/lukegb/snowstorm/blte"
	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/encoding"
)

var (
	// ErrUnknownRegion means that the region is invalid.
	ErrUnknownRegion = errors.New("client: unknown region")

	// ErrUnknownProgram means that the supplied program code does not refer to a
	// currently available Blizzard program.
	ErrUnknownProgram = errors.New("client: unknown program")

	// ErrNoFilenameMapper means that the Client has no FilenameMapper assigned to it.
	// FilenameMappers are program specific and must be added after calling client.New().
	ErrNoFilenameMapper = errors.New("client: no filename mapper registered")
)

type errBadStatus struct {
	statusCode int
	status     string

	wantedStatusCode int
}

func (e errBadStatus) Error() string {
	return fmt.Sprintf("client: server status was \"%d %s\"; wanted \"%d %s\"", e.statusCode, e.status, e.wantedStatusCode, http.StatusText(e.wantedStatusCode))
}

// A Client provides a nice interface to interacting with NGDP, to make retrieving individual files easy.
type Client struct {
	LowLevelClient *LowLevelClient

	CDNInfo     *ngdp.CDNInfo
	VersionInfo *ngdp.VersionInfo

	BuildConfig *ngdp.BuildConfig
	CDNConfig   *ngdp.CDNConfig

	ArchiveMapper  *ArchiveMapper
	EncodingMapper *encoding.Mapper
	FilenameMapper ngdp.FilenameMapper
}

// New creates a new Client for the given ProgramCode and Region.
//
// It will automatically create an ArchiveMapper and Encoder as appropriate.
func New(ctx context.Context, program ngdp.ProgramCode, region ngdp.Region) (*Client, error) {
	glog.Info("Initialising new NGDP Client")
	llc := &LowLevelClient{}

	// Fetch CDN and Version info.
	cdn, version, err := llc.Info(ctx, program, region)
	if err != nil {
		return nil, err
	}

	// Fetch Build and CDN configs.
	cdnConfig, buildConfig, err := llc.Configs(ctx, cdn, version)
	if err != nil {
		return nil, err
	}

	// Build encoding and archive mappers.
	encodingMapper, archiveMapper, err := llc.Mappers(ctx, cdn, cdnConfig, buildConfig)
	if err != nil {
		return nil, err
	}

	return &Client{
		LowLevelClient: llc,

		CDNInfo:     &cdn,
		VersionInfo: &version,

		BuildConfig: &buildConfig,
		CDNConfig:   &cdnConfig,

		ArchiveMapper:  archiveMapper,
		EncodingMapper: encodingMapper,
	}, nil
}

// Fetch retrieves a given file by the hash of its contents. After all, CASC is content-addressable storage.
func (c *Client) Fetch(ctx context.Context, h ngdp.ContentHash) (io.ReadCloser, error) {
	// Convert the content hash to a CDN hash.
	cdnHash, err := c.EncodingMapper.ToCDNHash(h)
	if err != nil {
		return nil, err
	}

	// Check to see if this is inside an archive.
	var resp *http.Response
	if entry, ok := c.ArchiveMapper.Map(cdnHash); ok {
		// We're inside an archive - make a Range request.
		req, err := http.NewRequest(http.MethodGet, cdnURL(*c.CDNInfo, ngdp.ContentTypeData, entry.Archive, ""), nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", entry.Offset, entry.Offset+entry.Size))

		resp, err = c.LowLevelClient.do(ctx, req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusPartialContent {
			return nil, errBadStatus{resp.StatusCode, resp.Status, http.StatusPartialContent}
		}
	} else {
		// We're not inside an archive, make a normal request.
		resp, err = c.LowLevelClient.get(ctx, *c.CDNInfo, ngdp.ContentTypeData, cdnHash, "")
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
		}
	}

	// Run the content through the BLTE decoder. It deserves it.
	r := blte.NewReader(resp.Body)
	return newWrappedCloser(r, resp.Body), nil
}

// FetchFilename retrieves a given file by its filename.
//
// FetchFilename requires that a FilenameMapper has been registered.
// For Heroes of the Storm, mndx.Decorate can be used to register an appropriate mapper.
func (c *Client) FetchFilename(ctx context.Context, fn string) (io.ReadCloser, error) {
	if c.FilenameMapper == nil {
		return nil, ErrNoFilenameMapper
	}

	h, ok := c.FilenameMapper.ToContentHash(fn)
	if !ok {
		return nil, fmt.Errorf("client: no such file: %v", fn)
	}

	return c.Fetch(ctx, h)
}
