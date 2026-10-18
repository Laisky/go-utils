package log

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
)

// ErrAlertEncoding rejects malformed or unrepresentable fields before local admission.
// The error never includes application values, callback errors or panic contents.
var ErrAlertEncoding = errors.New("alert fields cannot be encoded faithfully")

// alertEncodingState tracks a lower bound on bytes and callback/encoding failures.
type alertEncodingState struct {
	remaining int
	err       error
}

// take stops encoding once its lower bound exceeds the complete message budget.
func (s *alertEncodingState) take(n int) bool {
	if s.err != nil {
		return false
	}
	if n > s.remaining {
		s.err = ErrAlertMessageTooLarge
		return false
	}
	s.remaining -= n
	return true
}

// text rejects invalid UTF-8 rather than accepting JSON's replacement characters.
func (s *alertEncodingState) text(v string) bool {
	if !utf8.ValidString(v) {
		s.err = ErrAlertEncoding
		return false
	}
	return s.take(len(v))
}

// failed records an encoder error without retaining its potentially sensitive text.
func (s *alertEncodingState) failed(err error) error {
	if err != nil && s.err == nil {
		if errors.Is(err, ErrAlertMessageTooLarge) {
			s.err = ErrAlertMessageTooLarge
		} else {
			s.err = ErrAlertEncoding
		}
	}
	return s.err
}

// newAlertEncoder constructs the standard Zap JSON field encoder with strict reflection.
func (a *Alert) newAlertEncoder() zapcore.Encoder {
	if a.encFactory != nil {
		return a.encFactory()
	}
	return zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		NewReflectedEncoder: func(w io.Writer) zapcore.ReflectedEncoder {
			return &alertReflectedEncoder{writer: w, limit: a.maxMessageBytes}
		},
	})
}

// addAlertFields preserves standard Zap representations while observing swallowed errors.
func addAlertFields(enc zapcore.Encoder, fields []zapcore.Field, limit int) (err error) {
	state := &alertEncodingState{remaining: limit}
	defer func() {
		if recover() != nil {
			err = errors.WithStack(ErrAlertEncoding)
		}
	}()
	checked := &alertObjectEncoder{ObjectEncoder: enc, state: state}
	for _, f := range fields {
		if state.err != nil {
			break
		}
		switch f.Type {
		case zapcore.InlineMarshalerType:
			f.Interface = alertObjectValue{value: f.Interface.(zapcore.ObjectMarshaler), state: state}
		case zapcore.StringerType:
			f.Interface = alertStringer{value: f.Interface.(fmt.Stringer), state: state}
		case zapcore.ErrorType:
			f.Interface = wrapAlertError(f.Interface.(error), state)
		case zapcore.ArrayMarshalerType, zapcore.ObjectMarshalerType,
			zapcore.BinaryType, zapcore.BoolType, zapcore.ByteStringType,
			zapcore.Complex128Type, zapcore.Complex64Type, zapcore.DurationType,
			zapcore.Float64Type, zapcore.Float32Type, zapcore.Int64Type,
			zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type,
			zapcore.StringType, zapcore.TimeType, zapcore.TimeFullType,
			zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type,
			zapcore.Uint8Type, zapcore.UintptrType, zapcore.ReflectType,
			zapcore.NamespaceType, zapcore.SkipType:
			// Checked encoder methods preserve the native representation.
		case zapcore.UnknownType:
			return errors.WithStack(ErrAlertEncoding)
		default:
			return errors.WithStack(ErrAlertEncoding)
		}
		f.AddTo(checked)
	}
	if state.err != nil {
		return errors.WithStack(state.err)
	}
	// Includes bound context and JSON expansion; no partially encoded entry is admitted.
	buffer, encodeErr := enc.EncodeEntry(zapcore.Entry{}, nil)
	if buffer != nil {
		defer buffer.Free()
	}
	if encodeErr != nil || buffer == nil {
		return errors.WithStack(ErrAlertEncoding)
	}
	if buffer.Len() > limit {
		return errors.WithStack(ErrAlertMessageTooLarge)
	}
	return nil
}

// alertObjectEncoder validates all object keys and primitive text without filtering fields.
type alertObjectEncoder struct {
	zapcore.ObjectEncoder
	state *alertEncodingState
}

// AddObject observes nested marshaler failures that Zap otherwise encodes as companion fields.
func (e *alertObjectEncoder) AddObject(key string, v zapcore.ObjectMarshaler) error {
	if !e.state.text(key) || !e.state.take(1) {
		return e.state.err
	}
	return e.state.failed(e.ObjectEncoder.AddObject(key, alertObjectValue{v, e.state}))
}

