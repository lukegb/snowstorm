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

// A ProgramCode is a reference to a particular game or game release channel.
//
// Blizzard tracks release and PTR as separate program codes, even though they usually refer to the same underlying CDN storage.
type ProgramCode string

const (
	// ProgramHotS is the ProgramCode for Heroes of the Storm.
	ProgramHotS ProgramCode = "hero"

	// ProgramHotSTest is the ProgramCode for the PTR of Heroes of the Storm.
	ProgramHotSTest ProgramCode = "herot"
)

// A Region is a reference to a game region, and is used for finding the nearest CDNs.
//
// In most cases, Akamai and Level3 are used anyway - China being the main exception.
type Region string

// The region codes below are all the ones used for Heroes of the Storm which are known at the time of writing.
const (
	RegionUnitedStates Region = "us"
	RegionEurope       Region = "eu"
	RegionChina        Region = "cn"
	RegionKorea        Region = "kr"
	RegionTaiwan       Region = "tw"
	RegionSingapore    Region = "sg"
)

// A ContentType is a type of thing stored on the CDN.
//
// Each separate content type is stored under a different directory.
type ContentType string

// The content types below are believed to be exhaustive.
const (
	ContentTypeConfig ContentType = "config"
	ContentTypeData   ContentType = "data"
	ContentTypePatch  ContentType = "patch"
)
