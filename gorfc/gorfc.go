//go:build (linux && cgo) || (amd64 && cgo) || (darwin && cgo)
// +build linux,cgo amd64,cgo darwin,cgo

// Package gorfc provides SAP NetWeawer RFC SDK client bindings for GO
package gorfc

/*

// ~~~~ windows ~~~~ //

#cgo windows CFLAGS: -D_CRT_NON_CONFORMING_SWPRINTFS -D_CRT_SECURE_NO_DEPRECATE -D_CRT_NONSTDC_NO_DEPRECATE -D_CONSOLE
#cgo windows CFLAGS: -DSAPonNT -D_AFXDLL -DWIN32 -D_WIN32_WINNT=0x0502 -DWIN64 -D_AMD64_
#cgo windows CFLAGS: -DSAPwithUNICODE -DUNICODE -D_UNICODE
#cgo windows CFLAGS: -DSAPwithTHREADS -D_ATL_ALLOW_CHAR_UNSIGNED -DSAP_PLATFORM_MAKENAME=ntintel
#cgo windows CFLAGS: -DNDEBUG -D_LARGEFILE_SOURCE -D_FILE_OFFSET_BITS=64 -D__NO_MATH_INLINES
#cgo windows CFLAGS: -O2 -g -pthread -pipe -m64
#cgo windows CFLAGS: -mwindows -march=x86-64
#cgo windows CFLAGS: -fno-strict-aliasing -fno-omit-frame-pointer -fexceptions -funsigned-char
#cgo windows CFLAGS: -Wall -Wno-uninitialized -Wno-long-long
#cgo windows CFLAGS: -Wcast-align -Wunused-variable
// todo -EHs ?
// todo -Gy ? -ffunction-sections -fdata-sections
// todo MD ? -lpthread -lm
// todo -nologo -W3 -Z7  -GL -O2 -Oy- /we4552 /we4700 /we4789

#cgo windows CFLAGS: -IC:/Tools/nwrfcsdk/include/
#cgo windows LDFLAGS: -LC:/Tools/nwrfcsdk/lib/ -lsapnwrfc -llibsapucum

#cgo windows LDFLAGS: -O2 -g -pthread -pie -fPIE
#cgo windows LDFLAGS: -OPT:REF -LTCG
// todo -NXCOMPAT -STACK:0x2000000 -SWAPRUN:NET -DEBUG -DEBUGTYPE:CV,FIXUP -MACHINE:amd64 -nologo

// ~~~~ linux ~~~~ //

#cgo linux CFLAGS: -DNDEBUG -D_LARGEFILE_SOURCE -D_FILE_OFFSET_BITS=64
#cgo linux CFLAGS: -DSAPwithUNICODE -D__NO_MATH_INLINES -DSAPwithTHREADS
#cgo linux CFLAGS: -DSAPonUNIX -DSAPonLIN
#cgo linux CFLAGS: -O2 -g -pthread -pipe -m64
#cgo linux CFLAGS: -fno-strict-aliasing -fno-omit-frame-pointer -fexceptions -funsigned-char
#cgo linux CFLAGS: -Wall -Wno-uninitialized -Wno-long-long
#cgo linux CFLAGS: -Wcast-align -Wno-unused-variable

#cgo linux CFLAGS: -I/usr/local/sap/nwrfcsdk/include
#cgo linux LDFLAGS: -L/usr/local/sap/nwrfcsdk/lib -lsapnwrfc -lsapucum

#cgo linux LDFLAGS: -O2 -g -pthread

// ~~~~ darwin ~~~~ //

#cgo darwin CFLAGS: -Wall -O2 -Wno-uninitialized -Wcast-align
#cgo darwin CFLAGS: -DSAP_UC_is_wchar -DSAPwithUNICODE -D__NO_MATH_INLINES -DSAPwithTHREADS -DSAPonDARW
#cgo darwin CFLAGS: -fexceptions -funsigned-char -fno-strict-aliasing -fPIC -pthread -std=c17 -mmacosx-version-min=10.15
#cgo darwin CFLAGS: -fno-omit-frame-pointer

#cgo darwin CFLAGS: -I/usr/local/sap/nwrfcsdk/include
#cgo darwin LDFLAGS: -L/usr/local/sap/nwrfcsdk/lib -lsapnwrfc -lsapucum
#cgo darwin LDFLAGS: -Wl,-rpath,/usr/local/sap/nwrfcsdk/lib

#cgo darwin LDFLAGS: -O2 -g -pthread
#cgo darwin LDFLAGS: -stdlib=libc++
#cgo darwin LDFLAGS: -mmacosx-version-min=10.15

#include <sapnwrfc.h>

static SAP_UC* GoMallocU(unsigned size) {
	return (SAP_UC*)(mallocU(size));
}

static unsigned GoStrlenU(SAP_UTF16 *str) {
	return strlenU(str);
}

*/
import "C"

