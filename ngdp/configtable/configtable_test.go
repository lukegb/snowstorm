package configtable

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

const (
	exampleTable = `Name!STRING:0|Path!STRING:0|Hosts!STRING:0
blah|blah|blah
foo|foo|foo foo2
baa|bab|bac bad bae
`
)

func TestLine(t *testing.T) {
	d := NewDecoder(strings.NewReader("hello\n"))
	got, err := d.line()
	if err != nil {
		t.Errorf("d.line(): %v", err)
	}

	if got != "hello" {
		t.Errorf("d.line() = %q; want %q", got, "hello")
	}
}

func TestLineError(t *testing.T) {
	d := NewDecoder(strings.NewReader("hello\n"))
	_, err := d.line()
	if err != nil {
		t.Errorf("d.line(): %v", err)
	}
	_, err = d.line()
	if err != io.EOF {
		t.Errorf("d.line(): %v; want EOF", err)
	}
}

func TestReadHeader(t *testing.T) {
	d := NewDecoder(strings.NewReader("Name!STRING:0|Path!STRING:0|Hosts!STRING:0\nblah|blah|blah\nfoo|foo|foo\nbar|bar|bar\n"))
	if err := d.readHeader(); err != nil {
		t.Errorf("d.readHeader(): %v", err)
		return
	}

	want := []column{
		{"Name", "STRING:0"},
		{"Path", "STRING:0"},
		{"Hosts", "STRING:0"},
	}
	got := d.columns
	if !reflect.DeepEqual(got, want) {
		t.Errorf("d.columns = %#v; want %#v", got, want)
	}
}

func TestReadHeaderEOF(t *testing.T) {
	d := NewDecoder(strings.NewReader(""))
	if err := d.readHeader(); err != io.EOF {
		t.Errorf("d.readHeader(): %v; want EOF", err)
	}
	if err := d.readHeader(); err != io.EOF {
		t.Errorf("d.readHeader() - again: %v; want EOF", err)
	}
	var s struct{}
	if err := d.Decode(&s); err != io.EOF {
		t.Errorf("d.Decode(): %v; want EOF", err)
	}
}

func TestReadHeaderErrors(t *testing.T) {
	for _, test := range []string{
		"NoType\n",
		"UnsupportedType!INT:0\n",
		"Duplicate!STRING:0|Duplicate!STRING:0\n",
	} {
		d := NewDecoder(strings.NewReader(test))
		if err := d.readHeader(); err == nil {
			t.Errorf("d.readHeader(): %v; want error", err)
		}
	}
}

func TestDecode(t *testing.T) {
	type S struct {
		Name       string
		Patho      string   `configtable:"Patho"`
		Hosts      []string `configtable:"Hosts, "`
		Delimited  []string `configtable:"Delimit,-"`
		unexported string   `configtable:"Patho"`
		Ignored    string   `configtable:"UnknownColumn"`
	}

	d := NewDecoder(strings.NewReader(`Name!STRING:0|Patho!STRING:0|Hosts!STRING:0|Delimit!STRING:0|NotDecoded!STRING:0
blah|blah|blah|blah|zab
foo|foo|foo foo2|foo-foo2|zab
baa|bab|bac bad-bae baf|bag-bah bai-baj|zab
`))
	wants := []S{
		{"blah", "blah", []string{"blah"}, []string{"blah"}, "", ""},
		{"foo", "foo", []string{"foo", "foo2"}, []string{"foo", "foo2"}, "", ""},
		{"baa", "bab", []string{"bac", "bad-bae", "baf"}, []string{"bag", "bah bai", "baj"}, "", ""},
	}

	for n, want := range wants {
		var s S
		if err := d.Decode(&s); err != nil {
			t.Errorf("%d: d.Decode: %v", n, err)
		}
		if !reflect.DeepEqual(want, s) {
			t.Errorf("%d: got=%#v; want=%#v", n, s, want)
		}
	}

	var s S
	if err := d.Decode(&s); err != io.EOF {
		t.Errorf("at EOF: d.Decode: %v; want EOF", err)
	}
}

func TestDecodeNonStructPtr(t *testing.T) {
	d := NewDecoder(strings.NewReader(exampleTable))

	var s string
	if err := d.Decode(&s); err == nil {
		t.Errorf("d.Decode: %v; want error", err)
	}

	var s2 struct{}
	if err := d.Decode(s2); err == nil {
		t.Errorf("d.Decode: %v; want error", err)
	}
}

func TestDecodeNonStringField(t *testing.T) {
	d := NewDecoder(strings.NewReader(exampleTable))
	var s struct {
		Name int
	}
	if err := d.Decode(&s); err == nil {
		t.Errorf("d.Decode: %v; want error", err)
	}
}

func TestDecodeColumnCountMismatch(t *testing.T) {
	d := NewDecoder(strings.NewReader("Column!STRING:0|Name!STRING:0\nsingle column\n"))
	var s struct{}
	if err := d.Decode(&s); err == nil {
		t.Errorf("d.Decode: %v; want error", err)
	}
}
