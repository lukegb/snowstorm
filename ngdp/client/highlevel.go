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
	"golang.org/x/sync/errgroup"

	"github.com/lukegb/snowstorm/blte"
	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/encoding"
	"github.com/lukegb/snowstorm/ngdp/keyvalue"
)

var (
	ErrUnknownRegion  = errors.New("client: unknown region")
	ErrUnknownProgram = errors.New("client: unknown program")

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
func New(octx context.Context, program ngdp.ProgramCode, region ngdp.Region) (*Client, error) {
	glog.Info("Initialising new NGDP Client")
	llc := &LowLevelClient{}

	// Fetch CDN and Version info.
	var cdn ngdp.CDNInfo
	var version ngdp.VersionInfo
	g, ctx := errgroup.WithContext(octx)
	g.Go(func() error {
		glog.Info("Retrieving CDN info")
		cdns, err := llc.cdns(ctx, program, region)
		if err != nil {
			return errors.Wrap(err, "retrieving CDN info")
		}
		for _, c := range cdns {
			if c.Name == region {
				cdn = c
				return nil
			}
		}
		return ErrUnknownRegion
	})
	g.Go(func() error {
		glog.Info("Retrieving version info")
		versions, err := llc.versions(ctx, program, region)
		if err != nil {
			return errors.Wrap(err, "retrieving version info")
		}
		for _, v := range versions {
			if v.Region == region {
				version = v
				return nil
			}
		}
		return ErrUnknownRegion
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Fetch Build and CDN configs.
	var buildConfig ngdp.BuildConfig
	var cdnConfig ngdp.CDNConfig
	g, ctx = errgroup.WithContext(octx)
	g.Go(func() error {
		glog.Info("Retrieving build config")
		resp, err := llc.get(ctx, cdn, ngdp.ContentTypeConfig, version.BuildConfig, "")
		if err != nil {
			return errors.Wrap(err, "retrieving build config")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
		}

		glog.Info("Parsing build config")
		if err := keyvalue.Decode(resp.Body, &buildConfig); err != nil {
			return errors.Wrap(err, "parsing build config")
		}

		return nil
	})
	g.Go(func() error {
		glog.Info("Retrieving CDN config")
		resp, err := llc.get(ctx, cdn, ngdp.ContentTypeConfig, version.CDNConfig, "")
		if err != nil {
			return errors.Wrap(err, "retrieving cdn config")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
		}

		glog.Info("Parsing CDN config")
		if err := keyvalue.Decode(resp.Body, &cdnConfig); err != nil {
			return errors.Wrap(err, "parsing cdn config")
		}

		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Build encoding and archive mappers.
	g, ctx = errgroup.WithContext(octx)
	var archiveMapper *ArchiveMapper
	var encodingMapper *encoding.Mapper
	g.Go(func() error {
		glog.Info("Downloading encoding table")
		resp, err := llc.get(ctx, cdn, ngdp.ContentTypeData, buildConfig.Encoding.CDNHash, "")
		if err != nil {
			return errors.Wrap(err, "downloading encoding table")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
		}

		glog.Info("Parsing encoding table")
		mapper, err := encoding.NewMapper(blte.NewReader(resp.Body))
		if err != nil {
			return errors.Wrap(err, "parsing encoding table")
		}
		glog.Info("Done building encoding mapper")

		encodingMapper = mapper
		return nil
	})
	g.Go(func() error {
		glog.Info("Building archive mapper")
		am, err := llc.NewArchiveMapper(ctx, cdn, cdnConfig.Archives)
		if err != nil {
			return errors.Wrap(err, "building archive mapper")
		}
		glog.Info("Done building archive mapper")

		archiveMapper = am
		return nil
	})
	if err := g.Wait(); err != nil {
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
