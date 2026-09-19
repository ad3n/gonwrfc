//go:build gorfc_native_test && cgo

package gorfc

/*
#include <sapnwrfc.h>
*/
import "C"

func nativeTypeDescriptionFixture(name string) (wrap func() (TypeDescription, error), destroy func() error, err error) {
	cName, err := fillString(name)
	defer freeSAPUC(cName)
	if err != nil {
		return nil, nil, err
	}
	var info C.RFC_ERROR_INFO
	handle := C.RfcCreateTypeDesc(cName, &info)
	if handle == nil {
		return nil, nil, rfcError(info, "creating local type description")
	}
	return func() (TypeDescription, error) {
			return wrapTypeDescription(handle)
		}, func() error {
			if handle == nil {
				return nil
			}
			if C.RfcDestroyTypeDesc(handle, &info) != C.RFC_OK {
				return rfcError(info, "destroying local type description")
			}
			handle = nil
			return nil
		}, nil
}
