package validation

import "time"

// RuleVerification 保存可追溯的规则试算结果，不包含原文、字段值或事件体。
type RuleVerification struct {
	Digest          string    `json:"definition_digest"`
	VerifiedAt      time.Time `json:"verified_at"`
	SampleCount     int       `json:"sample_count"`
	Passed          int       `json:"passed"`
	CoveredBranches []string  `json:"covered_branches"`
}
