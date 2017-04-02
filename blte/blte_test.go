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

package blte

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestReader(t *testing.T) {
	for _, test := range []struct {
		fn   string
		want string
	}{
		{
			"noheader.uncompressed.blte",
			"this BLTE file contains uncompressed data, with no chunks",
		},
		{
			"noheader.zlib.blte",
			"this BLTE file contains zlib-compressed data, with no chunks",
		},
		{
			"onechunk.uncompressed.blte",
			"this BLTE file contains uncompressed data, with a single chunk",
		},
		{
			"onechunk.zlib.blte",
			"this BLTE file contains zlib-compressed data, with a single chunk",
		},
		{
			"manychunks.uncompressed.blte",
			"this BLTE file contains an obscene number of uncompressed chunks - at least, a sufficient number of chunks to make sure that decoding is happening correctly, even where the number of chunks exceeds 255, since it almost certainly will at some point, and thus we should be prepared.",
		},
		{
			"manychunks.zlib.blte",
			"this BLTE file contains an obscene number of zlib-compressed chunks - at least, a sufficient number of chunks to make sure that decoding is happening correctly, even where the number of chunks exceeds 255, since it almost certainly will at some point, and thus we should be prepared.",
		},
		{
			"manychunks.mixed.blte",
			"this BLTE file contains an obscene number of a mixture of uncompressed and zlib-compressed chunks - at least, a sufficient number of chunks to make sure that decoding is happening correctly, even where the number of chunks exceeds 255, since it almost certainly will at some point, and thus we should be prepared.",
		},
	} {
		test := test
		t.Run(test.fn, func(t *testing.T) {
			path := filepath.Join("testdata", test.fn)
			f, err := os.Open(path)
			defer f.Close()
			if err != nil {
				t.Errorf("os.Open(%q): %v", path, err)
				return
			}

			r := NewReader(f)
			buf, err := ioutil.ReadAll(r)
			if err != nil {
				t.Errorf("ioutil.ReadAll: %v", err)
				return
			}

			got := string(buf)
			if got != test.want {
				t.Errorf("got %q; want %q", got, test.want)
			}
		})
	}
}
