package utils

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/go-utils/v6/log"
)

func TestJSON(t *testing.T) {
	t.Parallel()

	t.Run("marshal", func(t *testing.T) {
		jb, err := json.Marshal("123")
		require.NoError(t, err)

		var v string
		json.Unmarshal(jb, &v)
		require.NoError(t, err)
		require.Equal(t, "123", v)
	})

	t.Run("marshal string", func(t *testing.T) {
		jb, err := json.MarshalToString("123")
		require.NoError(t, err)

		var v string
		json.UnmarshalFromString(jb, &v)
		require.NoError(t, err)
		require.Equal(t, "123", v)
	})

	t.Run("json not support comment", func(t *testing.T) {
		d := struct {
			K string
		}{}
		raw := `{
			// comment
			"k": "v"  // comment
			}`
		err := json.Unmarshal([]byte(raw), &d)
		require.ErrorContains(t, err, "invalid character '/'")
	})

	t.Run("json support comment", func(t *testing.T) {
		d := struct {
			K string
		}{}
		raw := `{
			// comment
			"k": "v"  // comment
			}`
		err := json.UnmarshalComment([]byte(raw), &d)
		require.NoError(t, err)
		require.Equal(t, "v", d.K)
	})
}

/*
cpu: Intel(R) Core(TM) i7-4790 CPU @ 3.60GHz
Benchmark_Str2Bytes/normal_str2bytes-8         	  868298	      1156 ns/op	    1024 B/op	       1 allocs/op
Benchmark_Str2Bytes/normal_bytes2str-8         	 1000000	      1216 ns/op	    1024 B/op	       1 allocs/op
Benchmark_Str2Bytes/unsafe_str2bytes-8         	11335250	        92.66 ns/op	       0 B/op	       0 allocs/op
Benchmark_Str2Bytes/unsafe_bytes2str-8         	11320952	       106.2 ns/op	       0 B/op	       0 allocs/op
PASS
*/
func Benchmark_Str2Bytes(b *testing.B) {
	rawStr := RandomStringWithLength(1024)
	rawBytes := []byte(rawStr)
	b.Run("normal_str2bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = []byte(rawStr)
		}
	})
	b.Run("normal_bytes2str", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = string(rawBytes)
		}
	})
	b.Run("unsafe_str2bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Str2Bytes(rawStr)
		}
	})
	b.Run("unsafe_bytes2str", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Bytes2Str(rawBytes)
		}
	})
}

func TestBytes2Str(t *testing.T) {
	rawStr := RandomStringWithLength(1024)
	rawBytes := []byte(rawStr)
	str := Bytes2Str(rawBytes)
	require.Equal(t, rawStr, str)

	// case: bytes should changed by string
	{
		rawBytes[0] = '@'
		rawBytes[1] = 'a'
		rawBytes[2] = 'b'
		rawBytes[3] = 'c'
		require.Equal(t, string(rawBytes), str)
	}

	// case: Str2Bytes should return the same bytes struct
	{
		newBytes := Str2Bytes(str)
		require.Equal(t, fmt.Sprintf("%x", newBytes), fmt.Sprintf("%x", rawBytes))
	}
}

func TestJSONMd5(t *testing.T) {
	type args struct {
		data any
	}
	type foo struct {
		Name string `json:"name"`
	}
	var nilArgs *foo
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"0", args{nil}, "", true},
		{"1", args{nilArgs}, "", true},
		{"2", args{foo{}}, "555dfa90763bd852d5dd9144887eed97", false},
		{"3", args{new(foo)}, "555dfa90763bd852d5dd9144887eed97", false},
		{"4", args{foo{""}}, "555dfa90763bd852d5dd9144887eed97", false},
		{"5", args{foo{Name: "a"}}, "88148e411b9b424a2e0ddf108cb02baa", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MD5JSON(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("MD5JSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MD5JSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewHasPrefixWithMagic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prefix []byte
		input  []byte
		want   bool
	}{
		{
			name:   "8-byte prefix match",
			prefix: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			input:  []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09},
			want:   true,
		},
		{
			name:   "8-byte prefix no match",
			prefix: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			input:  []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x09},
			want:   false,
		},
		{
			name:   "4-byte prefix match",
			prefix: []byte{0x01, 0x02, 0x03, 0x04},
			input:  []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			want:   true,
		},
		{
			name:   "4-byte prefix no match",
			prefix: []byte{0x01, 0x02, 0x03, 0x04},
			input:  []byte{0x01, 0x02, 0x03, 0x05},
			want:   false,
		},
		{
			name:   "2-byte prefix match",
			prefix: []byte{0x01, 0x02},
			input:  []byte{0x01, 0x02, 0x03},
			want:   true,
		},
		{
			name:   "2-byte prefix no match",
			prefix: []byte{0x01, 0x02},
			input:  []byte{0x01, 0x03},
			want:   false,
		},
		{
			name:   "empty prefix",
			prefix: []byte{},
			input:  []byte{0x01, 0x02},
			want:   true,
		},
		{
			name:   "non-matching prefix",
			prefix: []byte{0x01, 0x02, 0x03},
			input:  []byte{0x04, 0x05, 0x06},
			want:   false,
		},
		{
			name:   "longer prefix",
			prefix: []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			input:  []byte{0x01, 0x02, 0x03, 0x04},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasPrefix := NewHasPrefixWithMagic(tt.prefix)
			if got := hasPrefix(tt.input); got != tt.want {
				t.Errorf("input: %x, prefix: %x, want: %v, got: %v",
					tt.input, tt.prefix, tt.want, got)
			}
		})
	}
}

