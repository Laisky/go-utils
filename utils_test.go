package utils

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
	_ "go.uber.org/automaxprocs"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-utils/v3/log"
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

func TestValidateFileHash(t *testing.T) {
	fp, err := os.CreateTemp("", "go-utils-*")
	require.NoError(t, err)
	defer os.Remove(fp.Name())
	defer fp.Close()

	content := []byte("jijf32ijr923e890dsfuodsafjlj;f9o2ur9re")
	_, err = fp.Write(content)
	require.NoError(t, err)

	err = ValidateFileHash(fp.Name(), "sha256:123")
	require.Error(t, err)

	err = ValidateFileHash(fp.Name(), "md5:123")
	require.Error(t, err)

	err = ValidateFileHash(fp.Name(), "sha254:123")
	require.Error(t, err)

	err = ValidateFileHash(fp.Name(), "")
	require.Error(t, err)

	err = ValidateFileHash(
		fp.Name(),
		"sha256:aea7e26c0e0b12ad210a8a0e45c379d0325b567afdd4b357158059b0ef03ae67",
	)
	require.NoError(t, err)

	err = ValidateFileHash(
		fp.Name(),
		"md5:794e37eea6b3df6e6eba69eb02f9b8c7",
	)
	require.NoError(t, err)
}

func TestJSON(t *testing.T) {
	jb, err := JSON.Marshal("123")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	var v string
	if err = JSON.Unmarshal(jb, &v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v != "123" {
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
	if name := GetFuncName(testFoo); name != "github.com/Laisky/go-utils/v3.testFoo" {
		t.Fatalf("want `testFoo`, got `%v`", name)
	}
}

func ExampleGetFuncName() {
	GetFuncName(testFoo) // "github.com/Laisky/go-utils.testFoo"
}

func TestFallBack(t *testing.T) {
	fail := func() any {
		panic("got error")
	}
	expect := 10
	got := FallBack(fail, 10)
	if expect != got.(int) {
		t.Errorf("expect %v got %v", expect, got)
	}
}

func ExampleFallBack() {
	targetFunc := func() any {
		panic("someting wrong")
	}

	FallBack(targetFunc, 10) // got 10
}

func TestRegexNamedSubMatch(t *testing.T) {
	reg := regexp.MustCompile(`^(?P<time>.{23}) {0,}\| {0,}(?P<app>[^ ]+) {0,}\| {0,}(?P<level>[^ ]+) {0,}\| {0,}(?P<thread>[^ ]+) {0,}\| {0,}(?P<class>[^ ]+) {0,}\| {0,}(?P<line>\d+) {0,}([\|:] {0,}(?P<args>\{.*\})){0,1}([\|:] {0,}(?P<message>.*)){0,1}`)
	str := "2018-04-02 02:02:10.928 | sh-datamining | INFO | http-nio-8080-exec-80 | com.pateo.qingcloud.gateway.core.zuul.filters.post.LogFilter | 74 | xxx"
	submatchMap := map[string]string{}
	if err := RegexNamedSubMatch(reg, str, submatchMap); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	for k, v := range submatchMap {
		fmt.Println(">>", k, ":", v)
	}

	if v1, ok := submatchMap["level"]; !ok {
		t.Fatalf("`level` should exists")
	} else if v1 != "INFO" {
		t.Fatalf("`level` shoule be `INFO`, but got: %v", v1)
	}
	if v2, ok := submatchMap["line"]; !ok {
		t.Fatalf("`line` should exists")
	} else if v2 != "74" {
		t.Fatalf("`line` shoule be `74`, but got: %v", v2)
	}
}

func ExampleRegexNamedSubMatch() {
	reg := regexp.MustCompile(`(?P<key>\d+.*)`)
	str := "12345abcde"
	groups := map[string]string{}
	if err := RegexNamedSubMatch(reg, str, groups); err != nil {
		log.Shared.Error("try to group match got error", zap.Error(err))
	}

	fmt.Println(groups)
	// Output: map[key:12345abcde]

}

func TestFlattenMap(t *testing.T) {
	data := map[string]any{}
	j := []byte(`{"a": "1", "b": {"c": 2, "d": {"e": 3}}, "f": 4, "g": {}}`)
	if err := JSON.Unmarshal(j, &data); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	FlattenMap(data, ".")
	if data["a"].(string) != "1" {
		t.Fatalf("expect %v, got %v", "1", data["a"])
	}
	if int(data["b.c"].(float64)) != 2 {
		t.Fatalf("expect %v, got %v", 2, data["b.c"])
	}
	if int(data["b.d.e"].(float64)) != 3 {
		t.Fatalf("expect %v, got %v", 3, data["b.d.e"])
	}
	if int(data["f"].(float64)) != 4 {
		t.Fatalf("expect %v, got %v", 4, data["f"])
	}
	if _, ok := data["g"]; ok {
		t.Fatalf("g should not exists")
	}
}

func ExampleFlattenMap() {
	data := map[string]any{
		"a": "1",
		"b": map[string]any{
			"c": 2,
			"d": map[string]any{
				"e": 3,
			},
		},
	}
	FlattenMap(data, "__")
	fmt.Println(data)
	// Output: map[a:1 b__c:2 b__d__e:3]
}

func TestTriggerGC(t *testing.T) {
	TriggerGC()
	ForceGC()
}

func TestTemplateWithMap(t *testing.T) {
	tpl := `123${k1} + ${k2}:${k-3} 22`
	data := map[string]any{
		"k1":  41,
		"k2":  "abc",
		"k-3": 213.11,
	}
	want := `12341 + abc:213.11 22`
	got := TemplateWithMap(tpl, data)
	if got != want {
		t.Fatalf("want `%v`, got `%v`", want, got)
	}
}

func TestURLMasking(t *testing.T) {
	type testcase struct {
		input  string
		output string
	}

	var (
		ret  string
		mask = "*****"
	)
	for _, tc := range []*testcase{
		{
			"http://12ijij:3j23irj@jfjlwef.ffe.com",
			"http://12ijij:" + mask + "@jfjlwef.ffe.com",
		},
		{
			"https://12ijij:3j23irj@123.1221.14/13",
			"https://12ijij:" + mask + "@123.1221.14/13",
		},
	} {
		ret = URLMasking(tc.input, mask)
		if ret != tc.output {
			t.Fatalf("expect %v, got %v", tc.output, ret)
		}
	}
}

func ExampleURLMasking() {
	originURL := "http://12ijij:3j23irj@jfjlwef.ffe.com"
	newURL := URLMasking(originURL, "*****")
	fmt.Println(newURL)
	// Output: http://12ijij:*****@jfjlwef.ffe.com
}

func TestAutoGC(t *testing.T) {
	var err error
	if err = log.Shared.ChangeLevel("debug"); err != nil {
		t.Fatalf("%+v", err)
	}

	var fp *os.File
	if fp, err = os.CreateTemp("", "test-gc*"); err != nil {
		t.Fatalf("%+v", err)
	}
	defer fp.Close()

	if _, err = fp.WriteString("123456789"); err != nil {
		t.Fatalf("%+v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = AutoGC(ctx,
		WithGCMemRatio(85),
		WithGCMemLimitFilePath(fp.Name()),
	)
	require.NoError(t, err)
	<-ctx.Done()
	// t.Error()

	// case: test err arguments
	{
		err = AutoGC(ctx, WithGCMemRatio(-1))
		require.Error(t, err)

		err = AutoGC(ctx, WithGCMemRatio(0))
		require.Error(t, err)

		err = AutoGC(ctx, WithGCMemRatio(101))
		require.Error(t, err)

		err = AutoGC(ctx, WithGCMemLimitFilePath(RandomStringWithLength(100)))
		require.Error(t, err)
	}
}

func ExampleAutoGC() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := AutoGC(
		ctx,
		WithGCMemRatio(85), // default
		WithGCMemLimitFilePath("/sys/fs/cgroup/memory/memory.limit_in_bytes"), // default
	); err != nil {
		log.Shared.Error("enable autogc", zap.Error(err))
	}
}

func TestForceGCBlocking(t *testing.T) {
	ForceGCBlocking()
}

func ExampleForceGCBlocking() {
	ForceGCBlocking()
}

func ExampleForceGCUnBlocking() {
	ForceGCUnBlocking()
}

func TestForceGCUnBlocking(t *testing.T) {
	ForceGCUnBlocking()

	var pool errgroup.Group
	for i := 0; i < 1000; i++ {
		pool.Go(func() error {
			ForceGCUnBlocking()
			return nil
		})
	}

	require.NoError(t, pool.Wait())
}

func TestReflectSet(t *testing.T) {
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

func TestUniqueStrings(t *testing.T) {
	orig := []string{}
	for i := 0; i < 100000; i++ {
		orig = append(orig, RandomStringWithLength(2))
	}
	t.Logf("generate length : %d", len(orig))
	orig = UniqueStrings(orig)
	t.Logf("after unique length : %d", len(orig))
	m := map[string]bool{}
	var ok bool
	for _, v := range orig {
		if _, ok = m[v]; ok {
			t.Fatalf("duplicate: %v", v)
		} else {
			m[v] = ok
		}
	}
}

func TestRunCMD(t *testing.T) {
	ctx := context.Background()
	type args struct {
		app  string
		args []string
	}
	tests := []struct {
		name       string
		args       args
		wantStdout []byte
		wantErr    bool
	}{
		{"sleep", args{"sleep", []string{"0.1"}}, []byte{}, false},
		{"sleep-err", args{"sleep", nil}, []byte("sleep: missing operand"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStdout, err := RunCMD(ctx, tt.args.app, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunCMD() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !bytes.Contains(gotStdout, tt.wantStdout) {
				t.Errorf("RunCMD() = %s, want %s", gotStdout, tt.wantStdout)
			}
		})
	}
}

// linux pipe has 16MB default buffer
func TestRunCMDForHugeFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "run_cmd-*")
	require.NoError(t, err)
	defer os.Remove(dir)

	fpath := filepath.Join(dir, "test.txt")
	fp, err := os.OpenFile(fpath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0664)
	require.NoError(t, err)

	for i := 0; i < 1024*18; i++ {
		_, err = fp.Write([]byte(RandomStringWithLength(1024)))
		require.NoError(t, err)
	}
	err = fp.Close()
	require.NoError(t, err)

	ctx := context.Background()
	out, err := RunCMD(ctx, "cat", fpath)
	require.NoError(t, err)
	require.Equal(t, len(out), 18*1024*1024)
}

func TestRemoveEmpty(t *testing.T) {
	type args struct {
		vs []string
	}
	tests := []struct {
		name  string
		args  args
		wantR []string
	}{
		{"0", args{[]string{"1"}}, []string{"1"}},
		{"1", args{[]string{"1", ""}}, []string{"1"}},
		{"2", args{[]string{"1", "", "  "}}, []string{"1"}},
		{"3", args{[]string{"1", "", "  ", "2", ""}}, []string{"1", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotR := RemoveEmpty(tt.args.vs); !reflect.DeepEqual(gotR, tt.wantR) {
				t.Errorf("RemoveEmpty() = %v, want %v", gotR, tt.wantR)
			}
		})
	}
}

func TestTrimEleSpaceAndRemoveEmpty(t *testing.T) {
	type args struct {
		vs []string
	}
	tests := []struct {
		name  string
		args  args
		wantR []string
	}{
		{"0", args{[]string{"1"}}, []string{"1"}},
		{"1", args{[]string{"1", ""}}, []string{"1"}},
		{"2", args{[]string{"1", "", "  "}}, []string{"1"}},
		{"3", args{[]string{"1", "", "  ", "2", ""}}, []string{"1", "2"}},
		{"4", args{[]string{"1", "", "  ", "2   ", ""}}, []string{"1", "2"}},
		{"5", args{[]string{"1", "", "  ", "   2   ", ""}}, []string{"1", "2"}},
		{"6", args{[]string{"1", "", "  ", "   2", ""}}, []string{"1", "2"}},
		{"7", args{[]string{"   1", "", "  ", "   2", ""}}, []string{"1", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotR := TrimEleSpaceAndRemoveEmpty(tt.args.vs); !reflect.DeepEqual(gotR, tt.wantR) {
				t.Errorf("TrimEleSpaceAndRemoveEmpty() = %v, want %v", gotR, tt.wantR)
			}
		})
	}
}

func TestInArray(t *testing.T) {
	type args struct {
		collection any
		ele        any
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"0", args{[]string{"1", "2"}, "2"}, true},
		{"1", args{[]string{"1", "2"}, "1"}, true},
		{"2", args{[]string{"1", "2"}, "3"}, false},
		{"3", args{[]int{1, 2}, 3}, false},
		{"4", args{[]int{1, 2}, 2}, true},
		{"5", args{[...]int{1, 2}, 3}, false},
		{"6", args{[...]int{1, 2}, 2}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InArray(tt.args.collection, tt.args.ele); got != tt.want {
				t.Errorf("InArray() = %v, want %v", got, tt.want)
			}
		})
	}

	isPanic := IsPanic(func() {
		InArray([]uint{1, 2}, 1)
	})
	require.True(t, isPanic)

	isPanic = IsPanic(func() {
		InArray([]int{1, 2}, "1")
	})
	require.True(t, isPanic)
}

