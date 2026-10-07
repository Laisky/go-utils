package utils

import (
	"fmt"
	"testing"
)

// TestURLMasking verifies that URLMasking replaces only the userinfo password of http and https URLs with the
// given mask, leaving the scheme, username, host, and path unchanged.
func TestURLMasking(t *testing.T) {
	t.Parallel()

	type testcase struct {
		input  string
		output string
	}

	var (
		ret  string
		mask = "*****"
	)
	for _, tc := range []*testcase{
		{
			"http://12ijij:3j23irj@jfjlwef.ffe.com",
			"http://12ijij:" + mask + "@jfjlwef.ffe.com",
		},
		{
			"https://12ijij:3j23irj@123.1221.14/13",
			"https://12ijij:" + mask + "@123.1221.14/13",
		},
	} {
		ret = URLMasking(tc.input, mask)
		if ret != tc.output {
			t.Fatalf("expect %v, got %v", tc.output, ret)
		}
	}
}

// ExampleURLMasking demonstrates replacing the password in a URL's userinfo with "*****".
func ExampleURLMasking() {
	originURL := "http://12ijij:3j23irj@jfjlwef.ffe.com"
	newURL := URLMasking(originURL, "*****")
	fmt.Println(newURL)
	// Output: http://12ijij:*****@jfjlwef.ffe.com
}
