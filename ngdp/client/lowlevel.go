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

	"golang.org/x/sync/errgroup"

	"github.com/golang/glog"
	"github.com/lukegb/snowstorm/blte"
	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/configtable"
	"github.com/lukegb/snowstorm/ngdp/encoding"
	"github.com/lukegb/snowstorm/ngdp/keyvalue"
	"github.com/pkg/errors"
)

var (
	suffixCDNs     = "cdns"
	suffixVersions = "versions"
)

// A LowLevelClient provides simple wrappers to make basic NGDP operations easier.
type LowLevelClient struct {
	Client *http.Client
}

// Fetch retrieves a piece of data content by its CDNHash.
func (c *LowLevelClient) Fetch(ctx context.Context, cdnInfo ngdp.CDNInfo, cdnHash ngdp.CDNHash) (io.ReadCloser, error) {
	resp, err := c.get(ctx, cdnInfo, ngdp.ContentTypeData, cdnHash, "")
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
	}

	r := blte.NewReader(resp.Body)
	return newWrappedCloser(r, resp.Body), nil
}

func (c *LowLevelClient) get(ctx context.Context, cdnInfo ngdp.CDNInfo, contentType ngdp.ContentType, cdnHash ngdp.CDNHash, suffix string) (*http.Response, error) {

	req, err := http.NewRequest(http.MethodGet, cdnURL(cdnInfo, contentType, cdnHash, suffix), nil)
	if err != nil {
		return nil, err
	}

	return c.do(ctx, req)
}

func (c *LowLevelClient) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)

	cl := c.Client
	if cl == nil {
		cl = http.DefaultClient
	}

	return cl.Do(req)
}

func (c *LowLevelClient) cdns(ctx context.Context, program ngdp.ProgramCode, region ngdp.Region) ([]ngdp.CDNInfo, error) {
	req, err := http.NewRequest(http.MethodGet, patchURL(program, region, suffixCDNs), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
	}

	var cdns []ngdp.CDNInfo
	d := configtable.NewDecoder(resp.Body)
	for {
		var cdn ngdp.CDNInfo
		if err := d.Decode(&cdn); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		cdns = append(cdns, cdn)
	}
	return cdns, nil
}

