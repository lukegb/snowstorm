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

package keyvalue

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	type Embedded struct {
		Left  string
		Right string
	}
	type T struct {
		String               string
		StringWithCustomName string `keyvalue:"swcn"`
		SliceOfString        []string
		Uint                 uint64
		Int                  int64
		Embedded             Embedded
		unexported           string
	}

	in := `# ignored line
string = blah
swcn = blah2
slice-of-string = blah1 blah2 blah3 blah4
uint = 65536
int = -300
ignored-field = ignored
embedded = left right
`
	want := T{
		String:               "blah",
		StringWithCustomName: "blah2",
		SliceOfString:        []string{"blah1", "blah2", "blah3", "blah4"},
		Uint:                 65536,
		Int:                  -300,
		Embedded: Embedded{
			Left:  "left",
			Right: "right",
		},
	}

	var got T
	if err := Decode(strings.NewReader(in), &got); err != nil {
		t.Errorf("Decode: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Decode = %#v; want = %#v", got, want)
	}
}

func TestDecodeErrorNotStructPointer(t *testing.T) {
	var r io.Reader

	var s string
	if err := Decode(r, s); err != ErrNotStructPointer {
		t.Errorf("Decode: %v; want %v", err, ErrNotStructPointer)
	}

	if err := Decode(r, &s); err != ErrNotStructPointer {
		t.Errorf("Decode: %v; want %v", err, ErrNotStructPointer)
	}
}

func TestDecodeErrorDecodingInt(t *testing.T) {
	type T struct {
		Int  int
		Uint uint
	}

	for _, test := range []string{
		"int = z",
		"uint = z",
	} {
		var got T
		if err := Decode(strings.NewReader(test), &got); err == nil {
			t.Errorf("Decode: %v; want error", err)
		}
	}
}

func TestDecodeErrorEmbeddedStruct(t *testing.T) {
	type T struct {
		Embedded struct {
			One int64
		}
	}

	var got T
	if err := Decode(strings.NewReader("embedded = one two three"), &got); err == nil {
		t.Errorf("Decode: %v; want error", err)
	}
	if err := Decode(strings.NewReader("embedded = one"), &got); err == nil {
		t.Errorf("Decode: %v; want error", err)
	}
}

func TestDecodeErrorUnknownType(t *testing.T) {
	type T struct {
		Interface interface{}
	}

	var got T
	if err := Decode(strings.NewReader("interface = 5"), &got); err == nil {
		t.Errorf("Decode: %v; want error", err)
	}
}
