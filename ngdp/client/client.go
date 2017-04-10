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
	"fmt"
	"io"
	"net/http"

	"github.com/lukegb/snowstorm/blte"
	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/configtable"
	"github.com/lukegb/snowstorm/ngdp/encoding"
	"github.com/lukegb/snowstorm/ngdp/keyvalue"
)

// getter is used in tests for mocking out http.Client.
type getter interface {
	Get(url string) (*http.Response, error)
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

type Client struct {
	program ngdp.ProgramCode
	region  ngdp.Region

	client getter

	inited            bool
	cachedCDN         *CDNInfo
	cachedVersion     *VersionInfo
	cachedBuildConfig *BuildConfig
	cachedEncoding    mapper
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

// Init retrieves and caches a CDN and Version information for the currently selected region.
//
// If not called explicitly, it will be invoked automatically when necessary.
func (c *Client) Init() error {
	if c.inited {
		return nil
	}

	cdns, err := c.CDNs()
	if err != nil {
		return err
	}

	versions, err := c.Versions()
	if err != nil {
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

	r, err := c.fetchCDNHash(ngdp.ContentTypeConfig, version.BuildConfig)
	if err != nil {
		return fmt.Errorf("ngdp: downloading buildconfig: %v", err)
	}
	defer r.Close()
	var buildConfig BuildConfig
	if err := keyvalue.Decode(r, &buildConfig); err != nil {
		return fmt.Errorf("ngdp: parsing buildconfig: %v", err)
	}

	c.cachedBuildConfig = &buildConfig

	r, err = c.fetchCDNHash(ngdp.ContentTypeData, buildConfig.Encoding.CDNHash)
	if err != nil {
		return fmt.Errorf("ngdp: downloading encoding: %v", err)
	}
	defer r.Close()
	mapper, err := parseEncoding(r)
	if err != nil {
		return fmt.Errorf("ngdp: parsing encoding: %v", err)
	}

	c.inited = true
	c.cachedEncoding = mapper
	return nil
}

func (c *Client) cdnURLs(contentType ngdp.ContentType, cdnHash ngdp.CDNHash) ([]string, error) {
	urls := make([]string, len(c.cachedCDN.Hosts))
	for n, host := range c.cachedCDN.Hosts {
		urls[n] = fmt.Sprintf("http://%s/%s/%s/%s/%s/%s", host, c.cachedCDN.Path, contentType, cdnHash[0:2], cdnHash[2:4], cdnHash)
	}
	return urls, nil
}

func (c *Client) fetchCDNHash(contentType ngdp.ContentType, cdnHash ngdp.CDNHash) (io.ReadCloser, error) {
	urls, err := c.cdnURLs(contentType, cdnHash)
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

	return c.fetchCDNHash(ngdp.ContentTypeData, cdnHash)
}

func versionsDataURL(region ngdp.Region, program ngdp.ProgramCode) string {
	return fmt.Sprintf("http://%s.patch.battle.net:1119/%s/versions", region, program)
}

func cdnDataURL(region ngdp.Region, program ngdp.ProgramCode) string {
	return fmt.Sprintf("http://%s.patch.battle.net:1119/%s/cdns", region, program)
}
