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
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/lukegb/snowstorm/blte"
	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/configtable"
	"github.com/lukegb/snowstorm/ngdp/encoding"
	"github.com/lukegb/snowstorm/ngdp/keyvalue"
)

// getter is used in tests for mocking out http.Client.
type getter interface {
	Get(url string) (*http.Response, error)
	Do(req *http.Request) (*http.Response, error)
}

// mapper is used in tests for mocking out encoding.Mapper.
type mapper interface {
	ToCDNHash(ngdp.ContentHash) (ngdp.CDNHash, error)
}

var (
	// parseEncoding is used to stub out the encoding parser in tests
	parseEncoding = func(r io.Reader) (mapper, error) {
		return encoding.NewMapper(blte.NewReader(r))
	}

	// XXX(lukegb): remove this bodge of a hack
	isInTest = false
)

var (
	ErrUnknownRegion = fmt.Errorf("ngdp: specified region is unknown for this product")
)

type CDNInfo struct {
	Name       ngdp.Region
	Path       string
	Hosts      []string
	ConfigPath string // unknown purpose
}

type VersionInfo struct {
	Region        ngdp.Region
	BuildConfig   ngdp.CDNHash
	CDNConfig     ngdp.CDNHash
	BuildID       int `configtable:"BuildId"`
	VersionsName  string
	ProductConfig ngdp.CDNHash
}

type BuildConfigEncoding struct {
	ContentHash ngdp.ContentHash
	CDNHash     ngdp.CDNHash
}

type BuildConfigEncodingSize struct {
	UncompressedSize uint64
	CompressedSize   uint64
}

type BuildConfig struct {
	Root ngdp.ContentHash

	Install     ngdp.ContentHash
	InstallSize uint64

	Download     ngdp.ContentHash
	DownloadSize uint64

	Encoding     BuildConfigEncoding
	EncodingSize BuildConfigEncodingSize

	Patch       ngdp.ContentHash
	PatchSize   uint64
	PatchConfig ngdp.CDNHash
}

type CDNConfig struct {
	Archives     []ngdp.CDNHash
	ArchiveGroup ngdp.CDNHash

	PatchArchives     []ngdp.CDNHash
	PatchArchiveGroup ngdp.CDNHash
}

type Client struct {
	program ngdp.ProgramCode
	region  ngdp.Region

	client getter

	inited               bool
	cachedCDN            *CDNInfo
	cachedVersion        *VersionInfo
	cachedBuildConfig    *BuildConfig
	cachedCDNConfig      *CDNConfig
	cachedEncoding       mapper
	cachedArchiveIndices map[ngdp.CDNHash]archiveIndexEntry
}

type archiveIndexEntry struct {
	fileCDNHash    ngdp.CDNHash
	archiveCDNHash ngdp.CDNHash
	size           uint32
	offset         uint32
}

// New creates a new Client for the given program using the default region.
func New(program ngdp.ProgramCode) *Client {
	return &Client{
		program: program,
		region:  ngdp.DefaultRegion,
		client:  http.DefaultClient,
	}
}

// CDNs returns information about the available CDNs.
func (c *Client) CDNs() ([]CDNInfo, error) {
	resp, err := c.client.Get(cdnDataURL(c.region, c.program))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CDNs server response: %s", resp.Status)
	}

	var cdns []CDNInfo
	d := configtable.NewDecoder(resp.Body)
	for {
		var cdn CDNInfo
		if err := d.Decode(&cdn); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		cdns = append(cdns, cdn)
	}
	return cdns, nil
}

