package model

// Page 是 Ingest 管理列表的分页响应，偏移量反映实际返回的页。
type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

// ListQuery 限制管理列表的翻页深度和搜索字符串。
type ListQuery struct {
	Limit  int32  `form:"limit,default=20" binding:"min=1,max=100"`
	Offset int32  `form:"offset,default=0" binding:"min=0,max=100000"`
	Q      string `form:"q" binding:"max=255"`
}

// InputListQuery 绑定接入点列表筛选。
type InputListQuery struct {
	ListQuery
	Protocol      string  `form:"protocol" binding:"omitempty,oneof=http syslog kafka"`
	Status        string  `form:"status" binding:"omitempty,oneof=enabled disabled"`
	ParsingRuleID *string `form:"rule" binding:"omitempty,uuid"`
	ExactName     string  `form:"name" binding:"max=100"`
	SyslogPort    *int32  `form:"syslog_port" binding:"omitempty,min=1024,max=65535"`
}

// MappingListQuery 绑定规则列表筛选，IDs 用于批量取得绑定规则名称。
type MappingListQuery struct {
	ListQuery
	ClassUID  *int32   `form:"class_uid" binding:"omitempty,gte=0"`
	Origin    string   `form:"origin" binding:"omitempty,oneof=builtin custom"`
	ParseType string   `form:"parse_type" binding:"omitempty,oneof=json cef kv regex delimiter"`
	IDs       []string `form:"id" binding:"max=100,dive,uuid"`
}

// MappingInputCountsQuery 限定统计数据接入数量的解析规则 ID。
type MappingInputCountsQuery struct {
	IDs []string `form:"id" binding:"min=1,max=100,dive,uuid"`
}
