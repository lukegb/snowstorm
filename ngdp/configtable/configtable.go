package configtable

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
	"strconv"
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
	byteLen int
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

		blizzType := strings.Split(strings.ToLower(bits[1]), ":")
		if len(blizzType) != 2 {
			d.err = fmt.Errorf("configtable: expected type to be TYPENAME:BYTELEN; got %q", bits[1])
			return d.err
		}
		byteLen, err := strconv.Atoi(blizzType[1])
		if err != nil {
			d.err = fmt.Errorf("configtable: expected type to be TYPENAME:BYTELEN; got %q: %v", bits[1], err)
			return d.err
		}

		if blizzType[0] != "string" && blizzType[0] != "hex" && blizzType[0] != "dec" {
			d.err = fmt.Errorf("configtable: unsupported type %q", bits[1])
			return d.err
		}

		columns[n] = column{
			name:    bits[0],
			colType: blizzType[0],
			byteLen: byteLen,
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

func byteWidth(k reflect.Kind) (width int, unsigned bool) {
	switch k {
	case reflect.Int, reflect.Uint:
		return 4, k == reflect.Uint // Go spec specifies at least 32-bits in size
	case reflect.Int8, reflect.Uint8:
		return 1, k == reflect.Uint8
	case reflect.Int16, reflect.Uint16:
		return 2, k == reflect.Uint16
	case reflect.Int32, reflect.Uint32:
		return 4, k == reflect.Uint32
	case reflect.Int64, reflect.Uint64:
		return 8, k == reflect.Uint64
	}
	panic(fmt.Sprintf("cannot handle kind %v", k))
}

func isValidPairing(from column, to reflect.Type) bool {
	k := to.Kind()
	switch {
	case k == reflect.String:
		// can always convert into a string literally
		return true

	case from.colType == "string" && k == reflect.Slice && to.Elem().Kind() == reflect.String:
		// can convert "string" into a slice of strings
		return true

	case from.colType == "dec":
		// can convert dec into an integer of sufficient width
		bw, _ := byteWidth(k)
		return bw >= from.byteLen

	case from.colType == "hex":
		switch {
		case k == reflect.Slice && to.Elem().Kind() == reflect.Uint8:
			// can convert hex into a slice of bytes
			return true
		case k == reflect.Array && to.Elem().Kind() == reflect.Uint8:
			// can convert hex into an array of bytes of exactly the correct length
			return to.Len() == from.byteLen
		}
	}
	return false
}

func convertTo(columnDelimiter *string, from column, value string, to reflect.Value) error {
	k := to.Kind()
	switch {
	case k == reflect.String:
		to.SetString(value)

	case from.colType == "string" && k == reflect.Slice && to.Type().Elem().Kind() == reflect.String:
		// can convert "string" into a slice of strings
		delim := " "
		if columnDelimiter != nil {
			delim = *columnDelimiter
		}
		bits := strings.Split(value, delim)
		bitsV := reflect.ValueOf(bits)
		to.Set(bitsV)

	case from.colType == "dec":
		// can convert dec into an integer of sufficient width
		bw, unsigned := byteWidth(k)
		if unsigned {
			v, err := strconv.ParseUint(value, 10, bw*8)
			if err != nil {
				return fmt.Errorf("parsing %q: %v", value, err)
			}
			to.SetUint(v)
		} else {
			v, err := strconv.ParseInt(value, 10, bw*8)
			if err != nil {
				return fmt.Errorf("parsing %q: %v", value, err)
			}
			to.SetInt(v)
		}

	case from.colType == "hex":
		switch {
		case k == reflect.Slice && to.Type().Elem().Kind() == reflect.Uint8:
			v, err := hex.DecodeString(value)
			if err != nil {
				return fmt.Errorf("parsing %q: %v", value, err)
			}
			to.SetBytes(v)
		case k == reflect.Array && to.Type().Elem().Kind() == reflect.Uint8:
			// can convert hex into an array of bytes of exactly the correct length
			vs, err := hex.DecodeString(value)
			if err != nil {
				return fmt.Errorf("parsing %q: %v", value, err)
			}
			arrLen := to.Len()
			for n, v := range vs {
				newN := arrLen - (len(vs) - n)
				to.Index(newN).SetUint(uint64(v))
			}
		}
	}

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

		if !isValidPairing(d.columns[columnID], f.Type) {
			return fmt.Errorf("configtable: cannot decode %v into %v", d.columns[columnID], f.Type)
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

		var delim *string
		if d, ok := columnDelimiters[n]; ok {
			delim = &d
		}

		if err := convertTo(delim, d.columns[n], s, v); err != nil {
			d.err = fmt.Errorf("configtable: %v", err)
			return d.err
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
