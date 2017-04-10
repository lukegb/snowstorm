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
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/lukegb/snowstorm/ngdp"
)

type fakeGetter struct {
	responses map[string]*http.Response
	requests  []string
}

func (f *fakeGetter) Get(url string) (*http.Response, error) {
	f.requests = append(f.requests, url)

	resp := f.responses[url]
	if resp == nil {
		return nil, fmt.Errorf("response for %q not stored", url)
	}

	return resp, nil
}

type fakeMapper struct {
	responses map[ngdp.ContentHash]ngdp.CDNHash
	requests  []ngdp.ContentHash
}

func (f *fakeMapper) ToCDNHash(contentHash ngdp.ContentHash) (ngdp.CDNHash, error) {
	f.requests = append(f.requests, contentHash)

	resp, ok := f.responses[contentHash]
	if !ok {
		return ngdp.CDNHash(""), fmt.Errorf("response for %x not stored", contentHash)
	}

	return resp, nil
}

func fakeHTTPResponse(statusCode int, headers map[string]string, body string) *http.Response {
	hdrs := make(http.Header)
	hdrs.Set("Server", "Protocol HTTP")
	hdrs.Set("Content-Type", "application/octet-stream")
	hdrs.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	for k, v := range headers {
		hdrs.Set(k, v)
	}

	return &http.Response{
		Status:     http.StatusText(statusCode),
		StatusCode: statusCode,
		Header:     hdrs,
		Body:       ioutil.NopCloser(strings.NewReader(body)),
	}
}

func TestNewClient(t *testing.T) {
	want := &Client{
		program: ngdp.ProgramHotS,
		region:  ngdp.DefaultRegion,
		client:  http.DefaultClient,
	}

	got := New(ngdp.ProgramHotS)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NewClient = %#v; want %#v", got, want)
	}
}

func testClient() (*Client, *fakeGetter) {
	fg := &fakeGetter{
		responses: make(map[string]*http.Response),
	}

	fm := &fakeMapper{
		responses: make(map[ngdp.ContentHash]ngdp.CDNHash),
	}
	parseEncoding = func(r io.Reader) (mapper, error) {
		return fm, nil
	}
	fm.responses["contenthash"] = "cdnhash"
	fg.responses["http://region.distro.example.com/tpr/Hero-Live-a/data/cd/nh/cdnhash"] = fakeHTTPResponse(http.StatusOK, nil, "hooray!")

	fg.responses["http://region.patch.battle.net:1119/program/cdns"] = fakeHTTPResponse(http.StatusOK, nil, `Name!STRING:0|Path!STRING:0|Hosts!STRING:0|ConfigPath!STRING:0
us|tpr/Hero-Live-a|blzddist1-a.akamaihd.net level3.blizzard.com|tpr/configs/data
tw|tpr/Hero-Live-a|blzddist1-a.akamaihd.net level3.blizzard.com|tpr/configs/data
eu|tpr/Hero-Live-a|blzddist1-a.akamaihd.net level3.blizzard.com|tpr/configs/data
kr|tpr/Hero-Live-a|blzddistkr1-a.akamaihd.net blizzard.nefficient.co.kr blzddist1-a.akamaihd.net|tpr/configs/data
cn|tpr/Hero-Live-a|client02.pdl.wow.battlenet.com.cn client03.pdl.wow.battlenet.com.cn blzddist1-a.akamaihd.net client01.pdl.wow.battlenet.com.cn client04.pdl.wow.battlenet.com.cn|tpr/configs/data
region|tpr/Hero-Live-a|region.distro.example.com|tpr/configs/data
`)
	fg.responses["http://region.patch.battle.net:1119/program/versions"] = fakeHTTPResponse(http.StatusOK, nil, `Region!STRING:0|BuildConfig!HEX:16|CDNConfig!HEX:16|KeyRing!HEX:16|BuildId!DEC:4|VersionsName!String:0|ProductConfig!HEX:16
us|46bbf430436ce472d8b6815b12e47569|a4bec782d8a2222cbaf38f2968c7ba9c||52008|24.3.52008|
eu|46bbf430436ce472d8b6815b12e47569|a4bec782d8a2222cbaf38f2968c7ba9c||52008|24.3.52008|
cn|46bbf430436ce472d8b6815b12e47569|a4bec782d8a2222cbaf38f2968c7ba9c||52008|24.3.52008|
kr|46bbf430436ce472d8b6815b12e47569|a4bec782d8a2222cbaf38f2968c7ba9c||52008|24.3.52008|
tw|46bbf430436ce472d8b6815b12e47569|a4bec782d8a2222cbaf38f2968c7ba9c||52008|24.3.52008|
sg|46bbf430436ce472d8b6815b12e47569|a4bec782d8a2222cbaf38f2968c7ba9c||52008|24.3.52008|
region|ffbbf430436ce472d8b6815b12e47569|ffbec782d8a2222cbaf38f2968c7ba9c||52008|24.3.52008|
`)
	fg.responses["http://region.distro.example.com/tpr/Hero-Live-a/config/ff/bb/ffbbf430436ce472d8b6815b12e47569"] = fakeHTTPResponse(http.StatusOK, nil, `
# Build Configuration

root = 566ce180fc2bf98bfd3af30a6ab86275
install = c9c0c7c16b6b0b639526637654ae359c
install-size = 38164
download = 2681d9f0b14f667aa4253640c23d6755
download-size = 19171929
encoding = e0e1a425726210c77158e77636bb8d8f 1535a825a3153660397b7fc362db6317
encoding-size = 44979819 44930354
patch = d70fb2d152dd8dcc0396fe200d44b1c7
patch-size = 1540225
patch-config = 39a0473f5b04384b16b9236dc7650313
build-fixed-hash = f569d5d78441d07548c1239b52ded63d
build-name = B52008
build-product = Hero
build-replay-hash = c0fd005923f9476b00c99475a07c08c6
build-t1-manifest-version = 2
build-uid = hero
`)
	fg.responses["http://region.distro.example.com/tpr/Hero-Live-a/data/15/35/1535a825a3153660397b7fc362db6317"] = fakeHTTPResponse(http.StatusOK, nil, "")

	return &Client{
		program: ngdp.ProgramCode("program"),
		region:  ngdp.Region("region"),
		client:  fg,
	}, fg
}