func ExampleExpCache() {
	cc := NewExpCache(context.Background(), 100*time.Millisecond)
	cc.Store("key", "val")
	cc.Load("key") // return "val"

	// data expired
	time.Sleep(200 * time.Millisecond)
	data, ok := cc.Load("key")
	fmt.Println(data)
	fmt.Println(ok)

	// Output: <nil>
	// false
}

func TestExpCache_Store(t *testing.T) {
	cm := NewExpCache(context.Background(), 100*time.Millisecond)
	key := "key"
	val := "val"
	cm.Store(key, val)
	for i := 0; i < 5; i++ {
		if vali, ok := cm.Load(key); !ok {
			t.Fatal("should ok")
		} else if vali.(string) != val {
			t.Fatalf("got: %+v", vali)
		}
	}

	time.Sleep(200 * time.Millisecond)
	if _, ok := cm.Load(key); ok {
		t.Fatal("should not ok")
	}
}

// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils
// BenchmarkExpMap-8   	  141680	     10275 ns/op	      54 B/op	       6 allocs/op
// PASS
// ok  	github.com/Laisky/go-utils	1.573s
func BenchmarkExpMap(b *testing.B) {
	cm, err := NewLRUExpiredMap(context.Background(),
		10*time.Millisecond,
		func() any { return 1 },
	)
	if err != nil {
		b.Fatalf("%+v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cm.Get(RandomStringWithLength(1))
		}
	})
}

