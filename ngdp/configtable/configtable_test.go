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

	complexExampleTable = `Region!STRING:0|BuildConfig!HEX:16|CDNConfig!HEX:16|KeyRing!HEX:16|BuildId!DEC:4|VersionsName!String:0|ProductConfig!HEX:16|OtherNumber!DEC:4
us|a423790b9bcee8ac532ceb39fe550685|c8043457fcf9eb6dac433e53fa47f5||44247|2.5.0.44247|f03448a5aa6c9f1e9307335946af05|27
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
		{"Name", "string", 0},
		{"Path", "string", 0},
		{"Hosts", "string", 0},
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
		"UnsupportedType!FOO:0\n",
		"Duplicate!STRING:0|Duplicate!STRING:0\n",
		"NoByteLength!FOO\n",
		"BadByteLength!FOO:BAR\n",
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
		Path       string   `configtable:"Patho"`
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

func TestDecodeBadDecode(t *testing.T) {
	for _, test := range []struct {
		inp string
		s   interface{}
	}{
		{"Test!DEC:1\nZZZ\n", &struct{ Test uint8 }{}},
		{"Test!DEC:1\nZZZ\n", &struct{ Test int8 }{}},
		{"Test!HEX:1\nZZZ\n", &struct{ Test []byte }{}},
		{"Test!HEX:1\nZZZ\n", &struct{ Test [1]byte }{}},
	} {
		d := NewDecoder(strings.NewReader(test.inp))
		if err := d.Decode(test.s); err == nil {
			t.Errorf("d.Decode: %v; want error", err)
		}
	}
}

func TestDecodeComplexExample(t *testing.T) {
	d := NewDecoder(strings.NewReader(complexExampleTable))
	type Version struct {
		Region        string
		BuildConfig   string
		CDNConfig     []byte
		BuildId       int32
		VersionsName  string
		ProductConfig [16]byte
		OtherNumber   uint32
	}
	want := Version{
		"us",
		"a423790b9bcee8ac532ceb39fe550685",
		[]byte{0xc8, 0x04, 0x34, 0x57, 0xfc, 0xf9, 0xeb, 0x6d, 0xac, 0x43, 0x3e, 0x53, 0xfa, 0x47, 0xf5},
		44247,
		"2.5.0.44247",
		[16]byte{0x00, 0xf0, 0x34, 0x48, 0xa5, 0xaa, 0x6c, 0x9f, 0x1e, 0x93, 0x07, 0x33, 0x59, 0x46, 0xaf, 0x05},
		27,
	}
	var got Version
	if err := d.Decode(&got); err != nil {
		t.Errorf("d.Decode: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%v; want=%v", got, want)
	}
}

func TestByteWidth(t *testing.T) {
	for _, test := range []struct {
		s        interface{}
		width    int
		unsigned bool
	}{
		{struct{ T int }{}, 4, false},
		{struct{ T uint }{}, 4, true},
		{struct{ T int8 }{}, 1, false},
		{struct{ T uint8 }{}, 1, true},
		{struct{ T int16 }{}, 2, false},
		{struct{ T uint16 }{}, 2, true},
		{struct{ T int32 }{}, 4, false},
		{struct{ T uint32 }{}, 4, true},
		{struct{ T int64 }{}, 8, false},
		{struct{ T uint64 }{}, 8, true},
	} {
		k := reflect.ValueOf(test.s).Type().Field(0).Type.Kind()
		width, unsigned := byteWidth(k)
		if width != test.width {
			t.Errorf("%v: byteWidth width=%v; want %v", k, width, test.width)
		}
		if unsigned != test.unsigned {
			t.Errorf("%v: byteWidth unsigned=%v; want %v", k, unsigned, test.unsigned)
		}
	}
}

func TestByteWidthPanic(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Errorf("expected panic, got nothing")
		}
	}()
	byteWidth(reflect.ValueOf(nil).Kind())
}
