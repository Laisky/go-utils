package utils

import (
	"encoding/asn1"
	"fmt"
	"regexp"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

// TestRegexNamedSubMatch2 verifies that RegexNamedSubMatch2 extracts named capture groups from a pipe-separated
// log line, returning "INFO" for the level group and "74" for the line group.
func TestRegexNamedSubMatch2(t *testing.T) {
	t.Parallel()

	reg := regexp.MustCompile(`^(?P<time>.{23}) {0,}\| {0,}(?P<app>[^ ]+) {0,}\| {0,}(?P<level>[^ ]+) {0,}\| {0,}(?P<thread>[^ ]+) {0,}\| {0,}(?P<class>[^ ]+) {0,}\| {0,}(?P<line>\d+) {0,}([\|:] {0,}(?P<args>\{.*\})){0,1}([\|:] {0,}(?P<message>.*)){0,1}`)
	str := "2018-04-02 02:02:10.928 | sh-datamining | INFO | http-nio-8080-exec-80 | com.pateo.qingcloud.gateway.core.zuul.filters.post.LogFilter | 74 | xxx"
	submatchMap, err := RegexNamedSubMatch2(reg, str)
	require.NoError(t, err)

	for k, v := range submatchMap {
		fmt.Println(">>", k, ":", v)
	}

	if v1, ok := submatchMap["level"]; !ok {
		t.Fatalf("`level` should exists")
	} else if v1 != "INFO" {
		t.Fatalf("`level` shoule be `INFO`, but got: %v", v1)
	}
	if v2, ok := submatchMap["line"]; !ok {
		t.Fatalf("`line` should exists")
	} else if v2 != "74" {
		t.Fatalf("`line` shoule be `74`, but got: %v", v2)
	}
}

// TestRegexNamedSubMatch verifies that the deprecated RegexNamedSubMatch fills a caller-provided map with the
// named capture groups of a pipe-separated log line, yielding "INFO" for level and "74" for line.
func TestRegexNamedSubMatch(t *testing.T) {
	t.Parallel()

	reg := regexp.MustCompile(`^(?P<time>.{23}) {0,}\| {0,}(?P<app>[^ ]+) {0,}\| {0,}(?P<level>[^ ]+) {0,}\| {0,}(?P<thread>[^ ]+) {0,}\| {0,}(?P<class>[^ ]+) {0,}\| {0,}(?P<line>\d+) {0,}([\|:] {0,}(?P<args>\{.*\})){0,1}([\|:] {0,}(?P<message>.*)){0,1}`)
	str := "2018-04-02 02:02:10.928 | sh-datamining | INFO | http-nio-8080-exec-80 | com.pateo.qingcloud.gateway.core.zuul.filters.post.LogFilter | 74 | xxx"
	submatchMap := map[string]string{}
	if err := RegexNamedSubMatch(reg, str, submatchMap); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	for k, v := range submatchMap {
		fmt.Println(">>", k, ":", v)
	}

	if v1, ok := submatchMap["level"]; !ok {
		t.Fatalf("`level` should exists")
	} else if v1 != "INFO" {
		t.Fatalf("`level` shoule be `INFO`, but got: %v", v1)
	}
	if v2, ok := submatchMap["line"]; !ok {
		t.Fatalf("`line` should exists")
	} else if v2 != "74" {
		t.Fatalf("`line` shoule be `74`, but got: %v", v2)
	}
}

// ExampleRegexNamedSubMatch demonstrates extracting a named capture group into a caller-provided map with
// RegexNamedSubMatch.
func ExampleRegexNamedSubMatch() {
	reg := regexp.MustCompile(`(?P<key>\d+.*)`)
	str := "12345abcde"
	groups := map[string]string{}
	if err := RegexNamedSubMatch(reg, str, groups); err != nil {
		log.Shared.Error("try to group match got error", zap.Error(err))
	}

	fmt.Println(groups)
	// Output: map[key:12345abcde]

}

// TestTemplateWithMap verifies that TemplateWithMap substitutes ${key} placeholders with int, string, and float64
// values, including keys that contain a hyphen.
func TestTemplateWithMap(t *testing.T) {
	t.Parallel()

	tpl := `123${k1} + ${k2}:${k-3} 22`
	data := map[string]any{
		"k1":  41,
		"k2":  "abc",
		"k-3": 213.11,
	}
	want := `12341 + abc:213.11 22`
	got := TemplateWithMap(tpl, data)
	if got != want {
		t.Fatalf("want `%v`, got `%v`", want, got)
	}
}

// TestDedent verifies that Dedent strips the smallest common leading indentation, expands leading tabs to the
// configured number of spaces (four by default), keeps interior blank lines, and drops leading and trailing
// blank lines.
func TestDedent(t *testing.T) {
	// t.Run("normal", func(t *testing.T) {
	// 	v := `
	// 	123
	// 	234
	// 	 345
	// 		222
	// 	`

	// 	dedent := Dedent(v, WithReplaceTabBySpaces(4))
	// 	require.Equal(t, "123\n234\n 345\n    222", dedent)
	// })

	t.Run("normal with blank lines", func(t *testing.T) {
		v := `
		123


		234

		 345
			222
		`

		dedent := Dedent(v, WithReplaceTabBySpaces(4))
		require.Equal(t, "123\n\n\n234\n\n 345\n    222", dedent)
	})

	t.Run("3 blanks", func(t *testing.T) {
		v := `
		123
		234
		 345	2
			222
		`

		dedent := Dedent(v, WithReplaceTabBySpaces(3))
		require.Equal(t, "123\n234\n 345\t2\n   222", dedent)
	})

	t.Run("shrink", func(t *testing.T) {
		v := `
		123
	   234
		`

		dedent := Dedent(v)
		require.Equal(t, " 123\n234", dedent)
	})

	t.Run("shrink with blank line", func(t *testing.T) {
		v := `
		123

	   234
		`

		dedent := Dedent(v)
		require.Equal(t, " 123\n\n234", dedent)
	})

}

// TestParseObjectIdentifier verifies that ParseObjectIdentifier parses dotted decimal strings into
// asn1.ObjectIdentifier values and rejects negative or non-numeric components and trailing dots with an
// "invalid oid format" error.
func TestParseObjectIdentifier(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input string
		want  asn1.ObjectIdentifier
		err   string
	}{
		{
			input: "1.2.3",
			want:  asn1.ObjectIdentifier{1, 2, 3},
			err:   "",
		},
		{
			input: "1.2.3.4.5",
			want:  asn1.ObjectIdentifier{1, 2, 3, 4, 5},
			err:   "",
		},
		{
			input: "1.2.3.4.55555",
			want:  asn1.ObjectIdentifier{1, 2, 3, 4, 55555},
			err:   "",
		},
		{
			input: "1.2.3.4.55555.2",
			want:  asn1.ObjectIdentifier{1, 2, 3, 4, 55555, 2},
			err:   "",
		},
		{
			input: "1.2.3.4.55555.-2",
			want:  nil,
			err:   fmt.Sprintf("invalid oid format"),
		},
		{
			input: "1.2.a",
			want:  nil,
			err:   "invalid oid format",
		},
		{
			input: "1.2.3.",
			want:  nil,
			err:   "invalid oid format",
		},
		{
			input: "1.2.3.4.5.",
			want:  nil,
			err:   "invalid oid format",
		},
	}

	for _, tc := range testCases {
		got, err := ParseObjectIdentifier(tc.input)
		if tc.err == "" {
			require.Equalf(t, tc.want, got, "input: %q", tc.input)
			continue
		}

		require.ErrorContainsf(t, err, tc.err, "input: %q", tc.input)
	}
}
