package json

import (
	"encoding/json"
	"testing"

	"github.com/brianvoe/gofakeit/v6"
	json2 "github.com/go-json-experiment/json"
	"github.com/stretchr/testify/require"
)

func TestUnmarshal(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	testCases := []struct {
		name     string
		input    []byte
		expected testStruct
		errMsg   string
	}{
		{
			name:     "0",
			input:    []byte(`{"name": "John", "age": 30}`),
			expected: testStruct{Name: "John", Age: 30},
		},
		{
			name:     "1",
			input:    []byte{},
			expected: testStruct{},
			errMsg:   "unexpected EOF",
		},
		{
			name:     "2",
			input:    []byte(`{"age": "30"}`),
			expected: testStruct{},
			errMsg:   "unable to unmarshal JSON string into Go value of type int",
		},
		{
			name:     "3",
			input:    []byte(`{"name": "John", "age": 30, "extra": "extra"}`),
			expected: testStruct{Name: "John", Age: 30},
		},
		{
			name:     "4",
			input:    []byte(`{"name": 123, "age": 30}`),
			expected: testStruct{},
			errMsg:   "unable to unmarshal JSON string into Go value of type string",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actual testStruct
			err := Unmarshal(tc.input, &actual)
			if tc.errMsg != "" {
				require.Error(t, err, tc.errMsg)
				return
			}

			require.NoErrorf(t, err, "[%s]", tc.name)
			require.Equal(t, tc.expected, actual)
		})
	}
}

// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils/v4/json
// cpu: AMD Ryzen 7 5700G with Radeon Graphics
// Benchmark_json_v1_v2/v1-marshal-16         	 2175998	       538.8 ns/op	     152 B/op	       2 allocs/op
// Benchmark_json_v1_v2/v1-unmarshal-16       	  726795	      1432 ns/op	     288 B/op	       7 allocs/op
// Benchmark_json_v1_v2/v2-marshal-16         	 1440228	       823.2 ns/op	     368 B/op	       5 allocs/op
// Benchmark_json_v1_v2/v2-unmarshal-16       	 1767696	       678.7 ns/op	       0 B/op	       0 allocs/op
// Benchmark_json_v1_v2/gutils-marshal-16     	 1413717	       839.9 ns/op	     368 B/op	       5 allocs/op
// Benchmark_json_v1_v2/gutils-unmarshal-comment-16         	  422198	      2765 ns/op	    1492 B/op	      14 allocs/op
func Benchmark_json_v1_v2(b *testing.B) {
	type testStruct struct {
		Name    string `json:"name"`
		Age     int    `json:"age"`
		Address string `json:"address"`
	}
	dataStruct := &testStruct{
		Name:    gofakeit.Name(),
		Age:     gofakeit.Number(1, 100),
		Address: gofakeit.Address().Address,
	}
	data, err := json.Marshal(dataStruct)
	require.NoError(b, err)

	b.Run("v1-marshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := json.Marshal(data)
			require.NoError(b, err)
		}
	})
	b.Run("v1-unmarshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err = json.Unmarshal(data, dataStruct)
			require.NoError(b, err)
		}
	})

	b.Run("v2-marshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := json2.Marshal(data)
			require.NoError(b, err)
		}
	})
	b.Run("v2-unmarshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err = json2.Unmarshal(data, dataStruct)
			require.NoError(b, err)
		}
	})

	b.Run("gutils-marshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := Marshal(data)
			require.NoError(b, err)
		}
	})
	b.Run("gutils-unmarshal-comment", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err = UnmarshalComment(data, dataStruct)
			require.NoError(b, err)
		}
	})
}