import (
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// ################################################################################
// # ERRORS                                                             	 	     #
// ################################################################################
// RfcError is returned by SAP NWRFC SDK
type RfcError struct {
	Description string
	ErrorInfo   rfcSDKError
}

func (err RfcError) Error() string {
	var b strings.Builder

	b.Grow(
		len(err.Description) +
			len(err.ErrorInfo.String()) +
			20,
	)

	b.WriteString("NWRFC SDK error: ")
	b.WriteString(err.Description)
	b.WriteString(" | ")
	b.WriteString(err.ErrorInfo.String())

	return b.String()
}

func rfcError(errorInfo C.RFC_ERROR_INFO, format string, a ...any) *RfcError {
	return &RfcError{fmt.Sprintf(format, a...), wrapError(&errorInfo)}
}

// GoRfcError is returned by gorfc
type GoRfcError struct {
	Description string
	GoError     error
}

func (err GoRfcError) Error() string {
	var b strings.Builder

	b.WriteString("GORFC error: ")
	b.WriteString(err.Description)

	if err.GoError != nil {
		b.WriteString(" | ")
		b.WriteString(err.GoError.Error())
	}

	return b.String()
}

func goRfcError(description string, goerror error) *GoRfcError {
	return &GoRfcError{description, goerror}
}

// ################################################################################
// # FILL FUNCTIONS                                                            	 #
// ################################################################################
// # Fill functions take Go values and return C values
// fillString allocates memory for the return value that has to be freed
func fillString(gostr string) (sapuc *C.SAP_UC, err error) {
	if gostr == "" {
		sapuc = C.GoMallocU(1)
		*sapuc = 0

		return
	}

	var rc C.RFC_RC
	var errorInfo C.RFC_ERROR_INFO
	var resultLen C.uint

	// Worst-case: each UTF-8 byte can expand to one UTF-16 code unit (2 bytes),
	// so len*2+1 avoids SDK-side reallocation for multibyte characters.
	sapucSize := C.uint(len(gostr)*2 + 1)
	sapuc = C.GoMallocU(sapucSize)
	*sapuc = 0
	var cStr *C.RFC_BYTE
	if len(gostr) > 0 {
		cStr = (*C.RFC_BYTE)(unsafe.Pointer(unsafe.StringData(gostr)))
	}

	rc = C.RfcUTF8ToSAPUC((*C.RFC_BYTE)(cStr), C.uint(len(gostr)), sapuc, &sapucSize, &resultLen, &errorInfo)
	if rc != C.RFC_OK {
		err = rfcError(errorInfo, "Could not fill the string \"%v\"", gostr)
	}

	return
}

func fillFunctionParameter(funcDesc C.RFC_FUNCTION_DESC_HANDLE, container C.RFC_FUNCTION_HANDLE, goName string, value any) (err error) {
	var rc C.RFC_RC
	var errorInfo C.RFC_ERROR_INFO
	var paramDesc C.RFC_PARAMETER_DESC
	var name *C.SAP_UC

	name, err = fillString(goName)
	defer C.free(unsafe.Pointer(name))

	if err != nil {
		return
	}

	rc = C.RfcGetParameterDescByName(funcDesc, name, &paramDesc, &errorInfo)
	if rc != C.RFC_OK {
		return rfcError(errorInfo, "Could not get the parameter description for \"%v\"", goName)
	}

	return fillVariable(paramDesc._type, container, (*C.SAP_UC)(&paramDesc.name[0]), value, paramDesc.typeDescHandle)
}

func fillVariable(cType C.RFCTYPE, container C.RFC_FUNCTION_HANDLE, cName *C.SAP_UC, value any, typeDesc C.RFC_TYPE_DESC_HANDLE) (err error) {
	var rc C.RFC_RC
	var errorInfo C.RFC_ERROR_INFO
	var structure C.RFC_STRUCTURE_HANDLE
	var table C.RFC_TABLE_HANDLE
	var cValue *C.SAP_UC
	var bValue *C.SAP_RAW

	// NOTE: cValue and bValue are freed individually inside their respective
	// switch cases below — after they are actually assigned — to avoid calling
	// C.free(nil) unconditionally on every invocation of this function.

	switch cType {
	case C.RFCTYPE_STRUCTURE:
		rc = C.RfcGetStructure(container, cName, &structure, &errorInfo)
		if rc != C.RFC_OK {
			return rfcError(errorInfo, "Could not get structure")
		}

		err = fillStructure(typeDesc, structure, value)
	case C.RFCTYPE_TABLE:
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice {
			return goRfcError(fmt.Sprintf("GO %s passed to ABAP TABLE parameter, expected GO array", reflect.TypeOf(value).String()), nil)
		}

		rc = C.RfcGetTable(container, cName, &table, &errorInfo)
		if rc != C.RFC_OK {
			return rfcError(errorInfo, "Could not get table")
		}

		err = fillTable(typeDesc, table, value)
	case C.RFCTYPE_BYTE:
		if bytesVal, ok := value.([]byte); ok && len(bytesVal) > 0 {
			bValue = (*C.SAP_RAW)(unsafe.Pointer(&bytesVal[0]))
			cLen := C.uint(len(bytesVal))
			rc = C.RfcSetBytes(container, cName, bValue, cLen, &errorInfo)
		}
	case C.RFCTYPE_XSTRING:
		if bytesVal, ok := value.([]byte); ok && len(bytesVal) > 0 {
			bValue = (*C.SAP_RAW)(unsafe.Pointer(&bytesVal[0]))
			cLen := C.uint(len(bytesVal))
			rc = C.RfcSetXString(container, cName, bValue, cLen, &errorInfo)
		}
	case C.RFCTYPE_CHAR:
		cValue, err = fillString(asString(value))
		defer C.free(unsafe.Pointer(cValue))

		cLen := C.uint(C.GoStrlenU((*C.SAP_UTF16)(cValue)))
		rc = C.RfcSetChars(container, cName, (*C.RFC_CHAR)(cValue), cLen, &errorInfo)
	case C.RFCTYPE_STRING:
		cValue, err = fillString(asString(value))
		defer C.free(unsafe.Pointer(cValue))

		cLen := C.uint(C.GoStrlenU((*C.SAP_UTF16)(cValue)))
		rc = C.RfcSetString(container, cName, cValue, cLen, &errorInfo)
	case C.RFCTYPE_NUM:
		cValue, err = fillString(asString(value))
		defer C.free(unsafe.Pointer(cValue))

		cLen := C.uint(C.GoStrlenU((*C.SAP_UTF16)(cValue)))
		rc = C.RfcSetNum(container, cName, (*C.RFC_NUM)(cValue), cLen, &errorInfo)
	case C.RFCTYPE_FLOAT, C.RFCTYPE_BCD, C.RFCTYPE_DECF16, C.RFCTYPE_DECF34:
		var goVal string

		switch v := value.(type) {
		case float64:
			goVal = strconv.FormatFloat(v, 'g', -1, 64)
		case string:
			goVal = v

		default:
			goVal = fmt.Sprint(v)
		}

		cValue, err = fillString(goVal)
		defer C.free(unsafe.Pointer(cValue))

		cLen := C.uint(C.GoStrlenU((*C.SAP_UTF16)(cValue)))
		rc = C.RfcSetString(container, cName, cValue, cLen, &errorInfo)
	case C.RFCTYPE_INT1:
		switch v := value.(type) {
		case int:
			rc = C.RfcSetInt(container, cName, C.RFC_INT(v), &errorInfo)
		case uint:
			rc = C.RfcSetInt(container, cName, C.RFC_INT(v), &errorInfo)
		case uint8:
			rc = C.RfcSetInt(container, cName, C.RFC_INT(v), &errorInfo)
		default:
			return goRfcError("invalid INT1 type", nil)
		}
	case C.RFCTYPE_INT2, C.RFCTYPE_INT, C.RFCTYPE_INT8:
		switch v := value.(type) {
		case int:
			rc = C.RfcSetInt(container, cName, C.RFC_INT(v), &errorInfo)
		case int8:
			rc = C.RfcSetInt(container, cName, C.RFC_INT(v), &errorInfo)
		case int16:
			rc = C.RfcSetInt(container, cName, C.RFC_INT(v), &errorInfo)
		case int32:
			rc = C.RfcSetInt(container, cName, C.RFC_INT(v), &errorInfo)
		case int64:
			rc = C.RfcSetInt(container, cName, C.RFC_INT(v), &errorInfo)
		default:
			return goRfcError("invalid integer type", nil)
		}
	case C.RFCTYPE_DATE:
		cValue, err = fillString(value.(time.Time).Format("20060102"))
		defer C.free(unsafe.Pointer(cValue))

		rc = C.RfcSetDate(container, cName, (*C.RFC_CHAR)(cValue), &errorInfo)
	case C.RFCTYPE_TIME:
		cValue, err = fillString(value.(time.Time).Format("150405"))
		defer C.free(unsafe.Pointer(cValue))

		rc = C.RfcSetTime(container, cName, (*C.RFC_CHAR)(cValue), &errorInfo)
	case C.RFCTYPE_UTCLONG:
		cValue, err = fillString(asString(value))
		defer C.free(unsafe.Pointer(cValue))

		cLen := C.uint(C.GoStrlenU((*C.SAP_UTF16)(cValue)))
		rc = C.RfcSetString(container, cName, cValue, cLen, &errorInfo)
	default:
		var goName string

		goName, err = wrapString(cName, true)

		return rfcError(errorInfo, "Unknown RFC type %v when filling %v", cType, goName)
	}

	if rc != C.RFC_OK {
		var goName string

		goName, err = wrapString(cName, true)
		err = rfcError(errorInfo, "Could not fill %v of type %v", goName, cType)
	}

	return
}

func fillStructure(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_STRUCTURE_HANDLE, value any) (err error) {
	var errorInfo C.RFC_ERROR_INFO

	if m, ok := value.(map[string]any); ok {
		for fieldName, fieldValue := range m {
			if err := fillStructureField(typeDesc, container, fieldName, fieldValue); err != nil {
				return err
			}
		}

		return nil
	}

	s := reflect.ValueOf(value)
	switch s.Type().Kind() {
	case reflect.Map:
		iter := s.MapRange()
		valid := false
		for iter.Next() {
			key := iter.Key()
			if !valid {
				if key.Kind() != reflect.String {
					return rfcError(errorInfo, "Could not fill structure passed as map with non-string keys")
				}

				valid = true
			}

			fieldName := key.String()
			fieldValue := iter.Value().Interface()
			if err = fillStructureField(typeDesc, container, fieldName, fieldValue); err != nil {
				return
			}
		}
	case reflect.Struct:
		t := s.Type()
		numField := s.NumField()
		for i := range numField {
			fieldName := t.Field(i).Name
			fieldValue := s.Field(i).Interface()
			if err = fillStructureField(typeDesc, container, fieldName, fieldValue); err != nil {
				return
			}
		}
	default:
		err = fillStructureField(typeDesc, container, "", s.Interface())
	}

	return
}

func fillStructureField(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_STRUCTURE_HANDLE, fieldName string, fieldValue any) (err error) {
	var rc C.RFC_RC
	var errorInfo C.RFC_ERROR_INFO
	var fieldDesc C.RFC_FIELD_DESC

	cName, err := fillString(fieldName)
	defer C.free(unsafe.Pointer(cName))

	rc = C.RfcGetFieldDescByName(typeDesc, cName, &fieldDesc, &errorInfo)
	if rc != C.RFC_OK {
		return rfcError(errorInfo, "Could not get field description for \"%v\"", fieldName)
	}

	return fillVariable(fieldDesc._type, C.RFC_FUNCTION_HANDLE(container), (*C.SAP_UC)(&fieldDesc.name[0]), fieldValue, fieldDesc.typeDescHandle)
}

func fillTable(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_TABLE_HANDLE, lines any) (err error) {
	var errorInfo C.RFC_ERROR_INFO
	var lineHandle C.RFC_STRUCTURE_HANDLE

	switch rows := lines.(type) {
	case []map[string]any:
		for _, row := range rows {
			lineHandle = C.RfcAppendNewRow(container, &errorInfo)
			if lineHandle == nil {
				return rfcError(errorInfo, "Could not append new row to table")
			}

			err = fillStructure(typeDesc, lineHandle, row)
			if err != nil {
				return
			}
		}

		return nil
	case []any:
		for _, row := range rows {
			lineHandle = C.RfcAppendNewRow(container, &errorInfo)
			if lineHandle == nil {
				return rfcError(errorInfo, "Could not append new row to table")
			}

			err = fillStructure(typeDesc, lineHandle, row)
			if err != nil {
				return
			}
		}

		return nil
	}

	rv := reflect.ValueOf(lines)
	if rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice {
		return goRfcError("table parameter must be slice or array", nil)
	}

	for i := 0; i < rv.Len(); i++ {
		lineHandle = C.RfcAppendNewRow(container, &errorInfo)
		if lineHandle == nil {
			return rfcError(errorInfo, "Could not append new row to table")
		}

		err = fillStructure(typeDesc, lineHandle, rv.Index(i).Interface())
	}

	return
}

// ################################################################################
// # WRAPPER FUNCTIONS                                                            #
// ################################################################################
// # Wrapper functions take C values and return Go values
func wrapString(sapuc *C.SAP_UC, strip bool) (string, error) {
	return nWrapString(sapuc, C.uint(C.GoStrlenU((*C.SAP_UTF16)(sapuc))), strip)
}

// utf8BufPool reuses temporary byte slices for SAP UC → UTF-8 conversion,
// reducing GC pressure in the hot-path nWrapString.
var utf8BufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 1024)
		return &b
	},
}