func TestGetStructFieldByName(t *testing.T) {
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

func Benchmark_NewSimpleExpCache(b *testing.B) {
	c := NewSingleItemExpCache(time.Millisecond)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if rand.Intn(10) < 5 {
				c.Set(RandomStringWithLength(rand.Intn(100)))
			} else {
				c.Get()
			}
		}
	})
}

func TestNewSimpleExpCache(t *testing.T) {
	// another test may change the clock's interval.
	// default interval is 10ms, so we need to set interval bigger than 10ms.
	//
	// time.clock's test set interval to 100ms.
	fmt.Println("interval", Clock.Interval())
	Clock.SetInterval(1 * time.Microsecond)
	c := NewSingleItemExpCache(200 * time.Millisecond)

	_, ok := c.Get()
	require.False(t, ok)
	_, ok = c.GetString()
	require.False(t, ok)
	_, ok = c.GetUintSlice()
	require.False(t, ok)

	data := "yo"
	c.Set(data)
	itf, ok := c.Get()
	require.True(t, ok)
	require.Equal(t, data, itf.(string))

	ret, ok := c.GetString()
	require.True(t, ok)
	require.Equal(t, data, ret)

	time.Sleep(200 * time.Millisecond)
	itf, ok = c.Get()
	require.False(t, ok)
	require.Equal(t, data, itf.(string))
}

