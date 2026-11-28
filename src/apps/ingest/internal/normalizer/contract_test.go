package normalizer_test

import (
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

func TestCompileDefinitionRequiresExplicitContractVersion(t *testing.T) {
	definition := []byte(`{"id":"network","parse":{"mode":"json"},"field_map":{"source":"src_endpoint.ip"},"constants":{"class_uid":4001}}`)
	if _, err := normalizer.CompileDefinition(definition); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("未绑定版本的规则应被拒绝: %v", err)
	}
}

func TestVersionedRuleRejectsOutOfRangeClassID(t *testing.T) {
	rule := normalizer.Mapping{
		ID: "network", ContractVersion: "v1", Parse: parser.Spec{Mode: parser.ModeJSON},
		Constants: map[string]any{},
		FieldMap:  map[string]string{"address": "src_endpoint.ip"},
	}
	for _, uid := range []int64{-1, math.MaxInt32 + 1} {
		rule.Constants["class_uid"] = strconv.FormatInt(uid, 10)
		if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
			t.Fatalf("class UID %d should be rejected, got %v", uid, err)
		}
	}
}

func TestVersionedRuleUsesEventContract(t *testing.T) {
	rule := normalizer.Mapping{
		ID: "network", ContractVersion: "v1", Parse: parser.Spec{Mode: parser.ModeJSON},
		Constants: map[string]any{"class_uid": 4001, "activity_id": 6},
		FieldMap:  map[string]string{"address": "src_endpoint.ip", "port": "src_endpoint.port"},
	}
	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Apply([]byte(`{"address":"192.0.2.1","port":"443"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if port := result.Fields["src_endpoint"].(map[string]any)["port"]; port != int64(443) {
		t.Fatalf("端口未归一化为整数: %v", port)
	}
	for _, sample := range []string{
		`{"address":"invalid-ip","port":"443"}`, `{"port":"65536"}`, `{"port":"443"}`,
	} {
		if result, err := engine.Apply([]byte(sample), time.Now()); err != nil || result.CandidateClass != 4001 || len(result.Issues) == 0 {
			t.Fatalf("样本应报告字段契约错误: %s, %v", sample, err)
		}
	}
	rule.FieldMap["address"] = "src_endpoint.address"
	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("应拒绝未知目标字段: %v", err)
	}
	delete(rule.FieldMap, "address")
	rule.Constants["activity_id"] = 15
	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("应拒绝当前事件类未定义的活动编码: %v", err)
	}
}

func TestSeverityIsConfiguredThroughConversion(t *testing.T) {
	rule := normalizer.Mapping{
		ID: "network", ContractVersion: "v1", Parse: parser.Spec{Mode: parser.ModeJSON},
		Constants: map[string]any{"class_uid": 4001},
		FieldMap:  map[string]string{"address": "src_endpoint.ip", "priority": "severity"},
	}
	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("severity label may only be supplied by conversion, got %v", err)
	}
}
