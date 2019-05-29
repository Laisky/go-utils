//+build gofuzz

package journal

func FuzzUnmarshalData(d []byte) int {
	v := Data{}
	if _, err := v.UnmarshalMsg(d); err != nil {
		return 1
	}
	if _, err := v.MarshalMsg(d); err != nil {
		return 1
	}
	return 0
}