// Versions returns information about the currently deployed version for each region.
func (c *Client) Versions() ([]VersionInfo, error) {
	resp, err := c.client.Get(versionsDataURL(c.region, c.program))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("versions server response: %s", resp.Status)
	}

	var versions []VersionInfo
	d := configtable.NewDecoder(resp.Body)
	for {
		var version VersionInfo
		if err := d.Decode(&version); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

// Version returns information about the current version for the currently selected region.
func (c *Client) Version() (VersionInfo, error) {
	if err := c.Init(); err != nil {
		return VersionInfo{}, err
	}

	return *c.cachedVersion, nil
}

// BuildConfig returns information about the current build config for the currently selected region.
func (c *Client) BuildConfig() (BuildConfig, error) {
	if err := c.Init(); err != nil {
		return BuildConfig{}, err
	}

	return *c.cachedBuildConfig, nil
}

// CDNConfig returns information about the current build config for the currently selected region.
func (c *Client) CDNConfig() (CDNConfig, error) {
	if err := c.Init(); err != nil {
		return CDNConfig{}, err
	}

	return *c.cachedCDNConfig, nil
}

// Init retrieves and caches a CDN and Version information for the currently selected region.
//
// If not called explicitly, it will be invoked automatically when necessary.
func (c *Client) Init() error {
	if c.inited {
		return nil
	}

	var eg errgroup.Group

	var cdns []CDNInfo
	var versions []VersionInfo

	eg.Go(func() error {
		var err error
		cdns, err = c.CDNs()
		return err
	})
	eg.Go(func() error {
		var err error
		versions, err = c.Versions()
		return err
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	var cdn *CDNInfo
	for _, ccdn := range cdns {
		if ccdn.Name != c.region {
			continue
		}
		cdn = &ccdn
		break
	}

	var version *VersionInfo
	for _, cversion := range versions {
		if cversion.Region != c.region {
			continue
		}
		version = &cversion
		break
	}

	if cdn == nil || version == nil {
		return ErrUnknownRegion
	}

	c.cachedCDN = cdn
	c.cachedVersion = version

	var buildConfig BuildConfig
	var cdnConfig CDNConfig
	var mapper mapper
	eg.Go(func() error {
		r, err := c.fetchCDNHash(ngdp.ContentTypeConfig, version.BuildConfig)
		if err != nil {
			return fmt.Errorf("ngdp: downloading buildconfig: %v", err)
		}
		defer r.Close()
		if err := keyvalue.Decode(r, &buildConfig); err != nil {
			return fmt.Errorf("ngdp: parsing buildconfig: %v", err)
		}
		return nil
	})
	eg.Go(func() error {
		r, err := c.fetchCDNHash(ngdp.ContentTypeConfig, version.CDNConfig)
		if err != nil {
			return fmt.Errorf("ngdp: downloading cdnconfig: %v", err)
		}
		defer r.Close()
		if err := keyvalue.Decode(r, &cdnConfig); err != nil {
			return fmt.Errorf("ngdp: parsing cdnconfig: %v", err)
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	eg.Go(func() error {
		r, err := c.fetchCDNHash(ngdp.ContentTypeData, buildConfig.Encoding.CDNHash)
		if err != nil {
			return fmt.Errorf("ngdp: downloading encoding: %v", err)
		}
		defer r.Close()
		mapper, err = parseEncoding(r)
		if err != nil {
			return fmt.Errorf("ngdp: parsing encoding: %v", err)
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	c.inited = true
	c.cachedEncoding = mapper
	c.cachedBuildConfig = &buildConfig
	c.cachedCDNConfig = &cdnConfig

	if err := c.initArchiveIndices(); err != nil {
		return err
	}

	return nil
}

// TODO(lukegb): move this somewhere more appropriate
func (c *Client) initArchiveIndices() error {
	if isInTest {
		return nil
	}

	var locMapLock sync.Mutex
	locMap := make(map[ngdp.CDNHash]archiveIndexEntry)

	var g errgroup.Group

	for _, archiveCDNHash := range c.cachedCDNConfig.Archives {
		archiveCDNHash := archiveCDNHash
		g.Go(func() error {
			r, err := c.fetchCDNHashWithSuffix(ngdp.ContentTypeData, archiveCDNHash, ".index")
			if err != nil {
				return err
			}
			defer r.Close()

			chunk := make([]byte, 4096)
			for {
				if _, err := io.ReadFull(r, chunk); err != nil {
					if err == io.ErrUnexpectedEOF {
						return nil
					}
					return err
				}

				locMapLock.Lock()

				for n := 0; n < 170; n++ {
					hdr := chunk[n*0x18 : (n+1)*0x18]

					// check if we're at the end
					isAllZeros := true
					for x := 0; x < 0x18; x++ {
						if hdr[x] != 0 {
							isAllZeros = false
							break
						}
					}
					if isAllZeros {
						break
					}

					cdnHash := ngdp.CDNHash(fmt.Sprintf("%0x", hdr[0x0:0x10]))
					size := binary.BigEndian.Uint32(hdr[0x10:0x14])
					offset := binary.BigEndian.Uint32(hdr[0x14:0x18])

					locMap[cdnHash] = archiveIndexEntry{
						fileCDNHash:    cdnHash,
						archiveCDNHash: archiveCDNHash,
						size:           size,
						offset:         offset,
					}
				}

				locMapLock.Unlock()
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	c.cachedArchiveIndices = locMap

	return nil
}

func (c *Client) cdnURLs(contentType ngdp.ContentType, cdnHash ngdp.CDNHash, suffix string) ([]string, error) {
	urls := make([]string, len(c.cachedCDN.Hosts))
	for n, host := range c.cachedCDN.Hosts {
		urls[n] = fmt.Sprintf("http://%s/%s/%s/%s/%s/%s%s", host, c.cachedCDN.Path, contentType, cdnHash[0:2], cdnHash[2:4], cdnHash, suffix)
	}
	return urls, nil
}

func (c *Client) fetchCDNHash(contentType ngdp.ContentType, cdnHash ngdp.CDNHash) (io.ReadCloser, error) {
	return c.fetchCDNHashWithSuffix(contentType, cdnHash, "")
}

func (c *Client) fetchCDNHashWithSuffix(contentType ngdp.ContentType, cdnHash ngdp.CDNHash, suffix string) (io.ReadCloser, error) {
	urls, err := c.cdnURLs(contentType, cdnHash, suffix)
	if err != nil {
		return nil, err
	}

	// Just take the first URL for now.
	// XXX(lukegb): investigate a better strategy maybe?
	url := urls[0]
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ngdq: server returned status: %v", resp.Status)
	}

	return resp.Body, nil
}

// Fetch retrieves a data file by its content hash.
//
// If necessary, the client will download and cache the index file.
// It is the caller's responsibility to close the returned io.ReadCloser.
func (c *Client) Fetch(h ngdp.ContentHash) (io.ReadCloser, error) {
	if err := c.Init(); err != nil {
		return nil, err
	}

	cdnHash, err := c.cachedEncoding.ToCDNHash(h)
	if err != nil {
		return nil, err
	}

	// check to see if this is contained inside an archive
	if archEntry, ok := c.cachedArchiveIndices[cdnHash]; ok {
		urls, err := c.cdnURLs(ngdp.ContentTypeData, archEntry.archiveCDNHash, "")
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest("GET", urls[0], nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", archEntry.offset, archEntry.offset+archEntry.size))

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusPartialContent {
			return nil, fmt.Errorf("ngdq: server returned status: %v", resp.Status)
		}

		return resp.Body, nil
	}

	return c.fetchCDNHash(ngdp.ContentTypeData, cdnHash)
}

func versionsDataURL(region ngdp.Region, program ngdp.ProgramCode) string {
	return fmt.Sprintf("http://%s.patch.battle.net:1119/%s/versions", region, program)
}

func cdnDataURL(region ngdp.Region, program ngdp.ProgramCode) string {
	return fmt.Sprintf("http://%s.patch.battle.net:1119/%s/cdns", region, program)
}