// AddArray observes nested array failures without accepting partial data.
func (e *alertObjectEncoder) AddArray(key string, v zapcore.ArrayMarshaler) error {
	if !e.state.text(key) || !e.state.take(1) {
		return e.state.err
	}
	return e.state.failed(e.ObjectEncoder.AddArray(key, alertArrayValue{v, e.state}))
}

// AddReflected observes reflection errors rather than exporting an error replacement.
func (e *alertObjectEncoder) AddReflected(key string, v any) error {
	if !e.state.text(key) || !e.state.take(1) {
		return e.state.err
	}
	return e.state.failed(e.ObjectEncoder.AddReflected(key, v))
}

// AddBinary preserves arbitrary bytes using Zap's standard base64 representation.
func (e *alertObjectEncoder) AddBinary(key string, v []byte) {
	if e.state.text(key) && e.state.take(base64.StdEncoding.EncodedLen(len(v))) {
		e.ObjectEncoder.AddBinary(key, v)
	}
}

// AddByteString validates byte strings as UTF-8 text; Binary handles arbitrary bytes.
func (e *alertObjectEncoder) AddByteString(key string, v []byte) {
	if !utf8.Valid(v) {
		e.state.err = ErrAlertEncoding
		return
	}
	if e.state.text(key) && e.state.take(len(v)) {
		e.ObjectEncoder.AddByteString(key, v)
	}
}

// AddString preserves the exact string when valid and within the whole-entry budget.
func (e *alertObjectEncoder) AddString(key, v string) {
	if e.state.text(key) && e.state.text(v) {
		e.ObjectEncoder.AddString(key, v)
	}
}

// OpenNamespace retains namespace ordering rather than flattening later fields.
func (e *alertObjectEncoder) OpenNamespace(key string) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.OpenNamespace(key)
	}
}

// alertArrayEncoder validates nested array values without changing their representation.
type alertArrayEncoder struct {
	zapcore.ArrayEncoder
	state *alertEncodingState
}

// AppendArray observes nested array failures before whole-entry admission.
func (e *alertArrayEncoder) AppendArray(v zapcore.ArrayMarshaler) error {
	if !e.state.take(1) {
		return e.state.err
	}
	return e.state.failed(e.ArrayEncoder.AppendArray(alertArrayValue{v, e.state}))
}

// AppendObject observes nested object failures before whole-entry admission.
func (e *alertArrayEncoder) AppendObject(v zapcore.ObjectMarshaler) error {
	if !e.state.take(1) {
		return e.state.err
	}
	return e.state.failed(e.ArrayEncoder.AppendObject(alertObjectValue{v, e.state}))
}

// AppendReflected preserves reflected values when the JSON encoder succeeds.
func (e *alertArrayEncoder) AppendReflected(v any) error {
	if !e.state.take(1) {
		return e.state.err
	}
	return e.state.failed(e.ArrayEncoder.AppendReflected(v))
}

// AppendString preserves valid UTF-8 without replacing its contents.
func (e *alertArrayEncoder) AppendString(v string) {
	if e.state.text(v) && e.state.take(1) {
		e.ArrayEncoder.AppendString(v)
	}
}

// AppendByteString validates textual byte arrays before standard Zap serialization.
func (e *alertArrayEncoder) AppendByteString(v []byte) {
	if !utf8.Valid(v) {
		e.state.err = ErrAlertEncoding
		return
	}
	if e.state.take(len(v) + 1) {
		e.ArrayEncoder.AppendByteString(v)
	}
}

// alertObjectValue runs application object serialization once with checked methods.
type alertObjectValue struct {
	value zapcore.ObjectMarshaler
	state *alertEncodingState
}

// MarshalLogObject returns a fixed error if the application serializer fails or panics.
func (v alertObjectValue) MarshalLogObject(enc zapcore.ObjectEncoder) (err error) {
	defer func() {
		if recover() != nil {
			v.state.err = ErrAlertEncoding
			err = ErrAlertEncoding
		}
	}()
	if checked, ok := enc.(*alertObjectEncoder); ok && checked.state == v.state {
		return v.state.failed(v.value.MarshalLogObject(checked))
	}
	return v.state.failed(v.value.MarshalLogObject(&alertObjectEncoder{enc, v.state}))
}

// alertArrayValue runs application array serialization once with checked methods.
type alertArrayValue struct {
	value zapcore.ArrayMarshaler
	state *alertEncodingState
}

