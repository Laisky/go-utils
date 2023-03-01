// Package gorm some useful tools for gorm
package gorm

import (
	"bytes"
	"compress/gzip"
	"database/sql/driver"
	"io"

	"github.com/Laisky/errors"

	gutils "github.com/Laisky/go-utils/v4"
)

// GzText store string with gzip into blob
type GzText string

// Value val -> db
func (j GzText) Value() (driver.Value, error) {
	out := new(bytes.Buffer)
	w := gzip.NewWriter(out)
	if _, err := w.Write([]byte(j)); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// Scan db -> val
func (j *GzText) Scan(value any) error {
	var val []byte
	switch value := value.(type) {
	case []byte:
		val = value
	case string:
		val = []byte(value)
	}

	if len(val) == 0 {
		*j = ""
		return nil
	}

	r, err := gzip.NewReader(bytes.NewReader(val))
	if err != nil {
		return err
	}
	defer gutils.SilentClose(r)
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	*j = GzText(string(b))
	return nil
}

// JSON store json into blob
type JSON []byte

// Value val -> db
func (j JSON) Value() (driver.Value, error) {
	if j.IsNull() {
		return nil, nil
	}
	return string(j), nil
}

// Scan db -> val
func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	s, ok := value.([]byte)
	if !ok {
		return errors.New("invalid scan source")
	}
	*j = append((*j)[0:0], s...)
	return nil
}

// Marshal return the encoding by json
func (j JSON) Marshal() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

// Unmarshal parses the JSON-encoded data and stores the result
// in the value pointed to by v. If v is nil or not a pointer,
// Unmarshal returns an InvalidUnmarshalError.
func (j *JSON) Unmarshal(data []byte) error {
	if j == nil {
		return errors.New("null point exception")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

// IsNull check is value is null
func (j JSON) IsNull() bool {
	return len(j) == 0 || string(j) == "null"
}

// Equals check whether equal to j1
func (j JSON) Equals(j1 JSON) bool {
	return bytes.Equal([]byte(j), []byte(j1))
}