func nWrapString(sapuc *C.SAP_UC, sapucLength C.uint, strip bool) (string, error) {
	var errorInfo C.RFC_ERROR_INFO
	var rc C.RFC_RC
	var resultLength C.uint

	if sapucLength == 0 {
		return "", nil
	}

	// Each SAP UC code unit can expand to at most 4 UTF-8 bytes + 1 for NUL.
	needed := uint(5*sapucLength + 1)

	// Grab a pooled buffer; grow it if the current capacity is insufficient.
	bufPtr := utf8BufPool.Get().(*[]byte)
	defer utf8BufPool.Put(bufPtr)

	if uint(cap(*bufPtr)) < needed {
		*bufPtr = make([]byte, needed)
	}

	buf := (*bufPtr)[:needed]
	utf8size := C.uint(needed)
	utf8Str := (*C.RFC_BYTE)(unsafe.Pointer(&buf[0]))

	rc = C.RfcSAPUCToUTF8(sapuc, C.uint(sapucLength), utf8Str, &utf8size, &resultLength, &errorInfo)
	if rc != C.RFC_OK {
		return "", fmt.Errorf("wrapString sapucLength %v utf8size %v", sapucLength, utf8size)
	}

	// Convert buf slice to Go string with a single allocation (no C.GoStringN).
	result := string(buf[:resultLength])
	if strip {
		result = strings.TrimRight(result, "\x00 ")
	}

	return result, nil
}

