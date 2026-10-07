package utils

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

type testEmbeddedSt struct{}

type testStCorrect1 struct {
	testEmbeddedSt
}

type testStCorrect2 struct {
	testEmbeddedSt string
}

type testStFail struct {
}

// PointerMethod is a no-op pointer-receiver method that TestHasMethod looks up by name on testStCorrect1. It takes
// no parameters and returns nothing.
func (t *testStCorrect1) PointerMethod() {

}

// Method is a no-op pointer-receiver method that TestHasMethod looks up by name on testStCorrect1. It takes no
// parameters and returns nothing.
func (t *testStCorrect1) Method() {

}

// TestHasMethod verifies that HasMethod finds pointer-receiver methods on both a testStCorrect1 value and pointer,
// and reports false for a testStFail value and pointer, which define no methods.
func TestHasMethod(t *testing.T) {
	st1 := testStCorrect1{}
	st1p := &testStCorrect1{}
	st2 := testStFail{}
	st2p := &testStFail{}

	_ = st1.testEmbeddedSt
	_ = st1p.testEmbeddedSt

	if !HasMethod(st1, "Method") {
		t.Fatal()
	}
	if !HasMethod(st1, "PointerMethod") {
		t.Fatal()
	}
	if !HasMethod(st1p, "Method") {
		t.Fatal()
	}
	if !HasMethod(st1p, "PointerMethod") {
		t.Fatal()
	}
	if HasMethod(st2, "Method") {
		t.Fatal()
	}
	if HasMethod(st2, "PointerMethod") {
		t.Fatal()
	}
	if HasMethod(st2p, "Method") {
		t.Fatal()
	}
	if HasMethod(st2p, "PointerMethod") {
		t.Fatal()
	}
}

// TestHasField verifies that HasField finds an embedded field and a named unexported field on struct values and
// pointers, and reports false for a struct value and pointer that have no such field.
func TestHasField(t *testing.T) {
	t.Parallel()

	st1 := testStCorrect1{}
	st1p := &testStCorrect1{}
	st2 := testStCorrect2{}
	st2p := &testStCorrect2{}
	st3 := testStFail{}
	st3p := &testStFail{}

	_ = st2.testEmbeddedSt

	if !HasField(st1, "testEmbeddedSt") {
		t.Fatal()
	}
	if !HasField(st1p, "testEmbeddedSt") {
		t.Fatal()
	}
	if !HasField(st2, "testEmbeddedSt") {
		t.Fatal()
	}
	if !HasField(st2p, "testEmbeddedSt") {
		t.Fatal()
	}
	if HasField(st3, "testEmbeddedSt") {
		t.Fatal()
	}
	if HasField(st3p, "testEmbeddedSt") {
		t.Fatal()
	}
}

// TestIsPtr verifies that IsPtr returns true for a pointer to a struct and false for a struct value.
func TestIsPtr(t *testing.T) {
	vp := &struct{}{}
	vt := struct{}{}

	if !IsPtr(vp) {
		t.Fatal()
	}
	if IsPtr(vt) {
		t.Fatal()
	}
}

// testFoo is an empty function whose fully qualified name is resolved by TestGetFuncName and ExampleGetFuncName.
// It takes no parameters and returns nothing.
func testFoo() {}

// TestGetFuncName verifies that GetFuncName returns the fully qualified name
// "github.com/Laisky/go-utils/v6.testFoo" for the testFoo function value.
func TestGetFuncName(t *testing.T) {
	t.Parallel()

	if name := GetFuncName(testFoo); name != "github.com/Laisky/go-utils/v6.testFoo" {
		t.Fatalf("want `testFoo`, got `%v`", name)
	}
}

// ExampleGetFuncName demonstrates obtaining the package-qualified name of a function value with GetFuncName.
func ExampleGetFuncName() {
	GetFuncName(testFoo) // "github.com/Laisky/go-utils.testFoo"
}

// TestReflectSet verifies that reflect can assign string values to every field of structs reached through
// pointers in a slice without panicking, and logs the resulting structs.
func TestReflectSet(t *testing.T) {
	t.Parallel()

	type st struct{ A, B string }
	ss := []*st{{}, {}}
	nFields := reflect.ValueOf(ss[0]).Elem().NumField()
	vs := [][]string{{"x1", "y1"}, {"x2", "y2"}}

	for i, s := range ss {
		for j := 0; j < nFields; j++ {
			// if reflect.ValueOf(s).Type() != reflect.Ptr {
			// 	sp = &s
			// }
			reflect.ValueOf(s).Elem().Field(j).Set(reflect.ValueOf(vs[i][j]))
		}
	}

	t.Logf("s0: %+v", ss[0])
	t.Logf("s1: %+v", ss[1])
	// t.Error()
}

