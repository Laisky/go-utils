package utils

import "time"

// UTCNow 获取当前 UTC 时间
func UTCNow() time.Time {
	return time.Now().UTC()
}

// ParseTs2String can parse unix timestamp(int64) to string
func ParseTs2String(ts int64, layout string) string {
	return ParseTs2Time(ts).Format(layout)
}

// ParseTs2Time can parse unix timestamp(int64) to time.Time
func ParseTs2Time(ts int64) time.Time {
	return time.Unix(ts, 0).UTC()
}