type rfcSDKError struct {
	Message       string
	Code          string
	Key           string
	AbapMsgClass  string
	AbapMsgType   string
	AbapMsgNumber string
	AbapMsgV1     string
	AbapMsgV2     string
	AbapMsgV3     string
	AbapMsgV4     string
}

func wrapError(errorInfo *C.RFC_ERROR_INFO) rfcSDKError {
	message, _ := wrapString(&errorInfo.message[0], true)
	code, _ := wrapString(C.RfcGetRcAsString(errorInfo.code), true)
	key, _ := wrapString(&errorInfo.key[0], true)
	abapMsgClass, _ := wrapString(&errorInfo.abapMsgClass[0], true)
	abapMsgType, _ := wrapString(&errorInfo.abapMsgType[0], true)
	abapMsgNumber, _ := wrapString((*C.SAP_UC)(&errorInfo.abapMsgNumber[0]), true)
	abapMsgV1, _ := wrapString(&errorInfo.abapMsgV1[0], true)
	abapMsgV2, _ := wrapString(&errorInfo.abapMsgV2[0], true)
	abapMsgV3, _ := wrapString(&errorInfo.abapMsgV3[0], true)
	abapMsgV4, _ := wrapString(&errorInfo.abapMsgV4[0], true)

	return rfcSDKError{message, code, key, abapMsgClass, abapMsgType, abapMsgNumber, abapMsgV1, abapMsgV2, abapMsgV3, abapMsgV4}
}

func (err rfcSDKError) String() string {
	var b strings.Builder

	b.Grow(
		len(err.Message) +
			len(err.Code) +
			len(err.Key) +
			len(err.AbapMsgClass) +
			len(err.AbapMsgType) +
			len(err.AbapMsgNumber) +
			len(err.AbapMsgV1) +
			len(err.AbapMsgV2) +
			len(err.AbapMsgV3) +
			len(err.AbapMsgV4) +
			32,
	)

	b.WriteString("rfcSDKError[")
	b.WriteString(err.Message)
	b.WriteString(", ")
	b.WriteString(err.Code)
	b.WriteString(", ")
	b.WriteString(err.Key)
	b.WriteString(", ")
	b.WriteString(err.AbapMsgClass)
	b.WriteString(", ")
	b.WriteString(err.AbapMsgType)
	b.WriteString(", ")
	b.WriteString(err.AbapMsgNumber)
	b.WriteString(", ")
	b.WriteString(err.AbapMsgV1)
	b.WriteString(", ")
	b.WriteString(err.AbapMsgV2)
	b.WriteString(", ")
	b.WriteString(err.AbapMsgV3)
	b.WriteString(", ")
	b.WriteString(err.AbapMsgV4)
	b.WriteByte(']')

	return b.String()
}

// ConnectionAttributes returned by getConnectionInfo() method
type ConnectionAttributes map[string]string

func wrapConnectionAttributes(attributes C.RFC_ATTRIBUTES, strip bool) (connAttr ConnectionAttributes, err error) {
	connAttr = make(map[string]string, 25)

	if err := putAttr(connAttr, "host", &attributes.host[0], 100, strip); err != nil {
		return nil, err
	}

	return
}

// FieldDescription type
type FieldDescription struct {
	Name      string
	FieldType string
	NucLength uint
	NucOffset uint
	UcLength  uint
	UcOffset  uint
	Decimals  uint
	TypeDesc  TypeDescription
}

// TypeDescription type
type TypeDescription struct {
	Name      string
	NucLength uint
	UcLength  uint
	Fields    []FieldDescription
}

func (t TypeDescription) String() string {
	var b strings.Builder

	b.WriteString(t.Name)
	b.WriteString(" NucLength=")
	b.WriteString(strconv.FormatUint(uint64(t.NucLength), 10))
	b.WriteString(" UcLength=")
	b.WriteString(strconv.FormatUint(uint64(t.UcLength), 10))

	return b.String()
}

func wrapTypeDescription(typeDesc C.RFC_TYPE_DESC_HANDLE) (goTypeDesc TypeDescription, err error) {
	var rc C.RFC_RC
	var errorInfo C.RFC_ERROR_INFO
	var fieldDesc C.RFC_FIELD_DESC
	var nucLength, ucLength C.uint
	var i, fieldCount C.uint

	typeName := (*C.SAP_UC)(C.malloc((C.size_t)(40 + 1)))
	*typeName = 0
	defer C.free(unsafe.Pointer(typeName))

	rc = C.RfcGetTypeName(typeDesc, (*C.RFC_CHAR)(typeName), &errorInfo)
	if rc != C.RFC_OK {
		return goTypeDesc, rfcError(errorInfo, "Failed getting type name")
	}

	name, err := wrapString(typeName, false)
	if err != nil {
		return
	}

	rc = C.RfcGetTypeLength(typeDesc, &nucLength, &ucLength, &errorInfo)
	if rc != C.RFC_OK {
		return goTypeDesc, rfcError(errorInfo, "Failed getting type(%v) length", name)
	}

	rc = C.RfcGetFieldCount(typeDesc, &fieldCount, &errorInfo)
	if rc != C.RFC_OK {
		return goTypeDesc, rfcError(errorInfo, "Failed getting field count")
	}

	goTypeDesc = TypeDescription{
		Name:      name,
		NucLength: uint(nucLength),
		UcLength:  uint(ucLength),
		Fields:    make([]FieldDescription, 0, fieldCount),
	}

	for i = 0; i < fieldCount; i++ {
		rc = C.RfcGetFieldDescByIndex(typeDesc, i, &fieldDesc, &errorInfo)
		if rc != C.RFC_OK {
			return goTypeDesc, rfcError(errorInfo, "Failed getting field by index(%v)", i)
		}

		var fieldName string
		var fieldType string

		fieldName, err = wrapString((*C.SAP_UC)(&fieldDesc.name[0]), false)
		fieldType, err = wrapString((*C.SAP_UC)(C.RfcGetTypeAsString(fieldDesc._type)), false)
		if err != nil {
			return
		}

		goFieldDesc := FieldDescription{
			Name:      fieldName,
			FieldType: fieldType,
			NucLength: uint(fieldDesc.nucLength),
			NucOffset: uint(fieldDesc.nucOffset),
			UcLength:  uint(fieldDesc.ucLength),
			UcOffset:  uint(fieldDesc.ucOffset),
			Decimals:  uint(fieldDesc.decimals),
		}

		if fieldDesc.typeDescHandle != nil {
			goFieldDesc.TypeDesc, err = wrapTypeDescription(fieldDesc.typeDescHandle)
			if err != nil {
				return
			}
		}

		goTypeDesc.Fields = append(goTypeDesc.Fields, goFieldDesc)
	}

	return
}

