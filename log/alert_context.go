package log

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
)

// alertContextSnapshot replays encoded bound fields without their original mutable values.
type alertContextSnapshot string

// MarshalLogObject preserves duplicate root keys and exact JSON projections with RawMessage.
func (snapshot alertContextSnapshot) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	decoder := json.NewDecoder(strings.NewReader(string(snapshot)))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return errors.WithStack(ErrAlertEncoding)
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return errors.WithStack(ErrAlertEncoding)
		}
		name, ok := key.(string)
		if !ok {
			return errors.WithStack(ErrAlertEncoding)
		}
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return errors.WithStack(ErrAlertEncoding)
		}
		if err = enc.AddReflected(name, raw); err != nil {
			return errors.WithStack(ErrAlertEncoding)
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return errors.WithStack(ErrAlertEncoding)
	}
	if _, err = decoder.Token(); err != io.EOF {
		return errors.WithStack(ErrAlertEncoding)
	}
	return nil
}
