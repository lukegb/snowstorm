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

	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/configtable"
)

var (
	suffixCDNs     = "cdns"
	suffixVersions = "versions"
)

// A LowLevelClient provides simple wrappers to make basic NGDP operations easier.
type LowLevelClient struct {
	Client *http.Client
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