func TestCDNs(t *testing.T) {
	c, _ := testClient()
	want := []CDNInfo{
		{"us", "tpr/Hero-Live-a", []string{"blzddist1-a.akamaihd.net", "level3.blizzard.com"}, "tpr/configs/data"},
		{"tw", "tpr/Hero-Live-a", []string{"blzddist1-a.akamaihd.net", "level3.blizzard.com"}, "tpr/configs/data"},
		{"eu", "tpr/Hero-Live-a", []string{"blzddist1-a.akamaihd.net", "level3.blizzard.com"}, "tpr/configs/data"},
		{"kr", "tpr/Hero-Live-a", []string{"blzddistkr1-a.akamaihd.net", "blizzard.nefficient.co.kr", "blzddist1-a.akamaihd.net"}, "tpr/configs/data"},
		{"cn", "tpr/Hero-Live-a", []string{"client02.pdl.wow.battlenet.com.cn", "client03.pdl.wow.battlenet.com.cn", "blzddist1-a.akamaihd.net", "client01.pdl.wow.battlenet.com.cn", "client04.pdl.wow.battlenet.com.cn"}, "tpr/configs/data"},
		{"region", "tpr/Hero-Live-a", []string{"region.distro.example.com"}, "tpr/configs/data"},
	}

	got, err := c.CDNs()
	if err != nil {
		t.Errorf("CDNs: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CDNs = %#v; want %#v", got, want)
	}
}

func TestCDNsGetError(t *testing.T) {
	for _, test := range []*http.Response{
		nil,
		fakeHTTPResponse(http.StatusNotFound, nil, "not found"),
		fakeHTTPResponse(http.StatusOK, nil, "Invalid"),
	} {
		c, fg := testClient()
		fg.responses["http://region.patch.battle.net:1119/program/cdns"] = test
		_, err := c.CDNs()
		if err == nil {
			t.Errorf("CDNs: %v; want error", err)
		}
	}
}

