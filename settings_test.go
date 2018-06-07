package utils_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Laisky/go-utils"
)

func TestViperSettings(t *testing.T) {
	configs := utils.Settings.Get("tasks.elasticsearch.configs").([]interface{})
	for _, c := range configs {
		config := c.(map[interface{}]interface{})
		var (
			index  string
			expire int
			term   = map[string]string{}
		)
		if val, ok := config["index"]; ok {
			index = val.(string)
		}
		if val, ok := config["expire"]; ok {
			expire = val.(int)
		}
		if val, ok := config["term"]; ok {
			if err := json.Unmarshal([]byte(val.(string)), &term); err != nil {
				panic(fmt.Sprintf("load delete settings error: %v", err))
			}
			t.Logf("%v, %v, %+v\n", index, expire, term)
		}
	}
	// t.Error("OK")
}

func TestStruct(t *testing.T) {
	type Demo struct {
		A *int
		B int
	}

	c := Demo{}
	if c.A == nil {
		t.Logf(">>> %v", c.A)
	}
	// t.Error("ok")
}

// func init() {
// 	utils.SetupSettings() // load config
// }
