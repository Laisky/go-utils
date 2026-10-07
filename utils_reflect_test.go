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

func (t *testStCorrect1) PointerMethod() {

}

func (t *testStCorrect1) Method() {

}

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

func testFoo() {}

func TestGetFuncName(t *testing.T) {
	t.Parallel()

	if name := GetFuncName(testFoo); name != "github.com/Laisky/go-utils/v6.testFoo" {
		t.Fatalf("want `testFoo`, got `%v`", name)
	}
}

func ExampleGetFuncName() {
	GetFuncName(testFoo) // "github.com/Laisky/go-utils.testFoo"
}

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
