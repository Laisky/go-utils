package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var renameAvTestCases = []struct {
	id     string
	source string
	target string
}{
	{"0", "gg5.co@CAWD-533-C_GG5.mp4", "cawd-533.mp4"},
	{"1", "gg5.co@SSIS-669-C_GG5.mp4", "ssis-669.mp4"},
	{"2", "heyzo_hd_0671_full.mp4", "heyzo-0671.mp4"},
	{"3", "heyzo_hd-0671_full.mp4", "heyzo-0671.mp4"},
	{"4", "heyzo_hd-0671_full.MP4", "heyzo-0671.mp4"},
	{"5", "heyzo_hd-0671_full.avi", "heyzo-0671.avi"},
	{"6", "heyzo_hd-0671_full.AVI", "heyzo-0671.avi"},
	{"7", "/a/b/c/heyzo_hd-0671_full.AVI", "/a/b/c/heyzo-0671.avi"},
	{"8", "hhd800.com@277DCV-219.mp4", "dcv-219.mp4"},
	{"9", "hhd800.com@277DCV-219.MP4", "dcv-219.mp4"},
	{"10", "hhd800.com@FC2-PPV-3129809.mp4", "fc2-ppv-3129809.mp4"},
	{"11", "hhd800.com@FC2-PPV-3192969_1.mp4", "fc2-ppv-3192969-1.mp4"},
	{"12", "hhd800.com@FC2-PPV-3192969_2.mp4", "fc2-ppv-3192969-2.mp4"},
	{"13", "hhd800.com@FSDSS-534.mp4", "fsdss-534.mp4"},
	{"14", "hhd800.com@IPX-950-C_X1080X.mp4", "ipx-950.mp4"},
	{"15", "hhd800.com@PPT-137.mp4", "ppt-137.mp4"},
	{"16", "hhd800.com@STARS-209_UNCENSORED_LEAKED.mp4", "stars-209.mp4"},
	{"17", "SSIS-448-C.mp4", "ssis-448.mp4"},
	{"18", "[98t.tv]dass-015.mp4", "dass-015.mp4"},
	{"19", "Woxav.Com@MIAA-293 姉の挑発を真に受けた童貞弟がイッてる 深田えいみ Uncensored 破解版.mp4", "miaa-293.mp4"},
	{"20", "hhd800.com@080422-003-CARIB.mp4", "caribbean-080422-003.mp4"},
	{"21", "070412-065-carib-whole_hd1.1024x768 2.mp4", "caribbean-070412-065.mp4"},
	{"22", "031913_552-1pon-whole1_hd 2.mp4", "1pondo-031913-552.mp4"},
	{"23", "(Caribbean)(102018-777)これが私の望むセックス 碧しの.mp4", "caribbean-102018-777.mp4"},
	{"24", "(FC2)(843225)女子アナ候補生パイパン美女 種付けまんこに連続生ハメ精子押し込み受精確実→潮吹きながら痙攣アクメ悶絶イキ.mp4", "fc2-843225.mp4"},
	{"25", "(Heyzo)(1723)弄ばれる童顔女教師 千野くるみ.mp4", "heyzo-1723.mp4"},
	{"26", "(Tokyo-Hot)(n1307)東熱激情 欲情綺麗なお姉さん特集 part2.mp4", "tokyo-hot-n1307.mp4"},
	{"27", "Caribbeancom 050520-001  050520-001 3 .mp4", "caribbean-050520-001.mp4"},

	{"521", "电锯人04.mp4", "电锯人04.mp4"},
	{"522", "Manufactured.Landscapes.2006.1080p.BluRay.x264-HANDJOB.mkv",
		"Manufactured.Landscapes.2006.1080p.BluRay.x264-HANDJOB.mkv"},
}

func Test_convertAvFilename(t *testing.T) {
	for _, c := range renameAvTestCases {
		got := convertAvFilename(c.source)
		require.Equal(t, c.target, got, "[%s]source is %q, expected is %q, but got %q",
			c.id, c.source, c.target, got)
	}
}