// ParameterDescription type
type ParameterDescription struct {
	Name          string
	ParameterType string
	Direction     string
	NucLength     uint
	UcLength      uint
	Decimals      uint
	DefaultValue  string
	ParameterText string
	Optional      bool
	TypeDesc      TypeDescription
	// ExtendedDescription any //This field can be used by the application programmer (i.e. you) to store arbitrary extra information.
}

func (paramDesc ParameterDescription) String() string {
	var b strings.Builder

	b.WriteString("paramDesc(name= ")
	b.WriteString(paramDesc.Name)

	b.WriteString(", paramType= ")
	b.WriteString(paramDesc.ParameterType)

	b.WriteString(", dir= ")
	b.WriteString(paramDesc.Direction)

	b.WriteString(", nucLen= ")
	b.WriteString(strconv.FormatUint(uint64(paramDesc.NucLength), 10))

	b.WriteString(", ucLen= ")
	b.WriteString(strconv.FormatUint(uint64(paramDesc.UcLength), 10))

	b.WriteString(", dec= ")
	b.WriteString(strconv.FormatUint(uint64(paramDesc.Decimals), 10))

	b.WriteString(", defValue= ")
	b.WriteString(paramDesc.DefaultValue)

	b.WriteString(", paramText= ")
	b.WriteString(paramDesc.ParameterText)

	b.WriteString(", optional= ")
	b.WriteString(strconv.FormatBool(paramDesc.Optional))

	b.WriteString(", typeDesc= ")
	b.WriteString(paramDesc.TypeDesc.String())

	b.WriteByte(')')

	return b.String()
}

// FunctionDescription type
type FunctionDescription struct {
	Name       string
	Parameters []ParameterDescription
}

func (funcDesc FunctionDescription) String() string {
	var b strings.Builder

	b.WriteString("FunctionDescription:\n Name: ")
	b.WriteString(funcDesc.Name)
	b.WriteString("\n Parameters:\n")

	for i := range funcDesc.Parameters {
		b.WriteString("    ")
		b.WriteString(funcDesc.Parameters[i].String())
		b.WriteByte('\n')
	}

	return b.String()
}

func wrapFunctionDescription(funcDesc C.RFC_FUNCTION_DESC_HANDLE) (goFuncDesc FunctionDescription, err error) {
	var rc C.RFC_RC
	var errorInfo C.RFC_ERROR_INFO
	var funcName C.RFC_ABAP_NAME
	var i, paramCount C.uint
	var paramDesc C.RFC_PARAMETER_DESC

	rc = C.RfcGetFunctionName(funcDesc, &funcName[0], &errorInfo)
	if rc != C.RFC_OK {
		return goFuncDesc, rfcError(errorInfo, "Failed getting function name")
	}

	goFuncName, err := wrapString((*C.SAP_UC)(&funcName[0]), false)
	if err != nil {
		return
	}

	rc = C.RfcGetParameterCount(funcDesc, &paramCount, &errorInfo)
	if rc != C.RFC_OK {
		return goFuncDesc, rfcError(errorInfo, "Failed getting function(%v) parameter count", goFuncName)
	}

	// NOTE: FunctionDescription is created after fetching paramCount so the
	// slice capacity is correct and avoids repeated reallocations on append.
	goFuncDesc = FunctionDescription{
		Name:       goFuncName,
		Parameters: make([]ParameterDescription, 0, paramCount),
	}

	for i = 0; i < paramCount; i++ {
		rc = C.RfcGetParameterDescByIndex(funcDesc, i, &paramDesc, &errorInfo)
		if rc != C.RFC_OK {
			return goFuncDesc, rfcError(errorInfo, "Failed getting function(%v) parameter description by index(%v)", goFuncName, i)
		}

		optional := true
		if paramDesc.optional == 0 {
			optional = false
		}

		var paramName string
		var paramType string
		var paramDir string
		var paramDefaultVal string
		var paramText string

		paramName, err = wrapString((*C.SAP_UC)(&paramDesc.name[0]), false)
		paramType, err = wrapString((*C.SAP_UC)(C.RfcGetTypeAsString(paramDesc._type)), false)
		paramDir, err = wrapString((*C.SAP_UC)(C.RfcGetDirectionAsString(paramDesc.direction)), false)
		paramDefaultVal, err = wrapString((*C.SAP_UC)(&paramDesc.defaultValue[0]), false)
		paramText, err = wrapString((*C.SAP_UC)(&paramDesc.parameterText[0]), false)
		if err != nil {
			return
		}

		goParamDesc := ParameterDescription{
			Name:          paramName,
			ParameterType: paramType,
			Direction:     paramDir,
			NucLength:     uint(paramDesc.nucLength),
			UcLength:      uint(paramDesc.ucLength),
			Decimals:      uint(paramDesc.decimals),
			DefaultValue:  paramDefaultVal,
			ParameterText: paramText,
			Optional:      optional,
		}

		if paramDesc.typeDescHandle != nil {
			goParamDesc.TypeDesc, err = wrapTypeDescription(paramDesc.typeDescHandle)
			if err != nil {
				return
			}
		}

		goFuncDesc.Parameters = append(goFuncDesc.Parameters, goParamDesc)
	}

	return
}

