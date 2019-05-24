package utils_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func ExampleFallBack() {
	targetFunc := func() interface{} {
		panic("someting wrong")
	}

	utils.FallBack(targetFunc, 10) // got 10
}

func foo() {}

func ExampleGetFuncName() {
	utils.GetFuncName(foo) // "github.com/Laisky/go-utils_test.foo"
}

func TestGetFuncName(t *testing.T) {
	if name := utils.GetFuncName(foo); name != "github.com/Laisky/go-utils_test.foo" {
		t.Fatalf("want `foo`, got `%v`", name)
	}
}

func TestFallBack(t *testing.T) {
	fail := func() interface{} {
		panic("got error")
	}
	expect := 10
	got := utils.FallBack(fail, 10)
	if expect != got.(int) {
		t.Errorf("expect %v got %v", expect, got)
	}
}

func ExampleRegexNamedSubMatch() {
	reg := regexp.MustCompile(`(?P<key>\d+.*)`)
	str := "12345abcde"
	groups := map[string]string{}
	if err := utils.RegexNamedSubMatch(reg, str, groups); err != nil {
		utils.Logger.Error("try to group match got error", zap.Error(err))
	}

	fmt.Printf("got: %+v", groups) // map[string]string{"key": 12345}
}

func TestRegexNamedSubMatch(t *testing.T) {
	reg := regexp.MustCompile(`^(?P<time>.{23}) {0,}\| {0,}(?P<app>[^ ]+) {0,}\| {0,}(?P<level>[^ ]+) {0,}\| {0,}(?P<thread>[^ ]+) {0,}\| {0,}(?P<class>[^ ]+) {0,}\| {0,}(?P<line>\d+) {0,}([\|:] {0,}(?P<args>\{.*\})){0,1}([\|:] {0,}(?P<message>.*)){0,1}`)
	str := "2018-04-02 02:02:10.928 | sh-datamining | INFO | http-nio-8080-exec-80 | com.pateo.qingcloud.gateway.core.zuul.filters.post.LogFilter | 74 | xxx"
	submatchMap := map[string]string{}
	if err := utils.RegexNamedSubMatch(reg, str, submatchMap); err != nil {
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
	utils.FlattenMap(data, "__") // {"a": "1", "b__c": 2, "b__d__e": 3}
}

func TestFlattenMap(t *testing.T) {
	data := map[string]interface{}{}
	j := []byte(`{"a": "1", "b": {"c": 2, "d": {"e": 3}}, "f": 4, "g": {}}`)
	if err := json.Unmarshal(j, &data); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	utils.FlattenMap(data, ".")
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
