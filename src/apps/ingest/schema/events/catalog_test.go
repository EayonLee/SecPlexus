package events_test

import (
	"slices"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
)

func TestNetworkActivityInheritsBaseFieldsAndClassEnums(t *testing.T) {
	catalog, err := events.Load()
	if err != nil {
		t.Fatal(err)
	}
	class, ok := catalog.Class(4001)
	if !ok || class.Name.ZhCN != "网络活动" || catalog.OCSFVersion != "1.10.0" {
		t.Fatalf("网络活动目录版本错误: %+v", class)
	}
	if class.Category.ZhCN != "网络类" || class.Category.En == class.Name.En {
		t.Fatalf("network category and event names must be distinct: %+v", class.Category)
	}
	fields := make(map[string]events.Field)
	for _, field := range class.Fields {
		if field.Name.ZhCN == "" || field.Description.ZhCN == "" {
			t.Fatalf("字段缺少中文定义: %s", field.Path)
		}
		fields[field.Path] = field
	}
	if severity := fields["severity_id"]; severity.Requirement != "required" ||
		!slices.EqualFunc(severity.Enum, []int{0, 1, 2, 3, 4, 5, 6, 99}, func(value events.EnumValue, code int) bool {
			return value.Value == code && value.Name.ZhCN != ""
		}) {
		t.Fatalf("严重度定义错误: %+v", severity)
	}
	if activity := fields["activity_id"]; len(activity.Enum) != 9 || activity.Enum[6].Name.En != "Traffic" {
		t.Fatalf("活动枚举应合并基类的 Unknown/Other 和网络活动编码: %+v", activity)
	}
	if ip := fields["src_endpoint.ip"]; ip.LogicalType != "ip_t" || ip.JSONType != "string" || ip.Owner != "mapping" {
		t.Fatalf("源地址类型不正确: %+v", ip)
	}
	if labels := fields["metadata.labels"]; !labels.Array || labels.JSONType != "string" {
		t.Fatalf("标签应为字符串数组: %+v", labels)
	}
	if time := fields["time"]; time.Requirement != "required" || time.Unit != "ms" {
		t.Fatalf("事件时间定义错误: %+v", time)
	}
	for _, uid := range []int{0, 1001, 1007, 2002, 2004, 3001, 3002, 4001, 4002, 4003, 6003} {
		if _, ok = catalog.Class(uid); !ok {
			t.Errorf("v1 missing class %d", uid)
		}
	}
	if catalog.ContractVersion != "v1" {
		t.Fatalf("platform version = %s", catalog.ContractVersion)
	}
	var destinationCount int
	for _, field := range catalog.Fields() {
		if field.Path == "dst_endpoint.ip" {
			destinationCount++
			if len(field.Usages) < 2 {
				t.Errorf("destination IP lost supported classes: %+v", field)
			}
		}
	}
	if destinationCount != 1 {
		t.Fatalf("destination entries = %d", destinationCount)
	}
}

func TestUserManagementKeepsAccountChangeActivityCodes(t *testing.T) {
	catalog, err := events.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		class   int
		caption string
	}{
		{3001, "Enable"},
		{3007, "Update"},
	} {
		eventClass, ok := catalog.Class(item.class)
		if !ok {
			t.Fatalf("missing class %d", item.class)
		}
		index := slices.IndexFunc(eventClass.Fields, func(field events.Field) bool { return field.Path == "activity_id" })
		if index < 0 || !slices.ContainsFunc(eventClass.Fields[index].Enum, func(value events.EnumValue) bool {
			return value.Value == 2 && value.Name.En == item.caption
		}) {
			t.Fatalf("wrong activity 2 in class %d", item.class)
		}
	}
}

func TestPublishedCategoryMatchesCategoryUIDDefinition(t *testing.T) {
	catalog, err := events.Load()
	if err != nil {
		t.Fatal(err)
	}
	categories := make(map[int]events.Text)
	for _, summary := range catalog.Classes() {
		class, _ := catalog.Class(summary.UID)
		if name, exists := categories[class.CategoryUID]; exists && name != class.Category {
			t.Fatalf("category %d has different names in published classes", class.CategoryUID)
		}
		categories[class.CategoryUID] = class.Category
		index := slices.IndexFunc(class.Fields, func(field events.Field) bool { return field.Path == "category_uid" })
		if index < 0 || !slices.ContainsFunc(class.Fields[index].Enum, func(value events.EnumValue) bool {
			return value.Value == class.CategoryUID && value.Name == class.Category
		}) {
			t.Errorf("class %d has a category label different from its category_uid enum", summary.UID)
		}
	}
}
