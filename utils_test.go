package utils_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/Laisky/go-utils"
)

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

func TestFlattenMap(t *testing.T) {
	data := map[string]interface{}{}
	j := []byte(`{"a": "1", "b": {"c": 2, "d": {"e": 3}}, "f": 4}`)
	if err := json.Unmarshal(j, &data); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	utils.FlattenMap(data)
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
}