// cpu: AMD Ryzen 7 5700G with Radeon Graphics
// Benchmark_HasPrefix/std-8         	404345066	         3.031 ns/op	       0 B/op	       0 allocs/op
// Benchmark_HasPrefix/custom-8      	562408310	         2.133 ns/op	       0 B/op	       0 allocs/op
// PASS
func Benchmark_HasPrefix(b *testing.B) {
	val := []byte("hello, world")
	prefix := []byte("hell")

	b.Run("std", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			bytes.HasPrefix(val, prefix)
		}
	})

	hasprefix := NewHasPrefixWithMagic(prefix)
	b.Run("custom", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hasprefix(val)
		}
	})
}

func TestDecodeByBase64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expected  []byte
		expectErr bool
	}{
		{
			name:      "empty string",
			input:     "",
			expected:  []byte{},
			expectErr: false,
		},
		{
			name:      "valid base64 string",
			input:     "aGVsbG8=",
			expected:  []byte("hello"),
			expectErr: false,
		},
		{
			name:      "invalid base64 string",
			input:     "invalid!@#$",
			expected:  nil,
			expectErr: true,
		},
		{
			name:      "base64 with padding",
			input:     "YWJjZA==",
			expected:  []byte("abcd"),
			expectErr: false,
		},
		{
			name:      "base64 without padding",
			input:     "YWJjZA",
			expected:  []byte(""),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeByBase64(tt.input)
			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestEncodeByBase64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty bytes",
			input:    []byte{},
			expected: "",
		},
		{
			name:     "basic string",
			input:    []byte("hello"),
			expected: "aGVsbG8=",
		},
		{
			name:     "binary data",
			input:    []byte{0x00, 0x01, 0x02, 0x03},
			expected: "AAECAw==",
		},
		{
			name:     "special characters",
			input:    []byte("!@#$%^&*()"),
			expected: "IUAjJCVeJiooKQ==",
		},
		{
			name:     "unicode string",
			input:    []byte("你好"),
			expected: "5L2g5aW9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeByBase64(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestBase64RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "empty",
			input: []byte{},
		},
		{
			name:  "ascii string",
			input: []byte("Hello, World!"),
		},
		{
			name:  "binary data",
			input: []byte{0xFF, 0x00, 0xAB, 0xCD},
		},
		{
			name:  "unicode string",
			input: []byte("你好世界"),
		},
		{
			name:  "long string",
			input: []byte(RandomStringWithLength(1024)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeByBase64(tt.input)
			decoded, err := DecodeByBase64(encoded)
			require.NoError(t, err)
			require.Equal(t, tt.input, decoded)
		})
	}
}

func ExampleEncodeByBase64() {
	input := []byte("Hello, World!")
	encoded := EncodeByBase64(input)
	fmt.Println(encoded)
	// Output: SGVsbG8sIFdvcmxkIQ==
}

func ExampleDecodeByBase64() {
	input := "SGVsbG8sIFdvcmxkIQ=="
	decoded, err := DecodeByBase64(input)
	if err != nil {
		log.Shared.Error("decode base64", zap.Error(err))
		return
	}
	fmt.Println(string(decoded))
	// Output: Hello, World!
}