// ExampleSetStructFieldsBySlice demonstrates filling the fields of a slice of struct pointers, in field order,
// from a matching slice of string rows, logging the error and returning early if SetStructFieldsBySlice fails.
func ExampleSetStructFieldsBySlice() {
	type ST struct{ A, B string }
	var (
		err error
		ss  = []*ST{{}, {}}
		vs  = [][]string{
			{"x0", "y0"},
			{"x1", "y1"},
		}
	)
	if err = SetStructFieldsBySlice(ss, vs); err != nil {
		log.Shared.Error("set struct val", zap.Error(err))
		return
	}

	fmt.Printf("%+v\n", ss)
	// ss = []*ST{{A: "x0", B: "y0"}, {A: "x1", B: "y1"}}
}

// TestSetStructFieldsBySlice verifies that SetStructFieldsBySlice assigns row values to struct fields in order,
// leaves fields unset for empty or short rows, ignores values beyond the field count, leaves structs without a
// matching row untouched, and works for slices of both struct pointers and struct values.
func TestSetStructFieldsBySlice(t *testing.T) {
	t.Parallel()

	type ST struct{ A, B string }
	var (
		err error
		ss  = []*ST{
			{},
			{},
			{},
			{},
			{},
			{},
		}
		vs = [][]string{
			{"x0", "y0"},       // 0
			{"x1", "y1"},       // 1
			{},                 // 2
			{"x3", "y3", "z3"}, // 3
			{"x4"},             // 4
		}
	)
	if err = SetStructFieldsBySlice(ss, vs); err != nil {
		t.Fatalf("%+v", err)
	}

	t.Logf("s0: %+v", ss[0])
	t.Logf("s1: %+v", ss[1])
	t.Logf("s2: %+v", ss[2])
	t.Logf("s3: %+v", ss[3])
	t.Logf("s4: %+v", ss[4])
	t.Logf("s5: %+v", ss[5])

	if ss[0].A != "x0" ||
		ss[0].B != "y0" ||
		ss[1].A != "x1" ||
		ss[1].B != "y1" ||
		ss[2].A != "" ||
		ss[2].B != "" ||
		ss[3].A != "x3" ||
		ss[3].B != "y3" ||
		ss[4].A != "x4" ||
		ss[4].B != "" ||
		ss[5].A != "" ||
		ss[5].B != "" {
		t.Fatalf("incorrect")
	}

	// non-pointer struct
	ss2 := []ST{
		{},
		{},
	}
	if err = SetStructFieldsBySlice(ss2, vs); err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("s0: %+v", ss2[0])
	t.Logf("s1: %+v", ss2[1])
	if ss2[0].A != "x0" ||
		ss2[0].B != "y0" ||
		ss2[1].A != "x1" ||
		ss2[1].B != "y1" {
		t.Fatalf("incorrect")
	}
}

// TestGetStructFieldByName verifies that GetStructFieldByName, on both a struct value and a struct pointer, returns
// the values of string, pointer, and int fields, and returns nil for a missing field and for a nil pointer field.
func TestGetStructFieldByName(t *testing.T) {
	t.Parallel()

	type foo struct {
		A string
		B *string
		C int
		E *string
	}

	s := "2"

	f := foo{"1", &s, 2, nil}
	if v := GetStructFieldByName(f, "A"); v.(string) != "1" {
		t.Fatalf("got %+v", v)
	}
	if v := GetStructFieldByName(f, "B"); v.(*string) != &s {
		t.Fatalf("got %+v", v)
	}
	if v := GetStructFieldByName(f, "C"); v.(int) != 2 {
		t.Fatalf("got %+v", v)
	}
	if v := GetStructFieldByName(f, "D"); v != nil {
		t.Fatalf("got %+v", v)
	}
	if v := GetStructFieldByName(f, "E"); v != nil {
		t.Fatalf("got %+v", v)
	}

	fi := &foo{"1", &s, 2, nil}
	if v := GetStructFieldByName(fi, "A"); v.(string) != "1" {
		t.Fatalf("got %+v", v)
	}
	if v := GetStructFieldByName(fi, "B"); v.(*string) != &s {
		t.Fatalf("got %+v", v)
	}
	if v := GetStructFieldByName(fi, "C"); v.(int) != 2 {
		t.Fatalf("got %+v", v)
	}
	if v := GetStructFieldByName(fi, "D"); v != nil {
		t.Fatalf("got %+v", v)
	}
	if v := GetStructFieldByName(fi, "E"); v != nil {
		t.Fatalf("got %+v", v)
	}
}

