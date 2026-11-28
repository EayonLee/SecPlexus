package service

import "encoding/json"

// SaveRuleInput 是保存解析规则及提交验证样本的用例参数。
type SaveRuleInput struct {
	ID              string
	Name            string
	Vendor          string
	Product         string
	Definition      json.RawMessage
	Samples         []string
	ExpectedVersion int32
	AuthorID        *string
}
