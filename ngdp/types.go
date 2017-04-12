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

// A CDNHash is usually an MD5 hash of the BLTE header of a data file. Blizzard uses these to generate filenames for storage on the CDN.
type CDNHash [md5.Size]byte

// A ContentHash is an MD5 hash of the raw contents of a file, before it is BLTE-encoded. These must be looked up in the encoding table to get a CDNHash before files can be downloaded.
type ContentHash [md5.Size]byte

// A CDNInfo contains information on which CDNs hold data for which regions, as well as what path the data is stored under.
type CDNInfo struct {
	Name       Region
	Path       string
	Hosts      []string
	ConfigPath string // unknown purpose
}

// A VersionInfo lists the current build and CDN config CDNHashes.
type VersionInfo struct {
	Region        Region
	BuildConfig   CDNHash
	CDNConfig     CDNHash
	BuildID       int `configtable:"BuildId"`
	VersionsName  string
	ProductConfig CDNHash
}

// A BuildConfigEncoding contains the content and CDN hashes of an encoding file.
type BuildConfigEncoding struct {
	ContentHash ContentHash
	CDNHash     CDNHash
}

// A BuildConfigEncodingSize contains the BLTE-encoded and raw sizes of the encoding file.
type BuildConfigEncodingSize struct {
	UncompressedSize uint64
	CompressedSize   uint64
}

// A BuildConfig contains information on the current root, install, and download files, as well as the encoding file, and the currently available patch.
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

// A CDNConfig contains information on the archives, which are used to bundle smaller files together on the CDN.
type CDNConfig struct {
	Archives     []CDNHash
	ArchiveGroup CDNHash

	PatchArchives     []CDNHash
	PatchArchiveGroup CDNHash
}

// A FilenameMapper represents a way for mapping filenames to content hashes.
//
// This might be implemented by a listfile, or MNDX, or any of the myriad other ways Blizzard has deigned to refer to files.
type FilenameMapper interface {
	ToContentHash(fn string) (h ContentHash, ok bool)
}