func (c *LowLevelClient) versions(ctx context.Context, program ngdp.ProgramCode, region ngdp.Region) ([]ngdp.VersionInfo, error) {
	req, err := http.NewRequest(http.MethodGet, patchURL(program, region, suffixVersions), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
	}

	var versions []ngdp.VersionInfo
	d := configtable.NewDecoder(resp.Body)
	for {
		var version ngdp.VersionInfo
		if err := d.Decode(&version); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func cdnURL(cdnInfo ngdp.CDNInfo, contentType ngdp.ContentType, cdnHash ngdp.CDNHash, suffix string) string {
	return fmt.Sprintf("http://%s/%s/%s/%02x/%02x/%032x%s", cdnInfo.Hosts[0], cdnInfo.Path, contentType, cdnHash[0], cdnHash[1], cdnHash, suffix)
}

func patchURL(program ngdp.ProgramCode, region ngdp.Region, suffix string) string {
	return fmt.Sprintf("http://%s.patch.battle.net:1119/%s/%s", region, program, suffix)
}

func (c *LowLevelClient) CDN(ctx context.Context, program ngdp.ProgramCode, region ngdp.Region) (ngdp.CDNInfo, error) {
	cdns, err := c.cdns(ctx, program, region)
	if err != nil {
		return ngdp.CDNInfo{}, errors.Wrap(err, "retrieving CDN info")
	}

	for _, c := range cdns {
		if c.Name == region {
			return c, nil
		}
	}

	return ngdp.CDNInfo{}, ErrUnknownRegion
}

func (c *LowLevelClient) Version(ctx context.Context, program ngdp.ProgramCode, region ngdp.Region) (ngdp.VersionInfo, error) {
	versions, err := c.versions(ctx, program, region)
	if err != nil {
		return ngdp.VersionInfo{}, errors.Wrap(err, "retrieving version info")
	}

	for _, c := range versions {
		if c.Region == region {
			return c, nil
		}
	}

	return ngdp.VersionInfo{}, ErrUnknownRegion
}

func (c *LowLevelClient) BuildConfig(ctx context.Context, cdn ngdp.CDNInfo, version ngdp.VersionInfo) (ngdp.BuildConfig, error) {
	resp, err := c.get(ctx, cdn, ngdp.ContentTypeConfig, version.BuildConfig, "")
	if err != nil {
		return ngdp.BuildConfig{}, errors.Wrap(err, "retrieving build config")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ngdp.BuildConfig{}, errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
	}

	var buildConfig ngdp.BuildConfig
	if err := keyvalue.Decode(resp.Body, &buildConfig); err != nil {
		return ngdp.BuildConfig{}, errors.Wrap(err, "parsing build config")
	}

	return buildConfig, nil
}

func (c *LowLevelClient) CDNConfig(ctx context.Context, cdn ngdp.CDNInfo, version ngdp.VersionInfo) (ngdp.CDNConfig, error) {
	resp, err := c.get(ctx, cdn, ngdp.ContentTypeConfig, version.CDNConfig, "")
	if err != nil {
		return ngdp.CDNConfig{}, errors.Wrap(err, "retrieving cdn config")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ngdp.CDNConfig{}, errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
	}

	var cdnConfig ngdp.CDNConfig
	if err := keyvalue.Decode(resp.Body, &cdnConfig); err != nil {
		return ngdp.CDNConfig{}, errors.Wrap(err, "parsing cdn config")
	}

	return cdnConfig, nil
}

func (c *LowLevelClient) EncodingTable(ctx context.Context, cdn ngdp.CDNInfo, encodingHash ngdp.CDNHash) (*encoding.Mapper, error) {
	resp, err := c.get(ctx, cdn, ngdp.ContentTypeData, encodingHash, "")
	if err != nil {
		return nil, errors.Wrap(err, "downloading encoding table")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errBadStatus{resp.StatusCode, resp.Status, http.StatusOK}
	}

	mapper, err := encoding.NewMapper(blte.NewReader(resp.Body))
	if err != nil {
		return nil, errors.Wrap(err, "parsing encoding table")
	}
	return mapper, nil
}

func (c *LowLevelClient) ArchiveMapper(ctx context.Context, cdn ngdp.CDNInfo, archives []ngdp.CDNHash) (*ArchiveMapper, error) {
	am, err := c.NewArchiveMapper(ctx, cdn, archives)
	if err != nil {
		return nil, errors.Wrap(err, "building archive mapper")
	}
	return am, nil
}

func (c *LowLevelClient) Info(ctx context.Context, program ngdp.ProgramCode, region ngdp.Region) (ngdp.CDNInfo, ngdp.VersionInfo, error) {
	var cdn ngdp.CDNInfo
	var version ngdp.VersionInfo
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		glog.Info("Retrieving CDN info")
		var err error
		cdn, err = c.CDN(ctx, program, region)
		return err
	})
	g.Go(func() error {
		glog.Info("Retrieving version info")
		var err error
		version, err = c.Version(ctx, program, region)
		return err
	})
	if err := g.Wait(); err != nil {
		return ngdp.CDNInfo{}, ngdp.VersionInfo{}, err
	}
	return cdn, version, nil
}

func (c *LowLevelClient) Configs(ctx context.Context, cdn ngdp.CDNInfo, version ngdp.VersionInfo) (ngdp.CDNConfig, ngdp.BuildConfig, error) {
	var cdnConfig ngdp.CDNConfig
	var buildConfig ngdp.BuildConfig
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		glog.Info("Retrieving build config")
		var err error
		buildConfig, err = c.BuildConfig(ctx, cdn, version)
		return err
	})
	g.Go(func() error {
		glog.Info("Retrieving CDN config")
		var err error
		cdnConfig, err = c.CDNConfig(ctx, cdn, version)
		return err
	})
	if err := g.Wait(); err != nil {
		return ngdp.CDNConfig{}, ngdp.BuildConfig{}, err
	}
	return cdnConfig, buildConfig, nil
}

func (c *LowLevelClient) Mappers(ctx context.Context, cdn ngdp.CDNInfo, cdnConfig ngdp.CDNConfig, buildConfig ngdp.BuildConfig) (*encoding.Mapper, *ArchiveMapper, error) {
	var encodingMapper *encoding.Mapper
	var archiveMapper *ArchiveMapper
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		glog.Info("Downloading encoding table")
		var err error
		encodingMapper, err = c.EncodingTable(ctx, cdn, buildConfig.Encoding.CDNHash)
		return err
	})
	g.Go(func() error {
		glog.Info("Building archive mapper")
		var err error
		archiveMapper, err = c.ArchiveMapper(ctx, cdn, cdnConfig.Archives)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	return encodingMapper, archiveMapper, nil
}
