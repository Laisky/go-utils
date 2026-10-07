package utils

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveEmpty(t *testing.T) {
	type args struct {
		vs []string
	}
	tests := []struct {
		name  string
		args  args
		wantR []string
	}{
		{"0", args{[]string{"1"}}, []string{"1"}},
		{"1", args{[]string{"1", ""}}, []string{"1"}},
		{"2", args{[]string{"1", "", "  "}}, []string{"1"}},
		{"3", args{[]string{"1", "", "  ", "2", ""}}, []string{"1", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotR := RemoveEmpty(tt.args.vs); !reflect.DeepEqual(gotR, tt.wantR) {
				t.Errorf("RemoveEmpty() = %v, want %v", gotR, tt.wantR)
			}
		})
	}
}

func TestTrimEleSpaceAndRemoveEmpty(t *testing.T) {
	type args struct {
		vs []string
	}
	tests := []struct {
		name  string
		args  args
		wantR []string
	}{
		{"0", args{[]string{"1"}}, []string{"1"}},
		{"1", args{[]string{"1", ""}}, []string{"1"}},
		{"2", args{[]string{"1", "", "  "}}, []string{"1"}},
		{"3", args{[]string{"1", "", "  ", "2", ""}}, []string{"1", "2"}},
		{"4", args{[]string{"1", "", "  ", "2   ", ""}}, []string{"1", "2"}},
		{"5", args{[]string{"1", "", "  ", "   2   ", ""}}, []string{"1", "2"}},
		{"6", args{[]string{"1", "", "  ", "   2", ""}}, []string{"1", "2"}},
		{"7", args{[]string{"   1", "", "  ", "   2", ""}}, []string{"1", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotR := TrimEleSpaceAndRemoveEmpty(tt.args.vs); !reflect.DeepEqual(gotR, tt.wantR) {
				t.Errorf("TrimEleSpaceAndRemoveEmpty() = %v, want %v", gotR, tt.wantR)
			}
		})
	}
}

// func Test_ConvertMap(t *testing.T) {
// 	{
// 		input := map[any]string{"123": "23"}
// 		got := ConvertMap(input)
// 		t.Log(got)
// 		require.True(t, reflect.DeepEqual(map[string]any{"123": "23"}, got))
// 	}

// 	{
// 		input := map[any]int{"123": 23}
// 		got := ConvertMap(input)
// 		t.Log(got)
// 		require.True(t, reflect.DeepEqual(map[string]any{"123": 23}, got))
// 	}

// 	{
// 		input := map[any]uint{"123": 23}
// 		got := ConvertMap(input)
// 		t.Log(got)
// 		require.True(t, reflect.DeepEqual(map[string]any{"123": uint(23)}, got))
// 	}

// 	{
// 		input := map[string]int{"123": 23}
// 		got := ConvertMap(input)
// 		t.Log(got)
// 		require.True(t, reflect.DeepEqual(map[string]any{"123": 23}, got))
// 	}

// }