// TestNilInterface verifies that NilInterface reports true for an untyped nil and for an interface holding a typed
// nil pointer (which is not == nil), and false for a struct value and an int.
func TestNilInterface(t *testing.T) {
	type foo struct{}
	var f *foo
	var v any
	var tf foo

	v = f
	require.NotEqual(t, v, nil)
	require.True(t, NilInterface(v))
	require.False(t, NilInterface(tf))
	require.False(t, NilInterface(123))
	require.True(t, NilInterface(nil))
}

// TestDeepClone verifies that DeepClone produces independent copies of a nested slice, a struct holding a pointer
// to a struct with a slice, and a pointer to such a struct, so later mutations of the source do not affect them.
func TestDeepClone(t *testing.T) {
	t.Run("slice", func(t *testing.T) {
		inner := []int{4, 5, 6}
		src := [][]int{inner}
		dst := DeepClone(src)

		inner[1] = 100
		require.NotEqual(t, src[0][1], dst[0][1])
	})

	type bar struct {
		A []string
	}

	type foo struct {
		A int
		B *bar
	}

	t.Run("struct", func(t *testing.T) {
		src := foo{
			A: 1,
			B: &bar{
				A: []string{"1", "2", "3"},
			},
		}
		dst := DeepClone(src)

		src.B.A[1] = "100"
		require.NotEqual(t, src.B.A[1], dst.B.A[1])
	})

	t.Run("*struct", func(t *testing.T) {
		src := &foo{
			A: 1,
			B: &bar{
				A: []string{"1", "2", "3"},
			},
		}
		dst := DeepClone(src)

		src.B.A[1] = "100"
		require.NotEqual(t, src.B.A[1], dst.B.A[1])
	})
}

// TestStructFieldRequired verifies that NotEmpty accepts a non-empty string, a pointer to it, and a non-zero int,
// and returns errors mentioning "is empty pointer", "is point to empty elem", or "is empty elem" for a nil
// pointer, a pointer to an empty string, and an empty string or zero float64 respectively.
func TestStructFieldRequired(t *testing.T) {
	v := struct {
		A  string
		AP *string
		B  int
		BB float64
	}{
		A: "123",
		B: 123,
	}

	require.NoError(t, NotEmpty(v.A, "A"))
	require.NoError(t, NotEmpty(&v.A, "*A"))
	require.ErrorContains(t, NotEmpty(v.AP, "AP"), "is empty pointer")

	emptyString := ""
	v.AP = &emptyString
	require.ErrorContains(t, NotEmpty(v.AP, "AP"), "is point to empty elem")
	require.ErrorContains(t, NotEmpty(*v.AP, "*AP"), "is empty elem")

	require.NoError(t, NotEmpty(v.B, "B"))
	require.ErrorContains(t, NotEmpty(v.BB, "BB"), "is empty elem")
}

// TestOptionalVal verifies that OptionalVal keeps a non-empty string and a non-zero int, and substitutes the
// optional value for a nil pointer field and a zero float64 field.
func TestOptionalVal(t *testing.T) {
	v := struct {
		A  string
		AP *string
		B  int
		BB float64
	}{
		A: "123",
		B: 123,
	}

	optStr := "laisky"
	optInt := 123
	optFloat64 := float64(123)

	v.A = OptionalVal(&v.A, optStr)
	require.Equal(t, v.A, "123")

	v.AP = OptionalVal(&v.AP, &optStr)
	require.Equal(t, v.AP, &optStr)

	v.B = OptionalVal(&v.B, optInt)
	require.Equal(t, v.B, optInt)

	v.BB = OptionalVal(&v.BB, optFloat64)
	require.Equal(t, v.BB, optFloat64)
}
