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

package ngdp

import "crypto/md5"

type CDNHash [md5.Size]byte
type ContentHash [md5.Size]byte

type CDNInfo struct {
	Name       Region
	Path       string
	Hosts      []string
	ConfigPath string // unknown purpose
}

type VersionInfo struct {
	Region        Region
	BuildConfig   CDNHash
	CDNConfig     CDNHash
	BuildID       int `configtable:"BuildId"`
	VersionsName  string
	ProductConfig CDNHash
}

type BuildConfigEncoding struct {
	ContentHash ContentHash
	CDNHash     CDNHash
}

type BuildConfigEncodingSize struct {
	UncompressedSize uint64
	CompressedSize   uint64
}

type BuildConfig struct {
	Root ContentHash

	Install     ContentHash
	InstallSize uint64

	Download     ContentHash
	DownloadSize uint64

	Encoding     BuildConfigEncoding
	EncodingSize BuildConfigEncodingSize

	Patch       ContentHash
	PatchSize   uint64
	PatchConfig CDNHash
}

type CDNConfig struct {
	Archives     []CDNHash
	ArchiveGroup CDNHash

	PatchArchives     []CDNHash
	PatchArchiveGroup CDNHash
}

type FilenameMapper interface {
	ToContentHash(fn string) (h ContentHash, ok bool)
}