func TestNewExpiredMap(t *testing.T) {
	ctx := context.Background()
	m, err := NewLRUExpiredMap(ctx, time.Millisecond, func() any { return 666 })
	require.NoError(t, err)

	const key = "key"
	v := m.Get(key)
	require.Equal(t, 666, v)
	v = m.Get(key)
	require.Equal(t, 666, v)
}

/*
cpu: Intel(R) Core(TM) i7-4790 CPU @ 3.60GHz
Benchmark_Str2Bytes/normal_str2bytes-8         	  868298	      1156 ns/op	    1024 B/op	       1 allocs/op
Benchmark_Str2Bytes/normal_bytes2str-8         	 1000000	      1216 ns/op	    1024 B/op	       1 allocs/op
Benchmark_Str2Bytes/unsafe_str2bytes-8         	11335250	        92.66 ns/op	       0 B/op	       0 allocs/op
Benchmark_Str2Bytes/unsafe_bytes2str-8         	11320952	       106.2 ns/op	       0 B/op	       0 allocs/op
PASS
*/
func Benchmark_Str2Bytes(b *testing.B) {
	rawStr := RandomStringWithLength(1024)
	rawBytes := []byte(rawStr)
	b.Run("normal_str2bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = []byte(rawStr)
		}
	})
	b.Run("normal_bytes2str", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = string(rawBytes)
		}
	})
	b.Run("unsafe_str2bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Str2Bytes(rawStr)
		}
	})
	b.Run("unsafe_bytes2str", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Bytes2Str(rawBytes)
		}
	})
}

