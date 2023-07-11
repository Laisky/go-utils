package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var renameAvTestCases = []struct {
	source string
	target string
}{
	{"gg5.co@CAWD-533-C_GG5.mp4", "cawd-533.mp4"},
	{"gg5.co@SSIS-669-C_GG5.mp4", "ssis-669.mp4"},
	{"heyzo_hd_0671_full.mp4", "heyzo-0671.mp4"},
	{"heyzo_hd-0671_full.mp4", "heyzo-0671.mp4"},
	{"heyzo_hd-0671_full.MP4", "heyzo-0671.mp4"},
	{"heyzo_hd-0671_full.avi", "heyzo-0671.avi"},
	{"heyzo_hd-0671_full.AVI", "heyzo-0671.avi"},
	{"hhd800.com@277DCV-219.mp4", "dcv-219.mp4"},
	{"hhd800.com@277DCV-219.MP4", "dcv-219.mp4"},
	{"hhd800.com@FC2-PPV-3129809.mp4", "fc2-ppv-3129809.mp4"},
	{"hhd800.com@FC2-PPV-3192969_1.mp4", "fc2-ppv-3192969_1.mp4"},
	{"hhd800.com@FC2-PPV-3192969_2.mp4", "fc2-ppv-3192969_2.mp4"},
	{"hhd800.com@FSDSS-534.mp4", "fsdss-534.mp4"},
	{"hhd800.com@IPX-950-C_X1080X.mp4", "ipx-950.mp4"},
	{"hhd800.com@PPT-137.mp4", "ppt-137.mp4"},
	{"hhd800.com@STARS-209_UNCENSORED_LEAKED.mp4", "stars-209.mp4"},
	{"SSIS-448-C.mp4", "ssis-448.mp4"},
	{"[98t.tv]dass-015.mp4", "dass-015.mp4"},
	{"Woxav.Com@MIAA-293 姉の挑発を真に受けた童貞弟がイッてる 深田えいみ Uncensored 破解版.mp4", "miaa-293.mp4"},

	{"电锯人04.mp4", "电锯人04.mp4"},
	{"Manufactured.Landscapes.2006.1080p.BluRay.x264-HANDJOB.mkv", "Manufactured.Landscapes.2006.1080p.BluRay.x264-HANDJOB.mkv"},
}

func Test_convertAvFilename(t *testing.T) {
	for _, c := range renameAvTestCases {
		got := convertAvFilename(c.source)
		require.Equal(t, c.target, got, "source is %q, expected is %q, but got %q", c.source, c.target, got)
	}
}
