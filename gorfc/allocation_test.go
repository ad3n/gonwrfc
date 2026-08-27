package gorfc

import "testing"

var stringResult string

func TestDescriptionStringAllocations(t *testing.T) {
	typeDesc := TypeDescription{Name: "Z_ITEM", NucLength: 42, UcLength: 84}
	paramDesc := ParameterDescription{
		Name:          "ITEM",
		ParameterType: "RFCTYPE_STRUCTURE",
		Direction:     "RFC_IMPORT",
		NucLength:     42,
		UcLength:      84,
		Decimals:      2,
		DefaultValue:  "0",
		ParameterText: "Item",
		Optional:      true,
		TypeDesc:      typeDesc,
	}
	funcDesc := FunctionDescription{Name: "Z_TEST", Parameters: []ParameterDescription{paramDesc, paramDesc}}
	sdkErr := rfcSDKError{Message: "message", Code: "code", Key: "key"}
	rfcErr := RfcError{Description: "description", ErrorInfo: sdkErr}

	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{
			"type",
			typeDesc.String,
			"Z_ITEM NucLength=42 UcLength=84",
		},
		{
			"parameter",
			paramDesc.String,
			"paramDesc(name= ITEM, paramType= RFCTYPE_STRUCTURE, dir= RFC_IMPORT, nucLen= 42, ucLen= 84, dec= 2, defValue= 0, paramText= Item, optional= true, typeDesc= Z_ITEM NucLength=42 UcLength=84)",
		},
		{
			"function",
			funcDesc.String,
			"FunctionDescription:\n Name: Z_TEST\n Parameters:\n" +
				"    paramDesc(name= ITEM, paramType= RFCTYPE_STRUCTURE, dir= RFC_IMPORT, nucLen= 42, ucLen= 84, dec= 2, defValue= 0, paramText= Item, optional= true, typeDesc= Z_ITEM NucLength=42 UcLength=84)\n" +
				"    paramDesc(name= ITEM, paramType= RFCTYPE_STRUCTURE, dir= RFC_IMPORT, nucLen= 42, ucLen= 84, dec= 2, defValue= 0, paramText= Item, optional= true, typeDesc= Z_ITEM NucLength=42 UcLength=84)\n",
		},
		{
			"sdk error",
			sdkErr.String,
			"rfcSDKError[message, code, key, , , , , , , ]",
		},
		{
			"RFC error",
			rfcErr.Error,
			"NWRFC SDK error: description | rfcSDKError[message, code, key, , , , , , , ]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(); got != tt.want {
				t.Fatalf("unexpected output\n got: %q\nwant: %q", got, tt.want)
			}

			allocs := testing.AllocsPerRun(100, func() {
				stringResult = tt.fn()
			})
			if allocs > 1 {
				t.Fatalf("String() allocated %.0f times; want at most the result allocation", allocs)
			}
		})
	}
}
