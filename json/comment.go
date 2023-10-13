package json

import (
	json2 "github.com/go-json-experiment/json"
	"github.com/pkg/errors"
	"github.com/tailscale/hujson"
)

var (
	// Unmarshal unmarshal json, do not support comment
	Unmarshal = json2.Unmarshal
)

// UnmarshalFromString unmarshal json from string, do not support comment
func UnmarshalFromString(str string, v interface{}) (err error) {
	return Unmarshal([]byte(str), v)
}

// UnmarshalComment unmarshal json, support comment
func UnmarshalComment(data []byte, v interface{}) (err error) {
	if len(data) == 0 {
		return nil
	}

	data, err = standardizeJSON(data)
	if err != nil {
		return errors.Wrap(err, "standardize json")
	}

	return json2.Unmarshal(data, v)
}

// UnmarshalCommentFromString unmarshal json from string, support comment
func UnmarshalCommentFromString(str string, v interface{}) (err error) {
	if str == "" {
		return nil
	}

	data, err := standardizeJSON([]byte(str))
	if err != nil {
		return errors.Wrap(err, "standardize json")
	}

	return Unmarshal(data, v)
}

func standardizeJSON(b []byte) ([]byte, error) {
	ast, err := hujson.Parse(b)
	if err != nil {
		return b, err
	}
	ast.Standardize()
	return ast.Pack(), nil
}
