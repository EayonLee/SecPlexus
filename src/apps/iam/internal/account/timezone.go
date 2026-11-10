package account

import "time"

// ValidTimeZone 判断时区表达是否可被接受。
// 接受语义值 utc、browser，空串表示未设置，以及可加载的 IANA 时区名。
func ValidTimeZone(value string) bool {
	if value == "" || value == "utc" || value == "browser" {
		return true
	}

	_, err := time.LoadLocation(value)
	return err == nil
}