func wrapVariable(cType C.RFCTYPE, container C.RFC_FUNCTION_HANDLE, cName *C.SAP_UC, cLen C.uint, typeDesc C.RFC_TYPE_DESC_HANDLE, strip bool) (result any, err error) {
	var rc C.RFC_RC
	var errorInfo C.RFC_ERROR_INFO
	var structure C.RFC_STRUCTURE_HANDLE
	var table C.RFC_TABLE_HANDLE
	var charValue *C.RFC_CHAR
	var stringValue *C.SAP_UC
	var numValue *C.RFC_NUM
	var byteValue *C.SAP_RAW
	var floatValue C.RFC_FLOAT
	var intValue C.RFC_INT
	var int1Value C.RFC_INT1
	var int2Value C.RFC_INT2
	var int8Value C.RFC_INT8
	var resultLen, strLen C.uint

	switch cType {
	case C.RFCTYPE_STRUCTURE:
		rc = C.RfcGetStructure(container, cName, &structure, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting structure")
		}

		return wrapStructure(typeDesc, structure, strip)
	case C.RFCTYPE_TABLE:
		rc = C.RfcGetTable(container, cName, &table, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting table")
		}

		return wrapTable(typeDesc, table, strip)
	case C.RFCTYPE_CHAR:
		charValue = (*C.RFC_CHAR)(C.GoMallocU(cLen))
		defer C.free(unsafe.Pointer(charValue))

		rc = C.RfcGetChars(container, cName, charValue, cLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting chars")
		}

		return nWrapString((*C.SAP_UC)(charValue), cLen, strip)
	case C.RFCTYPE_STRING:
		rc = C.RfcGetStringLength(container, cName, &strLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting string length")
		}

		stringValue = (*C.SAP_UC)(C.GoMallocU(strLen + 1))
		defer C.free(unsafe.Pointer(stringValue))

		rc = C.RfcGetString(container, cName, stringValue, strLen+1, &resultLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting string")
		}

		return wrapString(stringValue, strip)
	case C.RFCTYPE_NUM:
		numValue = (*C.RFC_NUM)(C.GoMallocU(cLen))
		defer C.free(unsafe.Pointer(numValue))

		rc = C.RfcGetNum(container, cName, numValue, cLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting num")
		}

		return nWrapString((*C.SAP_UC)(numValue), cLen, strip)
	case C.RFCTYPE_BYTE:
		byteValue = (*C.SAP_RAW)(C.malloc(C.size_t(cLen)))
		defer C.free(unsafe.Pointer(byteValue))

		rc = C.RfcGetBytes(container, cName, byteValue, cLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting bytes")
		}

		return C.GoBytes(unsafe.Pointer(byteValue), C.int(cLen)), err
	case C.RFCTYPE_XSTRING:
		rc = C.RfcGetStringLength(container, cName, &strLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting xstring length")
		}

		byteValue = (*C.SAP_RAW)(C.malloc(C.size_t(strLen)))
		defer C.free(unsafe.Pointer(byteValue))

		rc = C.RfcGetXString(container, cName, byteValue, strLen, &resultLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting xstring")
		}

		return C.GoBytes(unsafe.Pointer(byteValue), C.int(strLen)), err
	case C.RFCTYPE_BCD:
		// Each BCD nibble yields at most one decimal digit; the representation
		// also needs a sign char and a decimal separator => (2*cLen)+1.
		// Allocating this exact upper-bound upfront avoids the free-reallocate
		// retry that the old code required.
		strLen = 2*cLen + 1
		stringValue = C.GoMallocU(strLen + 1)
		defer C.free(unsafe.Pointer(stringValue))

		rc = C.RfcGetString(container, cName, stringValue, strLen+1, &resultLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting BCD")
		}

		return wrapString(stringValue, strip)
	case C.RFCTYPE_DECF16, C.RFCTYPE_DECF34:
		// Upper bound: (2*cLen)+1 for digits/sign/separator, +9 for exponent.
		// Allocating this exact upper-bound upfront avoids the free-reallocate
		// retry that the old code required.
		strLen = 2*cLen + 10
		stringValue = C.GoMallocU(strLen + 1)
		defer C.free(unsafe.Pointer(stringValue))

		rc = C.RfcGetString(container, cName, stringValue, strLen+1, &resultLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting DECF")
		}

		return wrapString(stringValue, strip)
	case C.RFCTYPE_FLOAT:
		rc = C.RfcGetFloat(container, cName, &floatValue, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting FLOAT")
		}

		return float64(floatValue), err
	case C.RFCTYPE_INT:
		rc = C.RfcGetInt(container, cName, &intValue, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting INT")
		}

		return int32(intValue), err
	case C.RFCTYPE_INT1:
		rc = C.RfcGetInt1(container, cName, &int1Value, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting INT1")
		}

		return uint8(int1Value), err
	case C.RFCTYPE_INT2:
		rc = C.RfcGetInt2(container, cName, &int2Value, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting INT2")
		}

		return int16(int2Value), err
	case C.RFCTYPE_INT8:
		rc = C.RfcGetInt8(container, cName, &int8Value, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting INT8")
		}

		return int64(int8Value), err
	case C.RFCTYPE_DATE:
		// Use a Go stack-allocated array instead of C.malloc to avoid the
		// heap round-trip for this small, fixed-size (8-element) buffer.
		var dateBuf [8]C.RFC_CHAR

		rc = C.RfcGetDate(container, cName, &dateBuf[0], &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting DATE")
		}

		value, _ := nWrapString((*C.SAP_UC)(&dateBuf[0]), 8, false)
		if value == "00000000" || ' ' == value[1] || err != nil {
			return
		}

		goDate, err := time.Parse("20060102", value)
		if err != nil {
			return nil, goRfcError("Error parsing ABAP RFC_DATE field", err)
		}

		return goDate, err
	case C.RFCTYPE_TIME:
		// Use a Go stack-allocated array instead of C.malloc to avoid the
		// heap round-trip for this small, fixed-size (6-element) buffer.
		var timeBuf [6]C.RFC_CHAR

		rc = C.RfcGetTime(container, cName, &timeBuf[0], &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting TIME")
		}

		value, _ := nWrapString((*C.SAP_UC)(&timeBuf[0]), 6, false)
		goTime, err := time.Parse("150405", value)
		if err != nil {
			return nil, goRfcError("Error parsing ABAP RFC_TIME field", err)
		}

		return goTime, err
	case C.RFCTYPE_UTCLONG:
		resultLen = 0
		strLen = 27

		stringValue = (*C.SAP_UC)(C.GoMallocU(strLen + 1))
		defer C.free(unsafe.Pointer(stringValue))

		rc = C.RfcGetString(container, cName, stringValue, strLen+1, &resultLen, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting UTCLONG")
		}

		utc, _ := nWrapString(stringValue, strLen, strip)

		return utc[:19] + "." + utc[20:], err
	}

	return result, rfcError(errorInfo, "Unknown RFC type %d when wrapping variable", cType)
}

