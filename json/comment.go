package json

import (
	"encoding/json"

	"github.com/Laisky/go-utils/v4/common"

	// json2 "github.com/go-json-experiment/json"
	"github.com/Laisky/errors/v2"
	"github.com/tailscale/hujson"
)

var (
	// Unmarshal unmarshal json, do not support comment
	Unmarshal = json.Unmarshal
)

// UnmarshalFromString unmarshal json from string, do not support comment
func UnmarshalFromString(str string, v interface{}) (err error) {
	return Unmarshal(common.Str2Bytes(str), v)
}

// UnmarshalComment unmarshal json, support comment
//
// Notice: this func will change the content of raw, all comments will be removed
func UnmarshalComment(raw []byte, v interface{}) (err error) {
	if len(raw) == 0 {
		return nil
	}

	data, err := standardizeJSON(raw)
	if err != nil {
		return errors.Wrap(err, "standardize json")
	}

	return Unmarshal(data, v)
}

// UnmarshalCommentFromString unmarshal json from string, support comment
//
// Notice: this func will change the content of raw, all comments will be removed
func UnmarshalCommentFromString(str string, v interface{}) (err error) {
	return UnmarshalComment(common.Str2Bytes(str), v)
}

func standardizeJSON(b []byte) ([]byte, error) {
	ast, err := hujson.Parse(b)
	if err != nil {
		return b, errors.Wrap(err, "parse json by hujson")
	}

	ast.Standardize()
	return ast.Pack(), nil
}
