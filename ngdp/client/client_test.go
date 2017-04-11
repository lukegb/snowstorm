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

func (f *fakeGetter) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
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
		return ngdp.CDNHash{}, fmt.Errorf("response for %x not stored", contentHash)
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
	isInTest = true

	fg := &fakeGetter{
		responses: make(map[string]*http.Response),
	}

	fm := &fakeMapper{
		responses: make(map[ngdp.ContentHash]ngdp.CDNHash),
	}
	parseEncoding = func(r io.Reader) (mapper, error) {
		return fm, nil
	}
	fm.responses[ngdp.ContentHash{0xca, 0xfe, 0xbe, 0xef}] = ngdp.CDNHash{0xfe, 0xed, 0xbe, 0x11}
	fg.responses["http://region.distro.example.com/tpr/Hero-Live-a/data/fe/ed/feedbe11000000000000000000000000"] = fakeHTTPResponse(http.StatusOK, nil, "hooray!")

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
	fg.responses["http://region.distro.example.com/tpr/Hero-Live-a/config/ff/be/ffbec782d8a2222cbaf38f2968c7ba9c"] = fakeHTTPResponse(http.StatusOK, nil, `
# CDN Configuration

archives = 002b6d5f5f572534f80f1191fadcf199
patch-archives = 03619da1c909c7a4447f16ac7d093098
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

var (
	buildConfigCDNHash       = ngdp.CDNHash{0x46, 0xbb, 0xf4, 0x30, 0x43, 0x6c, 0xe4, 0x72, 0xd8, 0xb6, 0x81, 0x5b, 0x12, 0xe4, 0x75, 0x69}
	cdnConfigCDNHash         = ngdp.CDNHash{0xa4, 0xbe, 0xc7, 0x82, 0xd8, 0xa2, 0x22, 0x2c, 0xba, 0xf3, 0x8f, 0x29, 0x68, 0xc7, 0xba, 0x9c}
	sampleBuildConfigCDNHash = ngdp.CDNHash{0xff, 0xbb, 0xf4, 0x30, 0x43, 0x6c, 0xe4, 0x72, 0xd8, 0xb6, 0x81, 0x5b, 0x12, 0xe4, 0x75, 0x69}
	sampleCDNConfigCDNHash   = ngdp.CDNHash{0xff, 0xbe, 0xc7, 0x82, 0xd8, 0xa2, 0x22, 0x2c, 0xba, 0xf3, 0x8f, 0x29, 0x68, 0xc7, 0xba, 0x9c}
)

func TestVersions(t *testing.T) {
	c, _ := testClient()
	want := []VersionInfo{
		{"us", buildConfigCDNHash, cdnConfigCDNHash, 52008, "24.3.52008", ngdp.CDNHash{}},
		{"eu", buildConfigCDNHash, cdnConfigCDNHash, 52008, "24.3.52008", ngdp.CDNHash{}},
		{"cn", buildConfigCDNHash, cdnConfigCDNHash, 52008, "24.3.52008", ngdp.CDNHash{}},
		{"kr", buildConfigCDNHash, cdnConfigCDNHash, 52008, "24.3.52008", ngdp.CDNHash{}},
		{"tw", buildConfigCDNHash, cdnConfigCDNHash, 52008, "24.3.52008", ngdp.CDNHash{}},
		{"sg", buildConfigCDNHash, cdnConfigCDNHash, 52008, "24.3.52008", ngdp.CDNHash{}},
		{"region", sampleBuildConfigCDNHash, sampleCDNConfigCDNHash, 52008, "24.3.52008", ngdp.CDNHash{}},
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
	want := VersionInfo{"region", sampleBuildConfigCDNHash, sampleCDNConfigCDNHash, 52008, "24.3.52008", ngdp.CDNHash{}}

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
	want := BuildConfig{
		Root:         ngdp.ContentHash{0x56, 0x6c, 0xe1, 0x80, 0xfc, 0x2b, 0xf9, 0x8b, 0xfd, 0x3a, 0xf3, 0xa, 0x6a, 0xb8, 0x62, 0x75},
		Install:      ngdp.ContentHash{0xc9, 0xc0, 0xc7, 0xc1, 0x6b, 0x6b, 0xb, 0x63, 0x95, 0x26, 0x63, 0x76, 0x54, 0xae, 0x35, 0x9c},
		InstallSize:  0x9514,
		Download:     ngdp.ContentHash{0x26, 0x81, 0xd9, 0xf0, 0xb1, 0x4f, 0x66, 0x7a, 0xa4, 0x25, 0x36, 0x40, 0xc2, 0x3d, 0x67, 0x55},
		DownloadSize: 0x1248a59,
		Encoding: BuildConfigEncoding{
			ContentHash: ngdp.ContentHash{0xe0, 0xe1, 0xa4, 0x25, 0x72, 0x62, 0x10, 0xc7, 0x71, 0x58, 0xe7, 0x76, 0x36, 0xbb, 0x8d, 0x8f},
			CDNHash:     ngdp.CDNHash{0x15, 0x35, 0xa8, 0x25, 0xa3, 0x15, 0x36, 0x60, 0x39, 0x7b, 0x7f, 0xc3, 0x62, 0xdb, 0x63, 0x17}},
		EncodingSize: BuildConfigEncodingSize{
			UncompressedSize: 0x2ae566b,
			CompressedSize:   0x2ad9532},
		Patch:       ngdp.ContentHash{0xd7, 0xf, 0xb2, 0xd1, 0x52, 0xdd, 0x8d, 0xcc, 0x3, 0x96, 0xfe, 0x20, 0xd, 0x44, 0xb1, 0xc7},
		PatchSize:   0x178081,
		PatchConfig: ngdp.CDNHash{0x39, 0xa0, 0x47, 0x3f, 0x5b, 0x4, 0x38, 0x4b, 0x16, 0xb9, 0x23, 0x6d, 0xc7, 0x65, 0x3, 0x13}}

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

	body, err := c.Fetch(ngdp.ContentHash{0xca, 0xfe, 0xbe, 0xef})
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
