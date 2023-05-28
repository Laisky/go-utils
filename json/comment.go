package json

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/tailscale/hujson"
)

// Unmarshal unmarshal json, support comment
func Unmarshal(data []byte, v interface{}) (err error) {
	if len(data) == 0 {
		return nil
	}

	data, err = standardizeJSON(data)
	if err != nil {
		return errors.Wrap(err, "standardize json")
	}

	return json.Unmarshal(data, v)
}

// UnmarshalFromString unmarshal json from string, support comment
func UnmarshalFromString(str string, v interface{}) (err error) {
	if str == "" {
		return nil
	}

	data, err := standardizeJSON([]byte(str))
	if err != nil {
		return errors.Wrap(err, "standardize json")
	}

	return json.Unmarshal(data, v)
}

func standardizeJSON(b []byte) ([]byte, error) {
	ast, err := hujson.Parse(b)
	if err != nil {
		return b, err
	}
	ast.Standardize()
	return ast.Pack(), nil
}
