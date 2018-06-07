package utils

import "time"

// UTCNow 获取当前 UTC 时间
func UTCNow() time.Time {
	return time.Now().UTC()
}

func ParseTs2String(ts int64, layout string) string {
	return time.Unix(ts, 0).UTC().Format(layout)
}
