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

package mndx

import (
	"crypto/md5"
	"errors"
	"path"
	"sort"
	"strings"
)

// Error constants
var (
	ErrDirFileNameClash = errors.New(`mndx: file and directory have clashing names`)
	ErrExists           = errors.New(`mndx: file has clashing name`)
	ErrNotExists        = errors.New(`mndx: no such file or directory`)
	ErrNotADirectory    = errors.New(`mndx: not a directory`)
)

// A TreeDents is a sort.Interface of TreeDirectoryEntry structs.
type TreeDents []*TreeDirectoryEntry

func (td TreeDents) Len() int           { return len(td) }
func (td TreeDents) Less(i, j int) bool { return td[i].Name < td[j].Name }
func (td TreeDents) Swap(i, j int)      { td[i], td[j] = td[j], td[i] }

// A TreeDirectoryEntry is a directory entry, either a nested directory or a file.
type TreeDirectoryEntry struct {
	Name string

	Directory *TreeDirectory
	File      *TreeFile
}

// A TreeDirectory is a container for TreeDirectory or TreeFile structs, which can be addressed by their name.
type TreeDirectory struct {
	dents     map[string]*TreeDirectoryEntry
	flatDents []*TreeDirectoryEntry
}

func (td *TreeDirectory) flatten() {
	if td.flatDents != nil {
		return
	}

	dents := make(TreeDents, 0, len(td.dents))
	for _, v := range td.dents {
		dents = append(dents, v)
		if v.Directory != nil {
			v.Directory.flatten()
		}
	}
	sort.Sort(dents)
	td.dents = nil
	td.flatDents = dents
}

func newTreeDirectory() *TreeDirectory {
	return &TreeDirectory{
		dents: make(map[string]*TreeDirectoryEntry),
	}
}

// Get returns a TreeDirectoryEntry for a given /-separated path.
func (td *TreeDirectory) Get(filePath string) (TreeDirectoryEntry, error) {
	filePath = strings.TrimLeft(path.Clean(filePath), "/")
	tde, err := td.get(strings.Split(filePath, "/"))
	if err != nil {
		return TreeDirectoryEntry{}, err
	}
	return *tde, nil
}

func (td *TreeDirectory) get(path []string) (*TreeDirectoryEntry, error) {
	cname := strings.ToLower(path[0])

	n := len(td.flatDents)
	i := sort.Search(n, func(i int) bool {
		return strings.ToLower(td.flatDents[i].Name) >= cname
	})

	if i == n {
		return nil, ErrNotExists
	}
	dent := td.flatDents[i]
	if strings.ToLower(dent.Name) != cname {
		return nil, ErrNotExists
	}

	if len(path) == 1 {
		// if this is the last segment, just return it
		return dent, nil
	}

	if dent.Directory == nil {
		return nil, ErrNotADirectory
	}

	return dent.Directory.get(path[1:])
}

func (td *TreeDirectory) asEntry(name string) *TreeDirectoryEntry {
	return &TreeDirectoryEntry{
		// the string-of-[]byte is here to ensure that we copy the bit of the string we need and don't retain a reference to the original string
		Name:      string([]byte(name)),
		Directory: td,
	}
}

func (td *TreeDirectory) mkdirs(path []string) (*TreeDirectory, error) {
	if len(path) == 0 {
		return td, nil
	}

	cname := strings.ToLower(path[0])
	dent, ok := td.dents[cname]
	if !ok {
		dent = newTreeDirectory().asEntry(path[0])
		td.dents[cname] = dent
	}
	if dent.Directory == nil {
		return nil, ErrDirFileNameClash
	}
	return dent.Directory.mkdirs(path[1:])
}

func (td *TreeDirectory) addFile(f *File, name string) (*TreeFile, error) {
	cname := strings.ToLower(name)
	_, ok := td.dents[cname]
	if ok {
		return nil, ErrExists
	}

	dent := newTreeFile(f).asEntry(name)
	td.dents[cname] = dent

	return dent.File, nil
}

// A TreeFile contains the metadata for a file, including its CDN hash.
type TreeFile struct {
	Size uint32

	LocaleFlags uint32
	FileDataID  uint32

	EncodingKey [md5.Size]byte
}

func newTreeFile(f *File) *TreeFile {
	return &TreeFile{
		Size: f.Size,

		LocaleFlags: f.LocaleFlags,
		FileDataID:  f.FileDataID,

		EncodingKey: f.EncodingKey,
	}
}

func (tf *TreeFile) asEntry(name string) *TreeDirectoryEntry {
	return &TreeDirectoryEntry{
		// the string-of-[]byte is here to ensure that we copy the bit of the string we need and don't retain a reference to the original string
		Name: string([]byte(name)),
		File: tf,
	}
}

// ToTree takes a FilenameMap and converts it into a tree structure.
func ToTree(fileMap FilenameMap) (*TreeDirectory, error) {
	root := newTreeDirectory()

	for filePath, file := range fileMap {
		filePath = strings.TrimLeft(path.Clean(filePath), "/")
		dirPath := path.Dir(filePath)
		dir, err := root.mkdirs(strings.Split(dirPath, "/"))
		if err != nil {
			return nil, err
		}
		if _, err := dir.addFile(file, path.Base(filePath)); err != nil {
			return nil, err
		}
	}
	root.flatten()

	return root, nil
}