// func Test_ConvertMap(t *testing.T) {
// 	{
// 		input := map[any]string{"123": "23"}
// 		got := ConvertMap(input)
// 		t.Log(got)
// 		require.True(t, reflect.DeepEqual(map[string]any{"123": "23"}, got))
// 	}

// 	{
// 		input := map[any]int{"123": 23}
// 		got := ConvertMap(input)
// 		t.Log(got)
// 		require.True(t, reflect.DeepEqual(map[string]any{"123": 23}, got))
// 	}

// 	{
// 		input := map[any]uint{"123": 23}
// 		got := ConvertMap(input)
// 		t.Log(got)
// 		require.True(t, reflect.DeepEqual(map[string]any{"123": uint(23)}, got))
// 	}

// 	{
// 		input := map[string]int{"123": 23}
// 		got := ConvertMap(input)
// 		t.Log(got)
// 		require.True(t, reflect.DeepEqual(map[string]any{"123": 23}, got))
// 	}

// }

func TestConvert2Map(t *testing.T) {
	type args struct {
		inputMap any
	}
	tests := []struct {
		name string
		args args
		want map[string]any
	}{
		{"0", args{map[any]string{"123": "23"}}, map[string]any{"123": "23"}},
		{"1", args{map[any]int{"123": 23}}, map[string]any{"123": 23}},
		{"2", args{map[any]uint{"123": 23}}, map[string]any{"123": uint(23)}},
		{"3", args{map[string]uint{"123": 23}}, map[string]any{"123": uint(23)}},
		{"4", args{map[int]uint{123: 23}}, map[string]any{"123": uint(23)}},
		{"5", args{map[float32]string{float32(123): "23"}}, map[string]any{"123": "23"}},
		{"6", args{map[float32]int{float32(123): 23}}, map[string]any{"123": 23}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertMap2StringKey(tt.args.inputMap); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStopSignal(t *testing.T) {
	stopCh := StopSignal(WithStopSignalCloseSignals(os.Interrupt, syscall.SIGTERM))
	select {
	case <-stopCh:
		t.Fatal("should not be closed")
	default:
	}

	err := syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	require.NoError(t, err)

	_, ok := <-stopCh
	require.False(t, ok)

	// case: panic
	{
		ok := IsPanic(func() {
			_ = StopSignal(WithStopSignalCloseSignals())
		})
		require.True(t, ok)
	}
}

func TestBytes2Str(t *testing.T) {
	rawStr := RandomStringWithLength(1024)
	rawBytes := []byte(rawStr)
	str := Bytes2Str(rawBytes)
	require.Equal(t, rawStr, str)

	// case: bytes should changed by string
	{
		rawBytes[0] = '@'
		rawBytes[1] = 'a'
		rawBytes[2] = 'b'
		rawBytes[3] = 'c'
		require.Equal(t, string(rawBytes), str)
	}

	// case: Str2Bytes should return the same bytes struct
	{
		newBytes := Str2Bytes(str)
		require.Equal(t, fmt.Sprintf("%x", newBytes), fmt.Sprintf("%x", rawBytes))
	}
}

func Benchmark_slice(b *testing.B) {
	type foo struct {
		val string
	}
	payload := RandomStringWithLength(128)

	b.Run("[]struct append", func(b *testing.B) {
		var data []foo
		for i := 0; i < b.N; i++ {
			data = append(data, foo{val: payload})
		}

		b.Log(len(data))
	})

	b.Run("[]*struct append", func(b *testing.B) {
		var data []*foo
		for i := 0; i < b.N; i++ {
			data = append(data, &foo{val: payload})
		}

		b.Log(len(data))
	})

	b.Run("[]struct with prealloc", func(b *testing.B) {
		data := make([]foo, 100)
		for i := 0; i < b.N; i++ {
			data[i%100] = foo{val: payload}
		}
	})

	b.Run("[]*struct with prealloc", func(b *testing.B) {
		data := make([]*foo, 100)
		for i := 0; i < b.N; i++ {
			data[i%100] = &foo{val: payload}
		}
	})
}

func TestJSONMd5(t *testing.T) {
	type args struct {
		data any
	}
	type foo struct {
		Name string `json:"name"`
	}
	var nilArgs *foo
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"0", args{nil}, "", true},
		{"1", args{nilArgs}, "", true},
		{"2", args{foo{}}, "555dfa90763bd852d5dd9144887eed97", false},
		{"3", args{new(foo)}, "555dfa90763bd852d5dd9144887eed97", false},
		{"4", args{foo{""}}, "555dfa90763bd852d5dd9144887eed97", false},
		{"5", args{foo{Name: "a"}}, "88148e411b9b424a2e0ddf108cb02baa", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MD5JSON(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("MD5JSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MD5JSON() = %v, want %v", got, tt.want)
			}
		})
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

func TestPanicIfErr(t *testing.T) {
	PanicIfErr(nil)

	err := errors.New("yo")
	defer func() {
		perr := recover()
		require.Equal(t, err, perr)
	}()
	PanicIfErr(err)
}

func TestDedent(t *testing.T) {
	// t.Run("normal", func(t *testing.T) {
	// 	v := `
	// 	123
	// 	234
	// 	 345
	// 		222
	// 	`

	// 	dedent := Dedent(v, WithReplaceTabBySpaces(4))
	// 	require.Equal(t, "123\n234\n 345\n    222", dedent)
	// })

	t.Run("normal with blank lines", func(t *testing.T) {
		v := `
		123


		234

		 345
			222
		`

		dedent := Dedent(v, WithReplaceTabBySpaces(4))
		require.Equal(t, "123\n\n\n234\n\n 345\n    222", dedent)
	})

	t.Run("3 blanks", func(t *testing.T) {
		v := `
		123
		234
		 345	2
			222
		`

		dedent := Dedent(v, WithReplaceTabBySpaces(3))
		require.Equal(t, "123\n234\n 345\t2\n   222", dedent)
	})

	t.Run("shrink", func(t *testing.T) {
		v := `
		123
	   234
		`

		dedent := Dedent(v)
		require.Equal(t, " 123\n234", dedent)
	})

	t.Run("shrink with blank line", func(t *testing.T) {
		v := `
		123

	   234
		`

		dedent := Dedent(v)
		require.Equal(t, " 123\n\n234", dedent)
	})

}

func TestDeepClone(t *testing.T) {
	t.Run("slice", func(t *testing.T) {
		inner := []int{4, 5, 6}
		src := [][]int{inner}
		dst := DeepClone(src)

		inner[1] = 100
		require.NotEqual(t, src[0][1], dst.([][]int)[0][1])
	})
}

type testCloseQuitlyStruct struct{}

func (f *testCloseQuitlyStruct) Close() error {
	return nil
}

func TestSilentClose(t *testing.T) {

	f := new(testCloseQuitlyStruct)
	SilentClose(f)
}

func TestContains(t *testing.T) {
	require.True(t, Contains([]string{"1", "2", "3"}, "2"))
	require.False(t, Contains([]string{"1", "2", "3"}, "4"))
	require.True(t, Contains([]int{1, 2, 3}, 2))
	require.False(t, Contains([]int{1, 2, 3}, 4))
}

func TestCtxKey(t *testing.T) {
	// Warning: should not use empty type as context key
	t.Run("empty type as key", func(t *testing.T) {
		type ctxKey struct{}

		var (
			keya, keyb ctxKey
		)

		ctx := context.Background()
		ctx = context.WithValue(ctx, keya, 123)

		require.Equal(t, 123, ctx.Value(keyb)) // <- this is incorrect
		require.Equal(t, 123, ctx.Value(keya))
	})

	t.Run("string as key", func(t *testing.T) {
		type ctxKey string

		var (
			keya ctxKey = "a"
			keyb ctxKey = "b"
		)

		ctx := context.Background()
		ctx = context.WithValue(ctx, keya, 123)

		require.Nil(t, ctx.Value(keyb))
		require.Equal(t, 123, ctx.Value(keya))
	})

	t.Run("different type string as key", func(t *testing.T) {
		type ctxKeyA string
		type ctxKeyB string

		var (
			keya ctxKeyA = "a"
			keyb ctxKeyB = "a"
		)

		ctx := context.Background()
		ctx = context.WithValue(ctx, keya, 123)

		require.Nil(t, ctx.Value(keyb))
		require.Equal(t, 123, ctx.Value(keya))
	})
}
