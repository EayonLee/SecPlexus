package normalizer_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
)

func TestRuleDefinitionRejectsDuplicateSourceIdentity(t *testing.T) {
	for _, field := range []string{"vendor", "product"} {
		definition := fmt.Sprintf(`{"%s":"Example","contract_version":"v1","parse":{"mode":"json"},"branches":[{"id":"network","name":"Network","class_uid":4001}]}`, field)
		if _, err := normalizer.CompileDefinition([]byte(definition)); err == nil {
			t.Errorf("definition.%s 应由独立来源字段填写", field)
		}
	}
}

func TestRuleDefinitionRejectsStoredSamples(t *testing.T) {
	definition := []byte(`{"contract_version":"v1","parse":{"mode":"json"},"branches":[],"samples":["private-log-sample"]}`)
	if _, err := normalizer.CompileDefinition(definition); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("a rule accepted embedded log samples: %v", err)
	}
}

func TestMixedRuleRejectsShadowedBranch(t *testing.T) {
	_, err := normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1","parse":{"mode":"json"},
		"branches":[
			{"id":"all-events","name":"Catch all","class_uid":4001,"match":[]},
			{"id":"login","name":"Login","class_uid":3002,"match":[{"field":"event","equals":"login"}]}
		]
	}`))
	if !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("unreachable branch was accepted: %v", err)
	}
}

func TestMixedRuleRejectsBranchCoveredBySeveralEarlierBranches(t *testing.T) {
	const definition = `{
		"contract_version":"v1","parse":{"mode":"json"},
		"branches":[
			{"id":"accepted","name":"Accepted","class_uid":3002,"match":[{"field":"event","equals":"login"},{"field":"status","equals":"ok"}]},
			{"id":"denied","name":"Denied","class_uid":3002,"match":[{"field":"event","equals":"login"},{"field":"status","equals":"failed"}]},
			{"id":"all-logins","name":"All logins","class_uid":3002,"match":[{"field":"event","equals":"login"},{"field":"status","in":["ok","failed"]}]}
		]
	}`
	if _, err := normalizer.CompileDefinition([]byte(definition)); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("branch covered by multiple earlier branches was accepted: %v", err)
	}
	withNewStatus := strings.Replace(definition, `["ok","failed"]`, `["ok","failed","timeout"]`, 1)
	if _, err := normalizer.CompileDefinition([]byte(withNewStatus)); err != nil {
		t.Fatalf("branch that also matches an uncovered record was rejected: %v", err)
	}
}

func TestRuleRejectsRemovedDefinitionOptions(t *testing.T) {
	_, err := normalizer.CompileDefinition([]byte(`{
		"id":"old","name":"Old","contract_version":"1.0.0","parse":{"mode":"json"},
		"constants":{"class_uid":4001}
	}`))
	if !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Errorf("old definition accepted: %v", err)
	}
	_, err = normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1","parse":{"mode":"json"},
		"multiline":{"start_pattern":"^START","max_lines":4},
		"branches":[{"id":"all","name":"All records","class_uid":4001,"match":[]}]
	}`))
	if !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Errorf("multiline definition accepted: %v", err)
	}
}