func wrapStructure(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_STRUCTURE_HANDLE, strip bool) (result map[string]any, err error) {
	var errorInfo C.RFC_ERROR_INFO
	var i, fieldCount C.uint
	var fieldDesc C.RFC_FIELD_DESC

	rc := C.RfcGetFieldCount(typeDesc, &fieldCount, &errorInfo)
	if rc != C.RFC_OK {
		return result, rfcError(errorInfo, "Failed getting field count")
	}

	result = make(map[string]any, int(fieldCount))
	for i = 0; i < fieldCount; i++ {
		rc = C.RfcGetFieldDescByIndex(typeDesc, i, &fieldDesc, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting field description by index(%v)", i)
		}

		var fieldName string

		fieldName, err = wrapString((*C.SAP_UC)(&fieldDesc.name[0]), strip)
		if err != nil {
			return
		}

		result[fieldName], err = wrapVariable(fieldDesc._type, C.RFC_FUNCTION_HANDLE(container), (*C.SAP_UC)(&fieldDesc.name[0]), fieldDesc.nucLength, fieldDesc.typeDescHandle, strip)
		if err != nil {
			return
		}
	}

	return
}

func wrapTable(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_TABLE_HANDLE, strip bool) (result []any, err error) {
	var errorInfo C.RFC_ERROR_INFO
	var i, lines C.uint

	rc := C.RfcGetRowCount(container, &lines, &errorInfo)
	if rc != C.RFC_OK {
		return result, rfcError(errorInfo, "Failed getting row count")
	}

	result = make([]any, lines)
	for i = 0; i < lines; i++ {
		rc = C.RfcMoveTo(container, i, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting moving cursor to index(%v)", i)
		}

		structHandle := C.RfcGetCurrentRow(container, &errorInfo)

		var line map[string]any

		line, err = wrapStructure(typeDesc, structHandle, strip)
		if err != nil {
			return
		}

		result[i] = line
	}
	return
}

func wrapResult(funcDesc C.RFC_FUNCTION_DESC_HANDLE, container C.RFC_FUNCTION_HANDLE, filterParameterDirection C.RFC_DIRECTION, strip bool) (result map[string]any, err error) {
	var errorInfo C.RFC_ERROR_INFO
	var i, paramCount C.uint
	var paramDesc C.RFC_PARAMETER_DESC

	rc := C.RfcGetParameterCount(funcDesc, &paramCount, &errorInfo)
	if rc != C.RFC_OK {
		return result, rfcError(errorInfo, "Failed getting parameter count")
	}

	result = make(map[string]any, int(paramCount))
	for i = 0; i < paramCount; i++ {
		rc = C.RfcGetParameterDescByIndex(funcDesc, i, &paramDesc, &errorInfo)
		if rc != C.RFC_OK {
			return result, rfcError(errorInfo, "Failed getting parameter decription by index(%v)", i)
		}

		if paramDesc.direction != filterParameterDirection {
			var fieldName string

			fieldName, err = wrapString((*C.SAP_UC)(&paramDesc.name[0]), strip)
			if err != nil {
				return
			}

			result[fieldName], err = wrapVariable(paramDesc._type, container, (*C.SAP_UC)(&paramDesc.name[0]), paramDesc.nucLength, paramDesc.typeDescHandle, strip)
			if err != nil {
				return
			}
		}
	}

	return
}

//################################################################################
//# NW RFC LIB FUNCTIONALITY                                                     #
//################################################################################

// GetNWRFCLibVersion returnd the major version, minor version and patchlevel of the SAP NetWeaver RFC library used.
func GetNWRFCLibVersion() (major, minor, patchlevel uint) {
	var cmaj, cmin, cpatch C.uint

	C.RfcGetVersion(&cmaj, &cmin, &cpatch)

	major = uint(cmaj)
	minor = uint(cmin)
	patchlevel = uint(cpatch)

	return
}

//################################################################################
//# CONNECTION                                                                   #
//################################################################################

// Connection Parameters
type ConnectionParameters map[string]string

// Client Connection
type Connection struct {
	handle             C.RFC_CONNECTION_HANDLE
	rstrip             bool
	returnImportParams bool
	alive              bool
	paramCount         C.uint
	connParams         []C.RFC_CONNECTION_PARAMETER
	connectionParams   ConnectionParameters
	// tHandle C.RFC_TRANSACTION_HANDLE
	// active_transaction bool
	// uHandle C.RFC_UNIT_HANDLE
	// active_unit bool
}

func connectionFinalizer(conn *Connection) {
	for _, connParam := range conn.connParams {
		C.free(unsafe.Pointer(connParam.name))
		C.free(unsafe.Pointer(connParam.value))
	}
}

// ConnectionFromParams creates a new connection with the given connection parameters and tries to open it.
// Returns the connection if successfull, otherwise nil.
func ConnectionFromParams(connectionParams ConnectionParameters) (conn *Connection, err error) {
	conn = new(Connection)

	conn.handle = nil
	conn.rstrip = true
	conn.returnImportParams = false
	conn.alive = false

	runtime.SetFinalizer(conn, connectionFinalizer)

	conn.paramCount = C.uint(len(connectionParams))
	conn.connectionParams = connectionParams
	conn.connParams = make([]C.RFC_CONNECTION_PARAMETER, conn.paramCount)

	i := 0
	for name, value := range conn.connectionParams {
		conn.connParams[i].name, err = fillString(name)
		if err != nil {
			return nil, err
		}

		conn.connParams[i].value, err = fillString(value)
		if err != nil {
			return nil, err
		}

		i++
	}

	err = conn.Open()
	if err != nil {
		return nil, err
	}

	return
}

// ConnectionFromDest creates a new connection with just the dest system id.
func ConnectionFromDest(dest string) (conn *Connection, err error) {
	return ConnectionFromParams(ConnectionParameters{"dest": dest})
}

// RStrip sets rstrip of the given connection to the passed parameter and returns the connection
// right strips strings returned from RFC call (default is true)
func (conn *Connection) RStrip(rstrip bool) *Connection {
	conn.rstrip = rstrip

	return conn
}

// ReturnImportParams sets returnImportParams of the given connection to the passed parameter and returns the connection
func (conn *Connection) ReturnImportParams(returnImportParams bool) *Connection {
	conn.returnImportParams = returnImportParams

	return conn
}

// Alive returns true if the connection is open else returns false.
func (conn *Connection) Alive() bool {
	return conn.alive
}

