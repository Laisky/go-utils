package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/Laisky/zap"
)

func testFoo() {}

func TestGetFuncName(t *testing.T) {
	if name := GetFuncName(testFoo); name != "github.com/Laisky/go-utils.testFoo" {
		t.Fatalf("want `testFoo`, got `%v`", name)
	}
}

func ExampleGetFuncName() {
	GetFuncName(testFoo) // "github.com/Laisky/go-utils_test.testFoo"
}

func TestFallBack(t *testing.T) {
	fail := func() interface{} {
		panic("got error")
	}
	expect := 10
	got := FallBack(fail, 10)
	if expect != got.(int) {
		t.Errorf("expect %v got %v", expect, got)
	}
}

func ExampleFallBack() {
	targetFunc := func() interface{} {
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
		Logger.Error("try to group match got error", zap.Error(err))
	}

	fmt.Printf("got: %+v", groups) // map[string]string{"key": 12345}
}

func TestFlattenMap(t *testing.T) {
	data := map[string]interface{}{}
	j := []byte(`{"a": "1", "b": {"c": 2, "d": {"e": 3}}, "f": 4, "g": {}}`)
	if err := json.Unmarshal(j, &data); err != nil {
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
	data := map[string]interface{}{
		"a": "1",
		"b": map[string]interface{}{
			"c": 2,
			"d": map[string]interface{}{
				"e": 3,
			},
		},
	}
	FlattenMap(data, "__") // {"a": "1", "b__c": 2, "b__d__e": 3}
}

func TestTriggerGC(t *testing.T) {
	TriggerGC()
	ForceGC()
}

func TestTemplateWithMap(t *testing.T) {
	tpl := `123${k1} + ${k2}:${k-3} 22`
	data := map[string]interface{}{
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
	fmt.Println(newURL) // http://12ijij:*****@jfjlwef.ffe.com
}

func TestDirSize(t *testing.T) {
	// size, err := DirSize("/Users/laisky/Projects/go/src/pateo.com/go-fluentd")
	size, err := DirSize(".")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("size: %v", size)
	// t.Error()
}

func ExampleDirSize() {
	dirPath := "."
	size, err := DirSize(dirPath)
	if err != nil {
		Logger.Error("get dir size", zap.Error(err), zap.String("path", dirPath))
	}
	Logger.Info("got size", zap.Int64("size", size), zap.String("path", dirPath))
}

func TestAutoGC(t *testing.T) {
	var err error
	if err = Logger.ChangeLevel("debug"); err != nil {
		t.Fatalf("%+v", err)
	}

	var fp *os.File
	if fp, err = ioutil.TempFile("", "test-gc"); err != nil {
		t.Fatalf("%+v", err)
	}
	defer fp.Close()

	if _, err = fp.WriteString("123456789"); err != nil {
		t.Fatalf("%+v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = AutoGC(ctx, WithGCMemLimitFilePath(fp.Name())); err != nil {
		t.Fatalf("%+v", err)
	}
	<-ctx.Done()
	// t.Error()
}

func ExampleAutoGC() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := AutoGC(
		ctx,
		WithGCMemRatio(85), // default
		WithGCMemLimitFilePath("/sys/fs/cgroup/memory/memory.limit_in_bytes"), // default
	); err != nil {
		Logger.Error("enable autogc", zap.Error(err))
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
		Logger.Error("set struct val", zap.Error(err))
		return
	}

	fmt.Printf("%+v\n", ss)
	//
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
