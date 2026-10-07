package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestJaegerTracingID verifies that NewJaegerTracingID formats explicit IDs as hex "trace:span:parent:flags" (with
// an empty parent for zero) that Parse round-trips, that a zero trace or span ID is not an error but is replaced by
// a random non-zero value while the other components are kept, and that a zero flag defaults to 0x04.
func TestJaegerTracingID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		traceID      uint64
		spanID       uint64
		parentSpanID uint64
		flag         byte
		// want is the exact encoding, or empty when a random component makes it unpredictable.
		want JaegerTracingID
		// randomTrace and randomSpan report that the zero input must be replaced by a random non-zero ID.
		randomTrace bool
		randomSpan  bool
		wantFlag    byte
	}{
		{
			name:     "explicit IDs, root span",
			traceID:  123456789,
			spanID:   987654321,
			flag:     0x04,
			want:     "75bcd15:3ade68b1::4",
			wantFlag: 0x04,
		},
		{
			name:         "explicit IDs with parent",
			traceID:      123456789,
			spanID:       987654321,
			parentSpanID: 0xabc,
			flag:         0x01,
			want:         "75bcd15:3ade68b1:abc:1",
			wantFlag:     0x01,
		},
		{
			name:     "zero flag defaults to 0x04",
			traceID:  1,
			spanID:   2,
			want:     "1:2::4",
			wantFlag: 0x04,
		},
		{
			name:        "zero trace ID is replaced by a random one",
			spanID:      987654321,
			flag:        0x04,
			randomTrace: true,
			wantFlag:    0x04,
		},
		{
			name:       "zero span ID is replaced by a random one",
			traceID:    123456789,
			flag:       0x04,
			randomSpan: true,
			wantFlag:   0x04,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewJaegerTracingID(tt.traceID, tt.spanID, tt.parentSpanID, tt.flag)
			require.NoError(t, err, "zero IDs are replaced, never reported as errors")
			if tt.want != "" {
				require.Equal(t, tt.want.String(), got.String())
			}

			traceID, spanID, parentSpanID, flag, err := got.Parse()
			require.NoError(t, err, "every generated tracing ID must parse")
			if tt.randomTrace {
				require.NotZero(t, traceID)
			} else {
				require.Equal(t, tt.traceID, traceID)
			}
			if tt.randomSpan {
				require.NotZero(t, spanID)
			} else {
				require.Equal(t, tt.spanID, spanID)
			}
			require.Equal(t, tt.parentSpanID, parentSpanID)
			require.Equal(t, tt.wantFlag, flag)
		})
	}
}

// TestJaegerTracingIDParse verifies Parse against well-formed and malformed tracing IDs: malformed component
// counts, empty or non-hexadecimal components, and zero trace or span IDs are errors with zero results, while a
// zero or empty parent ID and a zero flags byte are accepted.
func TestJaegerTracingIDParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        JaegerTracingID
		wantErr    bool
		wantTrace  uint64
		wantSpan   uint64
		wantParent uint64
		wantFlag   byte
	}{
		{name: "root span", raw: "75bcd15:3ade68b1::4", wantTrace: 0x75bcd15, wantSpan: 0x3ade68b1, wantFlag: 4},
		{name: "explicit zero parent and flags", raw: "1:2:0:0", wantTrace: 1, wantSpan: 2},
		{name: "child span", raw: "a:b:c:d", wantTrace: 0xa, wantSpan: 0xb, wantParent: 0xc, wantFlag: 0xd},
		{name: "empty", raw: "", wantErr: true},
		{name: "three components", raw: "1:2:3", wantErr: true},
		{name: "five components", raw: "1:2:3:4:5", wantErr: true},
		{name: "empty trace ID", raw: ":2:3:4", wantErr: true},
		{name: "non-hexadecimal span ID", raw: "1:xyz:3:4", wantErr: true},
		{name: "zero trace ID", raw: "0:2::4", wantErr: true},
		{name: "zero-padded zero span ID", raw: "1:0000000000000000::4", wantErr: true},
		{name: "flags wider than one byte", raw: "1:2:3:100", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			traceID, spanID, parentSpanID, flag, err := tt.raw.Parse()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantTrace, traceID)
			require.Equal(t, tt.wantSpan, spanID)
			require.Equal(t, tt.wantParent, parentSpanID)
			require.Equal(t, tt.wantFlag, flag)
		})
	}
}