func TestVersions(t *testing.T) {
	c, _ := testClient()
	want := []VersionInfo{
		{"us", "46bbf430436ce472d8b6815b12e47569", "a4bec782d8a2222cbaf38f2968c7ba9c", 52008, "24.3.52008", ""},
		{"eu", "46bbf430436ce472d8b6815b12e47569", "a4bec782d8a2222cbaf38f2968c7ba9c", 52008, "24.3.52008", ""},
		{"cn", "46bbf430436ce472d8b6815b12e47569", "a4bec782d8a2222cbaf38f2968c7ba9c", 52008, "24.3.52008", ""},
		{"kr", "46bbf430436ce472d8b6815b12e47569", "a4bec782d8a2222cbaf38f2968c7ba9c", 52008, "24.3.52008", ""},
		{"tw", "46bbf430436ce472d8b6815b12e47569", "a4bec782d8a2222cbaf38f2968c7ba9c", 52008, "24.3.52008", ""},
		{"sg", "46bbf430436ce472d8b6815b12e47569", "a4bec782d8a2222cbaf38f2968c7ba9c", 52008, "24.3.52008", ""},
		{"region", "ffbbf430436ce472d8b6815b12e47569", "ffbec782d8a2222cbaf38f2968c7ba9c", 52008, "24.3.52008", ""},
	}

	got, err := c.Versions()
	if err != nil {
		t.Errorf("Versions: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Versions = %#v; want %#v", got, want)
	}
}

func TestVersionsGetError(t *testing.T) {
	for _, test := range []*http.Response{
		nil,
		fakeHTTPResponse(http.StatusNotFound, nil, "not found"),
		fakeHTTPResponse(http.StatusOK, nil, "Invalid"),
	} {
		c, fg := testClient()
		fg.responses["http://region.patch.battle.net:1119/program/versions"] = test
		_, err := c.Versions()
		if err == nil {
			t.Errorf("Versions: %v; want error", err)
		}
	}
}

func TestVersion(t *testing.T) {
	c, _ := testClient()
	want := VersionInfo{"region", "ffbbf430436ce472d8b6815b12e47569", "ffbec782d8a2222cbaf38f2968c7ba9c", 52008, "24.3.52008", ""}

	got, err := c.Version()
	if err != nil {
		t.Errorf("Version: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Version = %#v; want %#v", got, want)
	}
}

func TestVersionError(t *testing.T) {
	c, _ := testClient()
	parseEncoding = func(r io.Reader) (mapper, error) {
		return nil, fmt.Errorf("blah")
	}

	_, err := c.Version()
	if err == nil {
		t.Errorf("Version: %v; want error", err)
	}
}

func TestBuildConfig(t *testing.T) {
	c, _ := testClient()
	want := BuildConfig{Root: "566ce180fc2bf98bfd3af30a6ab86275", Install: "c9c0c7c16b6b0b639526637654ae359c", InstallSize: 0x9514, Download: "2681d9f0b14f667aa4253640c23d6755", DownloadSize: 0x1248a59, Encoding: BuildConfigEncoding{ContentHash: "e0e1a425726210c77158e77636bb8d8f", CDNHash: "1535a825a3153660397b7fc362db6317"}, EncodingSize: BuildConfigEncodingSize{UncompressedSize: 0x2ae566b, CompressedSize: 0x2ad9532}, Patch: "d70fb2d152dd8dcc0396fe200d44b1c7", PatchSize: 0x178081, PatchConfig: "39a0473f5b04384b16b9236dc7650313"}

	got, err := c.BuildConfig()
	if err != nil {
		t.Errorf("BuildConfig: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildConfig = %#v; want %#v", got, want)
	}
}

func TestBuildConfigError(t *testing.T) {
	c, _ := testClient()
	parseEncoding = func(r io.Reader) (mapper, error) {
		return nil, fmt.Errorf("blah")
	}

	_, err := c.BuildConfig()
	if err == nil {
		t.Errorf("BuildConfig: %v; want error", err)
	}
}

func TestFetch(t *testing.T) {
	c, _ := testClient()

	body, err := c.Fetch("contenthash")
	if err != nil {
		t.Errorf("Fetch: %v", err)
	}
	defer body.Close()

	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		t.Errorf("ReadAll: %v", err)
	}

	want := []byte("hooray!")
	if !reflect.DeepEqual(bodyBytes, want) {
		t.Errorf("Fetch returned %q; want %q", bodyBytes, want)
	}
}
