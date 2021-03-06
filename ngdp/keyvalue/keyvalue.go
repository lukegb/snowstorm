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
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var (
	fieldNameRegexp = regexp.MustCompile(`[\p{Lu}][^\p{Lu}]*`)
)

// Error constants
var (
	ErrNotStructPointer = fmt.Errorf("keyvalue: cannot decode into non-struct-pointer")
)

const (
	structTag      = "keyvalue"
	commentChar    = "#"
	valueSeparator = "="
)

func convertFieldName(s string) string {
	bits := fieldNameRegexp.FindAllString(s, -1)
	for n, bit := range bits {
		bits[n] = strings.ToLower(bit)
	}
	return strings.Join(bits, "-")
}

// Decode decodes a file containing key-value pairs into a given interface.
func Decode(ir io.Reader, s interface{}) error {
	if reflect.TypeOf(s).Kind() != reflect.Ptr {
		return ErrNotStructPointer
	}

	v := reflect.Indirect(reflect.ValueOf(s))
	st := v.Type()
	if !v.IsValid() || st.Kind() != reflect.Struct {
		return ErrNotStructPointer
	}

	// create mappings from field names to reflect.Values.
	fieldToValue := make(map[string]reflect.Value)
	fields := v.NumField()
	for n := 0; n < fields; n++ {
		f := st.Field(n)
		// cheat and use PkgPath to check if this field is exported.
		if f.PkgPath != "" {
			// unexported, skip since we won't be able to set it anyway.
			continue
		}

		fieldName := convertFieldName(f.Name)
		if tag := f.Tag.Get(structTag); tag != "" {
			fieldName = tag
		}

		fieldToValue[fieldName] = v.Field(n)
	}

	// now read through the file
	r := bufio.NewScanner(ir)

	for r.Scan() {
		txt := strings.TrimSpace(r.Text())
		if len(txt) == 0 || strings.HasPrefix(txt, commentChar) {
			// skip line
			continue
		}

		bits := strings.SplitN(txt, valueSeparator, 2)
		key := strings.TrimSpace(bits[0])
		value := strings.TrimSpace(bits[1])

		f, ok := fieldToValue[key]
		if !ok {
			// no field to smush value into, skip
			continue
		}

		if err := setValue(f, value); err != nil {
			return fmt.Errorf("keyvalue: setting field %v to %q: %v", key, value, err)
		}
	}

	return nil
}

func setValue(f reflect.Value, value string) error {
	switch {
	case f.Kind() == reflect.String:
		f.SetString(value)
	case f.Kind() == reflect.Slice && f.Type().Elem().Kind() == reflect.Uint8:
		// interpret as hex
		vh, err := hex.DecodeString(value)
		if err != nil {
			return err
		}
		f.Set(reflect.ValueOf(vh))
	case f.Kind() == reflect.Array && f.Type().Elem().Kind() == reflect.Uint8:
		// interpret as hex
		vh, err := hex.DecodeString(value)
		if err != nil {
			return err
		}

		for n := f.Len() - 1; n >= 0 && n >= f.Len()-len(vh); n-- {
			f.Index(n).SetUint(uint64(vh[n]))
		}
	case f.Kind() == reflect.Slice:
		bits := strings.Split(value, " ")

		slice := reflect.MakeSlice(f.Type(), len(bits), len(bits))
		for n, v := range bits {
			if err := setValue(slice.Index(n), v); err != nil {
				return err
			}
		}
		f.Set(slice)
	case f.Kind() >= reflect.Int && f.Kind() <= reflect.Int64:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		f.SetInt(v)
	case f.Kind() >= reflect.Uint && f.Kind() <= reflect.Uint64:
		v, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		f.SetUint(v)
	case f.Kind() == reflect.Struct:
		bits := strings.Split(value, " ")
		if len(bits) != f.NumField() {
			return fmt.Errorf("keyvalue: unpacking into embedded struct of different length")
		}
		for n, bit := range bits {
			fv := f.Field(n)
			if err := setValue(fv, bit); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("keyvalue: don't know how to unpack into kind %v", f.Kind())
	}
	return nil
}