func TestV1RuleExtractsEmbeddedFieldsAndKeepsBaseMappings(t *testing.T) {
	engine, err := normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1","parse":{"mode":"json"},
		"processors":[{"operation":"trim","source":"summary","target":"clean_summary"},
			{"operation":"json","source":"payload","path":"client","target":"client_ip"}],
		"shared":{"field_map":{"clean_summary":"message","client_ip":"dst_endpoint.ip"}},
		"branches":[{"id":"network","name":"Network","class_uid":4001,"match":[{"field":"kind","equals":"flow"}]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, sample := range []struct {
		raw string
		uid int
	}{
		{`{"kind":"flow","summary":"  Example  ","payload":"{\"client\":\"192.0.2.20\"}"}`, 4001},
		{`{"summary":"  Example  ","payload":"{\"client\":\"192.0.2.20\"}"}`, 0},
	} {
		result, err := engine.Apply([]byte(sample.raw), time.Unix(0, 0))
		if err != nil || fmt.Sprint(result.Fields["class_uid"]) != fmt.Sprint(sample.uid) || result.Fields["message"] != "Example" {
			t.Fatalf("sample parsed as %#v, %v", result, err)
		}
		if sample.uid == 4001 {
			if dst, ok := result.Fields["dst_endpoint"].(map[string]any); !ok || dst["ip"] != "192.0.2.20" {
				t.Fatalf("missing mapped IP: %#v", result.Fields)
			}
		}
	}
}

func TestV1RuleWithoutClassificationProducesBaseEvent(t *testing.T) {
	engine, err := normalizer.CompileDefinition([]byte(`{"contract_version":"v1","parse":{"mode":"json"},"shared":{"field_map":{"msg":"message"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Apply([]byte(`{"msg":"Example"}`), time.Unix(0, 0))
	if err != nil || result.Fields["message"] != "Example" || len(result.Issues) != 0 {
		t.Fatalf("general rule parsed as %#v, %v", result, err)
	}
}

func TestV1RuleRejectsDuplicateSharedTargetsWithoutClassification(t *testing.T) {
	_, err := normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1",
		"parse":{"mode":"json"},
		"shared":{"field_map":{"src":"src_endpoint.ip","src_addr":"src_endpoint.ip"}}
	}`))
	if !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("duplicate shared target accepted: %v", err)
	}
}

func TestV1BuiltInClassesEnforceRequiredContext(t *testing.T) {
	for _, tc := range []struct {
		class int
		body  string
	}{
		{4002, `{"http_request.url.path":"/health","activity_id":3}`},
		{4003, `{"query.hostname":"example.net","src_endpoint.ip":"192.0.2.10"}`},
		{1007, `{"device.hostname":"example-host","device.type_id":1,"actor.user.name":"svc","process.pid":47}`},
		{1001, `{"device.hostname":"example-host","device.type_id":1,"actor.user.name":"svc","file.name":"report.txt","file.type_id":1}`},
		{3001, `{"user.name":"new-user"}`},
		{6003, `{"api.operation":"ListResources","actor.user.name":"svc","src_endpoint.ip":"192.0.2.10"}`},
		{2004, `{"finding_info.uid":"finding-01"}`},
		{2002, `{"finding_info.uid":"finding-02","vulnerabilities":[{"cve":{"uid":"CVE-2024-1234"},"unused":"never forwarded"}]}`},
	} {
		t.Run(fmt.Sprint(tc.class), func(t *testing.T) {
			var constants map[string]any
			if err := json.Unmarshal([]byte(tc.body), &constants); err != nil {
				t.Fatal(err)
			}
			fieldMap := make(map[string]string, len(constants))
			for path, value := range constants {
				// 活动枚举和对象数组同样走源字段约束。
				if value != nil {
					fieldMap[path] = path
				}
			}
			encoded, err := json.Marshal(map[string]any{
				"contract_version": "v1",
				"parse":            map[string]string{"mode": "json"},
				"branches": []map[string]any{{
					"id": "selected", "name": "Selected", "class_uid": tc.class, "field_map": fieldMap,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			engine, err := normalizer.CompileDefinition(encoded)
			if err != nil {
				t.Fatal(err)
			}
			good, err := engine.Apply([]byte(tc.body), time.Unix(0, 0))
			if err != nil || fmt.Sprint(good.Fields["class_uid"]) != fmt.Sprint(tc.class) || len(good.Issues) != 0 {
				t.Fatalf("valid sample: %+v %v", good, err)
			}
			if strings.Contains(fmt.Sprint(good.Fields), "never forwarded") {
				t.Fatalf("unlisted object attribute was retained: %v", good.Fields)
			}
			failed, err := engine.Apply([]byte(`{"irrelevant":"data"}`), time.Unix(0, 0))
			if err != nil || failed.CandidateClass != tc.class || len(failed.Issues) == 0 || fmt.Sprint(failed.Fields["class_uid"]) != "0" {
				t.Fatalf("incomplete sample: %+v %v", failed, err)
			}
		})
	}
}

func TestRuleDefaultsOnlyForMissingSources(t *testing.T) {
	engine, err := normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1","parse":{"mode":"json"},
		"shared":{"field_map":{"summary":"message","outcome":"status_id"},
			"defaults":{"message":"fallback","status_id":0}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		log     string
		message any
		status  any
	}{
		{`{"extra":1}`, "fallback", int64(0)},
		{`{"summary":"","outcome":1}`, "", int64(1)},
		{`{"summary":null,"outcome":-42}`, nil, nil},
	} {
		result, err := engine.Apply([]byte(item.log), time.Unix(1, 0))
		if err != nil {
			t.Fatal(err)
		}
		if result.Fields["message"] != item.message || result.Fields["status_id"] != item.status {
			t.Errorf("%s: message=%#v status=%#v", item.log, result.Fields["message"], result.Fields["status_id"])
		}
	}
}

func TestRuleRejectsInvalidDefaults(t *testing.T) {
	for _, defaults := range []string{`{"severity_id":1}`, `{"status_id":-1}`, `{"status_id":"1"}`, `{"status_id":null}`} {
		definition := fmt.Sprintf(`{"contract_version":"v1","parse":{"mode":"json"},
			"shared":{"field_map":{"outcome":"status_id"},"defaults":%s}}`, defaults)
		if _, err := normalizer.CompileDefinition([]byte(definition)); !errors.Is(err, normalizer.ErrInvalidRule) {
			t.Errorf("%s accepted: %v", defaults, err)
		}
	}
}

func TestHTTPStatusCodesFollowProtocolRange(t *testing.T) {
	for _, item := range []struct {
		code  int
		valid bool
	}{{42, false}, {100, true}, {204, true}, {499, true}, {599, true}, {600, false}} {
		definition := fmt.Sprintf(`{"contract_version":"v1","parse":{"mode":"json"},
			"branches":[{"id":"http","name":"HTTP","class_uid":4002,
			"field_map":{"upstream":"http_response.code"},"defaults":{"http_response.code":%d}}]}`, item.code)
		if _, err := normalizer.CompileDefinition([]byte(definition)); (err == nil) != item.valid {
			t.Errorf("status %d: %v", item.code, err)
		}
	}
}

func TestRuleDerivesFieldsFromBoundedTextOperations(t *testing.T) {
	engine, err := normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1","parse":{"mode":"json"},
		"processors":[
			{"operation":"trim_prefix","source":"uri","value":"prefix:","target":"without_prefix"},
			{"operation":"trim_suffix","source":"without_prefix","value":".end","target":"without_suffix"},
			{"operation":"decode_uri","source":"without_suffix","target":"decoded_uri"},
			{"operation":"decode_base64","source":"b64","target":"decoded_base64"},
			{"operation":"decode_hex","source":"hex","target":"decoded_hex"},
			{"operation":"decode_hex","source":"binary","target":"decoded_binary"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	got, issues := engine.ProcessFields(map[string]any{"uri": "prefix:a%2Fb+c.end", "b64": "5pel", "hex": "e69cac", "binary": "ff"})
	if got["decoded_uri"] != "a/b+c" || got["decoded_base64"] != "日" || got["decoded_hex"] != "本" ||
		len(issues) != 1 || issues[0].Path != "binary" || got["decoded_binary"] != nil {
		t.Fatalf("fields=%#v issues=%#v", got, issues)
	}
}

func TestRuleAuthoringValidatesCatalogConstraints(t *testing.T) {
	catalog, err := events.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		class int
		path  string
		valid bool
	}{
		{4001, "src_endpoint.ip", true},
		{4001, "src_endpoint.port", false},
		{4001, "message", false},
		{4002, "http_request.url.hostname", false},
		{4002, "http_request.url.path", true},
		{4002, "http_request.url.url_string", true},
	} {
		definition := fmt.Sprintf(`{"contract_version":"v1","catalog_revision":%q,"parse":{"mode":"json"},"description":"Traffic mapping",
			"branches":[{"id":"class","name":"Traffic","class_uid":%d,
				"field_map":{"source":%q}}]}`, catalog.CatalogRevision, item.class, item.path)
		_, _, err := normalizer.PrepareDefinition(json.RawMessage(definition))
		if (err == nil) != item.valid {
			t.Errorf("class=%d path=%s: %v", item.class, item.path, err)
		}
	}
}

func TestProcessExecutableRequiresFileNameAndTypeWhenMapped(t *testing.T) {
	const definition = `{"contract_version":"v1","parse":{"mode":"json"},"description":"Process mapping",
		"branches":[{"id":"process","name":"Process","class_uid":1007,
		"field_map":{"host":"device.hostname","account":"actor.user.name","pid":"process.pid","image":"process.file.path"%s},
		"constants":{"device.type_id":0%s}}]}`
	for _, sample := range []struct {
		mappings  string
		constants string
		valid     bool
	}{
		{"", "", false},
		{`,"name":"process.file.name"`, "", false},
		{`,"name":"process.file.name"`, `,"process.file.type_id":1`, true},
	} {
		_, _, err := normalizer.PrepareDefinition(json.RawMessage(fmt.Sprintf(definition, sample.mappings, sample.constants)))
		if (err == nil) != sample.valid {
			t.Errorf("process executable mapping %q with %q: %v", sample.mappings, sample.constants, err)
		}
	}
}

func TestNetworkConnectionGetsStandardDirection(t *testing.T) {
	engine, err := normalizer.CompileDefinition([]byte(`{"contract_version":"v1","parse":{"mode":"json"},
		"branches":[{"id":"network","name":"Network","class_uid":4001,
		"field_map":{"ip":"src_endpoint.ip","protocol":"connection_info.protocol_num","direction":"connection_info.direction_id"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		input string
		want  int64
	}{
		{`{"ip":"192.0.2.10","protocol":6}`, 0},
		{`{"ip":"192.0.2.10","protocol":6,"direction":1}`, 1},
	} {
		out, err := engine.Apply([]byte(sample.input), time.Unix(1, 0))
		if err != nil {
			t.Fatal(err)
		}
		connection, _ := out.Fields["connection_info"].(map[string]any)
		if connection["direction_id"] != sample.want {
			t.Errorf("connection = %#v, want direction %d", connection, sample.want)
		}
	}
}

func TestDraftRejectsSharedFieldWithDifferentClassMeanings(t *testing.T) {
	definition := json.RawMessage(`{
		"contract_version":"v1","parse":{"mode":"json"},
		"description":"Compare behavior across classes",
		"shared":{"field_map":{"account":"user.name","activity":"activity_id"}},
		"branches":[
			{"id":"older","name":"Account changes","class_uid":3001,"match":[{"field":"source","equals":"older"}]},
			{"id":"current","name":"User management","class_uid":3007,"match":[]}
		]
	}`)
	if _, _, err := normalizer.PrepareDefinition(definition); err == nil {
		t.Fatal("activity code 2 must not be shared by classes that interpret it differently")
	}
	if _, err := normalizer.CompileDefinition(definition); err != nil {
		t.Fatalf("an existing revision should remain readable: %v", err)
	}
}

func TestMappingWriteRequiresDescriptionAndEventClass(t *testing.T) {
	for _, item := range []struct{ definition, field string }{
		{`{"contract_version":"v1","parse":{"mode":"json"}}`, "definition.description"},
		{`{"contract_version":"v1","parse":{"mode":"json"},"description":"Network log"}`, "class_uid"},
	} {
		_, _, err := normalizer.PrepareDefinition(json.RawMessage(item.definition))
		var issue *validation.Issue
		if !errors.As(err, &issue) || string(issue.Field) != item.field || issue.Code != "required" {
			t.Fatalf("field %s: %v", item.field, err)
		}
	}
}

func TestSeverityDefaultIsUsedOnlyWhenSourceIsMissing(t *testing.T) {
	engine, err := normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1","parse":{"mode":"json"},
		"shared":{"severity":{"field":"level","values":{"minor":2},"default":4}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		raw      string
		severity int
		used     bool
	}{
		{`{"tag":"x"}`, 4, true},
		{`{"level":"minor"}`, 2, false},
		{`{"level":"invalid"}`, 0, false},
		{`{"level":null}`, 0, false},
	} {
		result, err := engine.Apply([]byte(item.raw), time.Unix(1, 0))
		if err != nil || result.Fields["severity_id"] != item.severity || (len(result.AppliedDefaults) > 0) != item.used {
			t.Errorf("%s: result=%#v err=%v", item.raw, result, err)
		}
	}
}