// MarshalLogArray returns a fixed error if the application serializer fails or panics.
func (v alertArrayValue) MarshalLogArray(enc zapcore.ArrayEncoder) (err error) {
	defer func() {
		if recover() != nil {
			v.state.err = ErrAlertEncoding
			err = ErrAlertEncoding
		}
	}()
	return v.state.failed(v.value.MarshalLogArray(&alertArrayEncoder{enc, v.state}))
}

// alertStringer observes panics before Zap can convert them into replacement strings.
type alertStringer struct {
	value fmt.Stringer
	state *alertEncodingState
}

// String returns the application-selected text once, recording callback failure separately.
func (v alertStringer) String() (result string) {
	defer func() {
		if recover() != nil {
			if isAlertNilPointer(v.value) {
				result = "<nil>"
			} else {
				v.state.err = ErrAlertEncoding
			}
		}
	}()
	return v.value.String()
}

// alertError retains the original rich error interfaces and eagerly captured basic text.
type alertError struct {
	value error
	state *alertEncodingState
	basic string
}

// Error returns the original basic text without evaluating the application twice.
func (v alertError) Error() string { return v.basic }

// alertErrorText captures basic text and the standard Zap typed-nil fallback.
func alertErrorText(value error, state *alertEncodingState) (basic string, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
			if isAlertNilPointer(value) {
				basic = "<nil>"
			} else {
				state.err = ErrAlertEncoding
			}
		}
	}()
	return value.Error(), false
}

// isAlertNilPointer identifies the nil-receiver panic fallback supported by Zap.
func isAlertNilPointer(value any) bool {
	v := reflect.ValueOf(value)
	return v.IsValid() && v.Kind() == reflect.Pointer && v.IsNil()
}

// alertFormattedError preserves the optional fmt.Formatter contract.
type alertFormattedError struct{ alertError }

// Format invokes the original formatter and observes panics before fmt replaces them.
func (v alertFormattedError) Format(state fmt.State, verb rune) {
	defer func() {
		if recover() != nil {
			v.state.err = ErrAlertEncoding
		}
	}()
	v.value.(fmt.Formatter).Format(state, verb)
}

// alertGroupError preserves the multi-error interface used by Zap.
type alertGroupError struct{ alertError }

// Errors wraps group members without replacing their text or rich-error interfaces.
func (v alertGroupError) Errors() (result []error) {
	defer func() {
		if recover() != nil {
			v.state.err = ErrAlertEncoding
		}
	}()
	original := v.value.(interface{ Errors() []error }).Errors()
	result = make([]error, len(original))
	for i, err := range original {
		if err != nil {
			result[i] = wrapAlertError(err, v.state)
		}
	}
	return result
}

// wrapAlertError retains only the interfaces Zap actually uses, with observed failures.
func wrapAlertError(err error, state *alertEncodingState) error {
	basic, panicked := alertErrorText(err, state)
	wrapped := alertError{value: err, state: state, basic: basic}
	if panicked {
		return wrapped
	}
	if _, ok := err.(interface{ Errors() []error }); ok {
		return alertGroupError{wrapped}
	}
	if _, ok := err.(fmt.Formatter); ok {
		return alertFormattedError{wrapped}
	}
	return wrapped
}

// alertReflectedEncoder preserves Zap's standard encoding/json projection contract.
type alertReflectedEncoder struct {
	writer io.Writer
	limit  int
}

// Encode uses standard JSON normalization and marshaler semantics, with a bounded writer.
// Unlike native String/ByteString fields, Reflect explicitly selects this JSON projection.
func (e *alertReflectedEncoder) Encode(value any) error {
	enc := json.NewEncoder(&alertJSONWriter{writer: e.writer, limit: e.limit})
	enc.SetEscapeHTML(false)
	return errors.Wrap(enc.Encode(value), "encode reflected alert value")
}

// alertJSONWriter rejects oversized or invalid UTF-8 reflected output before copying it.
type alertJSONWriter struct {
	writer io.Writer
	limit  int
}

// Write passes through complete valid JSON encoder output within the explicit limit.
func (w *alertJSONWriter) Write(b []byte) (int, error) {
	if len(b) > w.limit {
		return 0, ErrAlertMessageTooLarge
	}
	if !utf8.Valid(b) {
		return 0, ErrAlertEncoding
	}
	n, err := w.writer.Write(b)
	return n, errors.Wrap(err, "write reflected alert value")
}
