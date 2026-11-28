package validation

import (
	"strconv"
	"strings"
)

const (
	maxSamples     = 20
	maxSampleBytes = 64 * 1024
	// MaxSampleRequestBytes 容纳规则定义与完整样本。JSON 转义最多将单字节扩大为六字节。
	MaxSampleRequestBytes = (4 << 20) + 6*maxSamples*maxSampleBytes
)

// CheckSamples 校验逐条样本的数量和解码后字节预算。
func CheckSamples(samples []string) error {
	if len(samples) == 0 {
		return &Issue{Field: "samples", Code: "required"}
	}
	if len(samples) > maxSamples {
		return &Issue{Field: "samples", Code: "too_many", Limit: maxSamples}
	}
	for index, sample := range samples {
		if len(sample) > maxSampleBytes {
			return &Issue{Field: Field("samples." + strconv.Itoa(index)), Code: "too_long", Limit: maxSampleBytes}
		}
		if strings.TrimSpace(sample) == "" {
			return &Issue{Field: Field("samples." + strconv.Itoa(index)), Code: "required"}
		}
	}
	return nil
}
