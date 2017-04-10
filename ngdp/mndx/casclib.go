package mndx

// #cgo CXXFLAGS: -I./CascLib/src -DWHO_NEEDS_A_BUILD_SYSTEM_ANYWAY -Wno-write-strings -Wno-conversion-null
// #include <stdint.h>
// #include <stdlib.h>
// #include "casclib.h"
import "C"
import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"unsafe"
)

type File struct {
	Name string
	Size uint32

	LocaleFlags uint32
	FileDataID  uint32

	EncodingKey [md5.Size]byte
}

func FileList(r io.Reader) (map[string]*File, error) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	cbuf := C.CBytes(buf)

	var cFileCount C.uint32_t
	var filesPtr *C.struct_mndx_file

	if ret := C.DoTheThing(cbuf, C.uint32_t(len(buf)), &filesPtr, &cFileCount); ret != 0 {
		return nil, fmt.Errorf("return code %x", ret)
	}

	fileCount := uint32(cFileCount)

	out := make(map[string]*File)
	for n := uint32(0); n < fileCount; n++ {
		f := *((*C.struct_mndx_file)(unsafe.Pointer(uintptr(unsafe.Pointer(filesPtr)) + uintptr(n)*C.sizeof_struct_mndx_file)))

		fn := C.GoString(f.name)
		out[fn] = &File{
			Name:        fn,
			Size:        uint32(f.size),
			LocaleFlags: uint32(f.localeFlags),
			FileDataID:  uint32(f.fileDataID),
			EncodingKey: *((*[16]byte)(unsafe.Pointer(&f.encodingKey))),
		}
	}

	C.free(cbuf)
	C.FreeTheThings(filesPtr, cFileCount)

	return out, nil
}