// Close closes the connection and sets alive to false.
func (conn *Connection) Close() (err error) {
	var errorInfo C.RFC_ERROR_INFO
	if conn.alive {
		conn.alive = false

		rc := C.RfcCloseConnection(conn.handle, &errorInfo)
		if rc != C.RFC_OK {
			return rfcError(errorInfo, "Connection could not be closed")
		}
	}

	return
}

// Open opens the connection and sets alive to true.
func (conn *Connection) Open() (err error) {
	var errorInfo C.RFC_ERROR_INFO

	conn.handle = C.RfcOpenConnection(&conn.connParams[0], conn.paramCount, &errorInfo)
	if conn.handle == nil {
		return rfcError(errorInfo, "Connection could not be opened")
	}

	conn.alive = true

	return
}

// Reopen closes and opens the connection.
func (conn *Connection) Reopen() (err error) {
	err = conn.Close()
	if err != nil {
		return
	}

	err = conn.Open()

	return
}

// Ping pings the server which the client is connected to and does nothing with the error if one occurs.
func (conn *Connection) Ping() (err error) {
	var errorInfo C.RFC_ERROR_INFO
	if !conn.alive {
		err = conn.Open()
		if err != nil {
			return
		}
	}

	rc := C.RfcPing(conn.handle, &errorInfo)
	if rc != C.RFC_OK {
		return rfcError(errorInfo, "Server could not be pinged")
	}

	return
}

// GetConnectionAttributes returns the wrapped connection attributes of the connection.
func (conn *Connection) GetConnectionAttributes() (connAttr ConnectionAttributes, err error) {
	var errorInfo C.RFC_ERROR_INFO
	var attributes C.RFC_ATTRIBUTES

	rc := C.RfcGetConnectionAttributes(conn.handle, &attributes, &errorInfo)
	if rc != C.RFC_OK || errorInfo.code != C.RFC_OK {
		return nil, rfcError(errorInfo, "Could not get connection attributes")
	}

	return wrapConnectionAttributes(attributes, conn.rstrip)
}

// GetFunctionDescription returns the wrapped function description of the given function.
func (conn *Connection) GetFunctionDescription(goFuncName string) (goFuncDesc FunctionDescription, err error) {
	var errorInfo C.RFC_ERROR_INFO

	funcName, err := fillString(goFuncName)
	defer C.free(unsafe.Pointer(funcName))
	if err != nil {
		return
	}

	if !conn.alive {
		err = conn.Open()
		if err != nil {
			return
		}
	}

	funcDesc := C.RfcGetFunctionDesc(conn.handle, funcName, &errorInfo)
	if funcDesc == nil {
		return goFuncDesc, rfcError(errorInfo, "Could not get function description for \"%v\"", goFuncName)
	}

	return wrapFunctionDescription(funcDesc)
}

// Call calls the given function with the given parameters and wraps the results returned.
func (conn *Connection) Call(goFuncName string, params any) (result map[string]any, err error) {
	if !conn.alive {
		return nil, goRfcError("Call() method requires an open connection", nil)
	}

	var errorInfo C.RFC_ERROR_INFO

	funcName, err := fillString(goFuncName)

	defer C.free(unsafe.Pointer(funcName))
	if err != nil {
		return
	}

	if !conn.alive {
		err = conn.Open()
		if err != nil {
			return
		}
	}

	funcDesc := C.RfcGetFunctionDesc(conn.handle, funcName, &errorInfo)
	if funcDesc == nil {
		return result, rfcError(errorInfo, "Could not get function description for \"%v\"", funcName)
	}

	funcCont := C.RfcCreateFunction(funcDesc, &errorInfo)
	if funcCont == nil {
		return result, rfcError(errorInfo, "Could not create function")
	}

	defer C.RfcDestroyFunction(funcCont, nil)

	if m, ok := params.(map[string]any); ok {
		for k, v := range m {
			if err := fillFunctionParameter(funcDesc, funcCont, k, v); err != nil {
				return nil, err
			}
		}
	} else {
		paramsValue := reflect.ValueOf(params)
		switch paramsValue.Type().Kind() {
		case reflect.Map:
			iter := paramsValue.MapRange()
			valid := false
			for iter.Next() {
				key := iter.Key()
				if key.Kind() == reflect.String {
					valid = true
				}

				if !valid {
					return result, rfcError(errorInfo, "Could not fill parameters passed as map with non-string keys")
				}

				fieldName := key.String()
				fieldValue := iter.Value().Interface()
				err = fillFunctionParameter(funcDesc, funcCont, fieldName, fieldValue)
				if err != nil {
					return
				}
			}
		case reflect.Struct:
			for i := 0; i < paramsValue.NumField(); i++ {
				fieldName := paramsValue.Type().Field(i).Name
				fieldValue := paramsValue.Field(i).Interface()

				err = fillFunctionParameter(funcDesc, funcCont, fieldName, fieldValue)
				if err != nil {
					return
				}
			}
		default:
			return result, rfcError(errorInfo, "Parameters can only be passed as types map[string]any or go-structures")
		}
	}

	rc := C.RfcInvoke(conn.handle, funcCont, &errorInfo)

	if rc != C.RFC_OK {
		return result, rfcError(errorInfo, "Could not invoke function \"%v\"", goFuncName)
	}

	if conn.returnImportParams {
		return wrapResult(funcDesc, funcCont, (C.RFC_DIRECTION)(0), conn.rstrip)
	}

	return wrapResult(funcDesc, funcCont, C.RFC_IMPORT, conn.rstrip)
}

func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	case int:
		return strconv.Itoa(s)
	case int8:
		return strconv.FormatInt(int64(s), 10)
	case int16:
		return strconv.FormatInt(int64(s), 10)
	case int32:
		return strconv.FormatInt(int64(s), 10)
	case int64:
		return strconv.FormatInt(s, 10)
	case uint:
		return strconv.FormatUint(uint64(s), 10)
	case uint8:
		return strconv.FormatUint(uint64(s), 10)
	case uint16:
		return strconv.FormatUint(uint64(s), 10)
	case uint32:
		return strconv.FormatUint(uint64(s), 10)
	case uint64:
		return strconv.FormatUint(s, 10)
	case float32:
		return strconv.FormatFloat(float64(s), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(s, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	default:
		return fmt.Sprint(v)
	}
}

func putAttr(m map[string]string, key string, ptr *C.SAP_UC, length C.uint, strip bool) error {
	v, err := nWrapString(ptr, length, strip)
	if err != nil {
		return err
	}

	m[key] = v

	return nil
}