func TestConvert2Map(t *testing.T) {
	type args struct {
		inputMap any
	}
	tests := []struct {
		name string
		args args
		want map[string]any
	}{
		{"0", args{map[any]string{"123": "23"}}, map[string]any{"123": "23"}},
		{"1", args{map[any]int{"123": 23}}, map[string]any{"123": 23}},
		{"2", args{map[any]uint{"123": 23}}, map[string]any{"123": uint(23)}},
		{"3", args{map[string]uint{"123": 23}}, map[string]any{"123": uint(23)}},
		{"4", args{map[int]uint{123: 23}}, map[string]any{"123": uint(23)}},
		{"5", args{map[float32]string{float32(123): "23"}}, map[string]any{"123": "23"}},
		{"6", args{map[float32]int{float32(123): 23}}, map[string]any{"123": 23}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertMap2StringKey(tt.args.inputMap); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Benchmark_slice(b *testing.B) {
	type foo struct {
		val string
	}
	payload := RandomStringWithLength(128)

	b.Run("[]struct append", func(b *testing.B) {
		var data []foo
		for i := 0; i < b.N; i++ {
			data = append(data, foo{val: payload})
		}

		b.Log(len(data))
	})

	b.Run("[]*struct append", func(b *testing.B) {
		var data []*foo
		for i := 0; i < b.N; i++ {
			data = append(data, &foo{val: payload})
		}

		b.Log(len(data))
	})

	b.Run("[]struct with prealloc", func(b *testing.B) {
		data := make([]foo, 100)
		for i := 0; i < b.N; i++ {
			data[i%100] = foo{val: payload}
		}
	})

	b.Run("[]*struct with prealloc", func(b *testing.B) {
		data := make([]*foo, 100)
		for i := 0; i < b.N; i++ {
			data[i%100] = &foo{val: payload}
		}
	})
}

func TestContains(t *testing.T) {
	require.True(t, Contains([]string{"1", "2", "3"}, "2"))
	require.False(t, Contains([]string{"1", "2", "3"}, "4"))
	require.True(t, Contains([]int{1, 2, 3}, 2))
	require.False(t, Contains([]int{1, 2, 3}, 4))
}

func TestReverseSlice(t *testing.T) {
	t.Parallel()

	// Test cases
	tests := []struct {
		name     string
		input    []interface{}
		expected []interface{}
	}{
		{
			name:     "Empty Slice",
			input:    []interface{}{},
			expected: []interface{}{},
		},
		{
			name:     "Slice with Even Number of Elements",
			input:    []interface{}{1, 2, 3, 4},
			expected: []interface{}{4, 3, 2, 1},
		},
		{
			name:     "Slice with Odd Number of Elements",
			input:    []interface{}{"a", "b", "c", "d", "e"},
			expected: []interface{}{"e", "d", "c", "b", "a"},
		},
	}

	// Run tests
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Make a copy of the input slice
			input := make([]interface{}, len(test.input))
			copy(input, test.input)

			// Reverse the slice
			ReverseSlice(input)

			// Check if the reversed slice matches the expected result
			if !reflect.DeepEqual(input, test.expected) {
				t.Errorf("unexpected result, got: %v, want: %v", input, test.expected)
			}
		})
	}
}

func TestUniqueStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vs   []string
		want []string
	}{
		{
			name: "Empty input",
			vs:   []string{},
			want: []string{},
		},
		{
			name: "No duplicates",
			vs:   []string{"a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "Duplicates",
			vs:   []string{"a", "b", "a", "c", "b"},
			want: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UniqueStrings(tt.vs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("random", func(t *testing.T) {
		t.Parallel()

		orig := []string{}
		for i := 0; i < 100000; i++ {
			orig = append(orig, RandomStringWithLength(2))
		}
		t.Logf("generate length : %d", len(orig))
		orig = UniqueStrings(orig)
		t.Logf("after unique length : %d", len(orig))
		m := map[string]bool{}
		var ok bool
		for _, v := range orig {
			if _, ok = m[v]; ok {
				t.Fatalf("duplicate: %v", v)
			} else {
				m[v] = ok
			}
		}
	})
}

// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// Benchmark_UniqueStrings
// Benchmark_UniqueStrings/100000
// Benchmark_UniqueStrings/100000-104         	1000000000	         0.003633 ns/op	       0 B/op	       0 allocs/op
func Benchmark_UniqueStrings(b *testing.B) {
	orig := []string{}
	for i := 0; i < b.N; i++ {
		for i := 0; i < 100000; i++ {
			orig = append(orig, RandomStringWithLength(2))
		}

		b.ResetTimer()
	}

	b.Run("100000", func(b *testing.B) {
		orig = UniqueStrings(orig)
	})
}

func TestRemoveEmptyVal(t *testing.T) {
	t.Parallel()

	t.Run("Empty map", func(t *testing.T) {
		m1 := map[string]any{}
		want1 := map[string]any{}
		got1 := RemoveEmptyVal(m1)
		if !reflect.DeepEqual(got1, want1) {
			t.Errorf("Test case 1 failed: got %v, want %v", got1, want1)
		}
	})

	t.Run("Map with non-empty values", func(t *testing.T) {
		m2 := map[string]any{
			"a": 1,
			"b": "hello",
			"c": map[string]any{
				"d": 2,
				"e": "",
			},
		}
		want2 := map[string]any{
			"a": 1,
			"b": "hello",
			"c": map[string]any{
				"d": 2,
			},
		}
		got2 := RemoveEmptyVal(m2)
		if !reflect.DeepEqual(got2, want2) {
			t.Errorf("Test case 2 failed: got %v, want %v", got2, want2)
		}
	})

	t.Run("Map with nested empty maps", func(t *testing.T) {
		m3 := map[string]any{
			"a": map[string]any{},
			"b": map[string]any{
				"c": map[string]any{},
			},
		}
		want3 := map[string]any{}
		got3 := RemoveEmptyVal(m3)
		if !reflect.DeepEqual(got3, want3) {
			t.Errorf("Test case 3 failed: got %v, want %v", got3, want3)
		}
	})

	t.Run("Map with nested non-empty maps", func(t *testing.T) {
		m3 := map[string]any{
			"a": map[string]any{},
			"b": map[string]any{
				"c": map[string]any{},
				"d": 123,
			},
			"e": map[string]string{},
			"f": map[string]string{
				"g": "123",
			},
		}
		want3 := map[string]any{
			"b": map[string]any{
				"d": 123,
			},
			"f": map[string]string{
				"g": "123",
			},
		}
		got3 := RemoveEmptyVal(m3)
		if !reflect.DeepEqual(got3, want3) {
			t.Errorf("Test case 3 failed: got %v, want %v", got3, want3)
		}
	})

	t.Run("map with empty slice", func(t *testing.T) {
		m4 := map[string]any{
			"a": []string{},
			"b": 123,
		}
		want4 := map[string]any{"b": 123}
		got4 := RemoveEmptyVal(m4)
		if !reflect.DeepEqual(got4, want4) {
			t.Errorf("Test case 4 failed: got %v, want %v", got4, want4)
		}
	})

	t.Run("map with nil", func(t *testing.T) {
		m5 := map[string]any{
			"a": nil,
		}
		want5 := map[string]any{}
		got5 := RemoveEmptyVal(m5)
		if !reflect.DeepEqual(got5, want5) {
			t.Errorf("Test case 5 failed: got %v, want %v", got5, want5)
		}
	})
}

func TestFilterSlice(t *testing.T) {
	t.Parallel()

	// Test case 1: Filter even numbers
	s1 := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	f1 := func(v int) bool {
		return v%2 == 0
	}
	expected1 := []int{2, 4, 6, 8, 10}
	result1 := FilterSlice(s1, f1)
	if !reflect.DeepEqual(result1, expected1) {
		t.Errorf("FilterSlice failed for test case 1, expected: %v, got: %v", expected1, result1)
	}

	// Test case 2: Filter strings with length greater than 3
	s2 := []string{"apple", "banana", "cat", "dog", "elephant"}
	f2 := func(v string) bool {
		return len(v) > 3
	}
	expected2 := []string{"apple", "banana", "elephant"}
	result2 := FilterSlice(s2, f2)
	if !reflect.DeepEqual(result2, expected2) {
		t.Errorf("FilterSlice failed for test case 2, expected: %v, got: %v", expected2, result2)
	}

	// Test case 3: Filter structs based on a condition
	type Person struct {
		Name   string
		Age    int
		Gender string
	}
	s3 := []Person{
		{Name: "Alice", Age: 25, Gender: "Female"},
		{Name: "Bob", Age: 30, Gender: "Male"},
		{Name: "Charlie", Age: 20, Gender: "Male"},
		{Name: "Diana", Age: 35, Gender: "Female"},
	}
	f3 := func(v Person) bool {
		return v.Age > 25 && v.Gender == "Female"
	}
	expected3 := []Person{
		{Name: "Diana", Age: 35, Gender: "Female"},
	}
	result3 := FilterSlice(s3, f3)
	if !reflect.DeepEqual(result3, expected3) {
		t.Errorf("FilterSlice failed for test case 3, expected: %v, got: %v", expected3, result3)
	}
}
