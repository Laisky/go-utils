package log

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
)

// alertCore snapshots bound context with the same eager semantics as Zap's IO core.
type alertCore struct {
	alert   *Alert
	enc     zapcore.Encoder
	bindErr error
	stopped bool
}

// GetZapCore returns an independent alert core for zapcore.NewTee or zap.WrapCore.
// With snapshots fields eagerly, including namespaces and mutable objects/bytes.
// Sync does not flush the asynchronous sender; Close cancels it and Done waits
// for reference release. Successful Write means local admission only.
func (a *Alert) GetZapCore() zapcore.Core { return &alertCore{alert: a, enc: a.newAlertEncoder()} }

// Enabled reports whether the alert level admits this entry.
func (c *alertCore) Enabled(level zapcore.Level) bool { return c.alert.level.Enabled(level) }

// With clones encoded context and snapshots newly bound fields without retaining them.
func (c *alertCore) With(fields []zapcore.Field) zapcore.Core {
	child := &alertCore{alert: c.alert, enc: c.enc.Clone(), bindErr: c.bindErr, stopped: c.stopped}
	if child.bindErr != nil || child.stopped {
		return child
	}
	selected, err := c.alert.selectAlertFields(fields)
	if err != nil {
		child.bindErr = err
		return child
	}
	child.bindErr = addAlertFields(child.enc, selected, c.alert.maxMessageBytes)
	// Legacy filtered namespaces stop all later fields, including later With calls.
	if c.alert.filteredAlertFields() {
		for _, f := range fields {
			if f.Type == zapcore.NamespaceType {
				child.stopped = true
				break
			}
		}
	}
	return child
}

// Check adds this core when the configured alert level accepts the entry.
func (c *alertCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

// Write encodes a complete entry and queues it, returning any admission error.
func (c *alertCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if err := c.alert.checkAlertEntry(entry); err != nil {
		return err
	}
	if c.bindErr != nil {
		return c.bindErr
	}
	enc := c.enc.Clone()
	if !c.stopped {
		selected, err := c.alert.selectAlertFields(fields)
		if err != nil {
			return err
		}
		if err = addAlertFields(enc, selected, c.alert.maxMessageBytes); err != nil {
			return err
		}
	}
	return c.alert.sendAlertEntry(entry, enc)
}

// Sync returns immediately; remote delivery has no synchronous flush contract.
func (c *alertCore) Sync() error { return nil }

// filteredAlertFields reports whether the caller explicitly selected a content policy.
func (a *Alert) filteredAlertFields() bool { return a.diagnosticRedaction || a.allowedFields != nil }

// selectAlertFields applies only explicitly selected legacy content policies.
func (a *Alert) selectAlertFields(fields []zapcore.Field) ([]zapcore.Field, error) {
	if a.filteredAlertFields() {
		return a.remoteFields(fields)
	}
	return fields, nil
}

// checkAlertEntry rejects closed, invalid-text or oversized metadata before encoding.
func (a *Alert) checkAlertEntry(entry zapcore.Entry) error {
	if a.closed.Load() || a.ctx.Err() != nil {
		return errors.New("alert is closed")
	}
	if !utf8.ValidString(entry.Message) || !utf8.ValidString(entry.LoggerName) {
		return errors.WithStack(ErrAlertEncoding)
	}
	if len(entry.Message) > a.maxMessageBytes || len(entry.LoggerName) > a.maxMessageBytes-len(entry.Message) {
		return errors.WithStack(ErrAlertMessageTooLarge)
	}
	return nil
}

// sendAlertEntry composes the existing wire message and applies the final whole-message cap.
func (a *Alert) sendAlertEntry(entry zapcore.Entry, enc zapcore.Encoder) error {
	if a.filteredAlertFields() {
		entry.Message = a.sanitizeDiagnosticText(entry.Message)
		entry.LoggerName = a.sanitizeDiagnosticText(entry.LoggerName)
	}
	buffer, err := enc.EncodeEntry(zapcore.Entry{}, nil)
	if buffer != nil {
		defer buffer.Free()
	}
	if err != nil || buffer == nil {
		return errors.WithStack(ErrAlertEncoding)
	}
	if buffer.Len() > a.maxMessageBytes {
		return errors.WithStack(ErrAlertMessageTooLarge)
	}
	msg := "logger: `" + entry.LoggerName + "`\n" +
		"time: `" + entry.Time.UTC().Format(time.RFC3339Nano) + "`\n" +
		"level: `" + entry.Level.String() + "`\n" +
		"message: `" + entry.Message + "`\n" + strings.TrimSpace(buffer.String())
	return a.Send(msg)
}

// Fields returns an immutable Inline snapshot of encoded bound context.
// It retains JSON numbers and duplicate keys without original callback references.
// Closed namespaces cannot restore binding order in a legacy hook; use this core
// directly, without adding a redundant alert hook around a tee.
func (c *alertCore) Fields() []zapcore.Field {
	if c.bindErr != nil {
		return nil
	}
	buffer, err := c.enc.EncodeEntry(zapcore.Entry{}, nil)
	if buffer != nil {
		defer buffer.Free()
	}
	if err != nil || buffer == nil {
		return nil
	}
	raw := strings.TrimSpace(buffer.String())
	if raw == "{}" {
		return nil
	}
	return []zapcore.Field{{Type: zapcore.InlineMarshalerType, Interface: alertContextSnapshot(raw)}}
}
