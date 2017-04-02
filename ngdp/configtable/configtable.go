package configtable

import (
	"bufio"
	"fmt"
	"io"
	"reflect"
	"strings"
)

const (
	typeDelimiter   = "!"
	columnDelimiter = "|"

	structTag = "configtable"
)

type column struct {
	name    string
	colType string
}

// A Decoder reads a Blizzard config table from an input stream.
type Decoder struct {
	columns     []column
	columnNames map[string]int
	s           *bufio.Scanner
	err         error
}

func (d *Decoder) line() (string, error) {
	if d.err != nil {
		return "", d.err
	}
	if !d.s.Scan() {
		d.err = d.s.Err()
		if d.err == nil {
			d.err = io.EOF
		}
		return "", d.err
	}
	return d.s.Text(), nil
}

func (d *Decoder) readHeader() error {
	if d.columns != nil {
		// already done, don't trigger twice
		return nil
	}

	headerLine, err := d.line()
	if err != nil {
		return err
	}
	fullHeaders := strings.Split(headerLine, columnDelimiter)

	columns := make([]column, len(fullHeaders))
	columnNames := make(map[string]int)
	for n, h := range fullHeaders {
		bits := strings.Split(h, typeDelimiter)
		if len(bits) != 2 {
			d.err = fmt.Errorf("configtable: missing type delimiter in header")
			return d.err
		}

		if bits[1] != "STRING:0" {
			d.err = fmt.Errorf("configtable: unsupported type %q; only strings supported!")
			return d.err
		}

		columns[n] = column{
			name:    bits[0],
			colType: bits[1],
		}

		if _, ok := columnNames[bits[0]]; ok {
			d.err = fmt.Errorf("configtable: duplicate column name %q", bits[0])
			return d.err
		}
		columnNames[bits[0]] = n
	}
	d.columns = columns
	d.columnNames = columnNames

	return nil
}

// Decode decodes a line from the config table into a provided struct.
func (d *Decoder) Decode(s interface{}) error {
	if err := d.readHeader(); err != nil {
		return err
	}

	if reflect.TypeOf(s).Kind() != reflect.Ptr {
		return fmt.Errorf("configtable: cannot decode into non-struct-pointer")
	}

	v := reflect.Indirect(reflect.ValueOf(s))
	st := v.Type()
	if !v.IsValid() || st.Kind() != reflect.Struct {
		return fmt.Errorf("configtable: cannot decode into non-struct-pointer")
	}

	// create mappings from column indexes to field indexes.
	columnToField := make(map[int]reflect.Value)
	columnDelimiters := make(map[int]string)
	fields := v.NumField()
	for n := 0; n < fields; n++ {
		f := st.Field(n)
		// cheat and use PkgPath to check if this field is exported.
		if f.PkgPath != "" {
			// unexported, skip since we won't be able to set it anyway.
			continue
		}
		columnName := f.Name
		var columnDelimiter string

		if tag := f.Tag.Get(structTag); tag != "" {
			if strings.Contains(tag, ",") {
				bits := strings.Split(tag, ",")
				columnName = bits[0]
				columnDelimiter = bits[1]
			}
		}

		columnID, ok := d.columnNames[columnName]
		if !ok {
			continue
		}

		switch {
		case f.Type.Kind() == reflect.String:
			break
		case f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String:
			break
		default:
			return fmt.Errorf("configtable: can only decode string or string-slices")
		}

		columnToField[columnID] = v.Field(n)
		if columnDelimiter != "" {
			columnDelimiters[columnID] = columnDelimiter
		}
	}

	ln, err := d.line()
	if err != nil {
		return err
	}

	bits := strings.Split(ln, columnDelimiter)
	if len(bits) != len(d.columns) {
		d.err = fmt.Errorf("configtable: column count mismatch: saw %d columns, expected %d", len(bits), len(d.columns))
		return d.err
	}

	for n, s := range bits {
		v, ok := columnToField[n]
		if !ok {
			continue
		}
		vt := v.Type()
		switch vt.Kind() {
		case reflect.String:
			columnToField[n].SetString(s)
		case reflect.Slice:
			delim := " "
			if d, ok := columnDelimiters[n]; ok {
				delim = d
			}
			bits := strings.Split(s, delim)
			bitsV := reflect.ValueOf(bits)
			columnToField[n].Set(bitsV)
		}
	}

	return nil
}

// NewDecoder creates a new Decoder from the provided io.Reader.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		s: bufio.NewScanner(r),
	}
}
