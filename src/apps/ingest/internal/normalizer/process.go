package normalizer

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// Processor 描述一个有限的字段处理。JSON 使用点分隔路径提取一个值。
type Processor struct {
	Operation string `json:"operation"`
	Source    string `json:"source"`
	Path      string `json:"path,omitempty"`
	Value     string `json:"value,omitempty"`
	Target    string `json:"target"`
}

// Extraction 配置报文提取及按顺序执行的字段处理。
type Extraction struct {
	Parse      parser.Spec `json:"parse"`
	Encoding   string      `json:"encoding,omitempty"`
	Processors []Processor `json:"processors,omitempty"`
}

// CompileExtraction 编译真实接收和提取预览共用的解析设置。
func CompileExtraction(spec Extraction) (*Compiled, error) {
	if len(spec.Processors) > 8 {
		return nil, fmt.Errorf("%w: too many field processors", ErrInvalidRule)
	}
	targets := make(map[string]bool, len(spec.Processors))
	for _, step := range spec.Processors {
		if step.Source == "" || len(step.Source) > 128 || !branchID.MatchString(step.Target) ||
			step.Source == step.Target || targets[step.Target] || isSensitiveKey(step.Source) || isSensitiveKey(step.Target) {
			return nil, fmt.Errorf("%w: invalid field processor", ErrInvalidRule)
		}
		switch step.Operation {
		case "json":
			if len(step.Path) > 128 || strings.Count(step.Path, ".") > 7 || isSensitiveKey(step.Path) {
				return nil, fmt.Errorf("%w: invalid JSON extraction path", ErrInvalidRule)
			}
			if _, err := validatePath(step.Path); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrInvalidRule, err)
			}
		case "trim_prefix", "trim_suffix":
			if step.Value == "" || len(step.Value) > 256 {
				return nil, fmt.Errorf("%w: invalid trim text", ErrInvalidRule)
			}
		case "trim", "decode_base64", "decode_uri", "decode_hex":
		default:
			return nil, fmt.Errorf("%w: unknown field processor", ErrInvalidRule)
		}
		if (step.Operation != "json" && step.Path != "") ||
			(step.Operation != "trim_prefix" && step.Operation != "trim_suffix" && step.Value != "") {
			return nil, fmt.Errorf("%w: unexpected field processor parameter", ErrInvalidRule)
		}
		targets[step.Target] = true
	}
	for _, step := range spec.Processors {
		delete(targets, step.Target)
		if targets[step.Source] {
			return nil, fmt.Errorf("%w: field processor refers to a later output", ErrInvalidRule)
		}
	}

	engine, err := compile(Mapping{Parse: spec.Parse, Encoding: spec.Encoding})
	if err != nil {
		return nil, err
	}
	engine.processors = spec.Processors
	return engine, nil
}

// ProcessFields 保留原始字段并添加派生字段；单字段失败只影响该项。
func (e *Compiled) ProcessFields(original map[string]any) (map[string]any, []Issue) {
	if len(e.processors) == 0 {
		return original, nil
	}
	fields := maps.Clone(original)
	if fields == nil {
		fields = make(map[string]any)
	}
	var issues []Issue
	for _, step := range e.processors {
		raw, present := fields[step.Source]
		value, isText := raw.(string)
		_, collision := fields[step.Target]
		switch {
		case collision:
			issues = append(issues, Issue{Path: step.Target, Code: "processor_target_exists"})
		case !present:
			continue
		case !isText || len(value) > 16*1024:
			issues = append(issues, Issue{Path: step.Source, Code: "invalid_processor_value"})
		case step.Operation == "trim":
			fields[step.Target] = strings.TrimSpace(value)
		case step.Operation == "json":
			extracted, ok := embeddedScalar(value, step.Path)
			if !ok {
				issues = append(issues, Issue{Path: step.Source, Code: "invalid_embedded_json"})
				continue
			}
			fields[step.Target] = extracted
		default:
			processed, ok := processText(step, value)
			if !ok {
				issues = append(issues, Issue{Path: step.Source, Code: "invalid_processor_value"})
				continue
			}
			fields[step.Target] = processed
		}
	}
	return fields, issues
}

func processText(step Processor, value string) (string, bool) {
	var decoded string
	var err error
	switch step.Operation {
	case "trim_prefix":
		return strings.CutPrefix(value, step.Value)
	case "trim_suffix":
		return strings.CutSuffix(value, step.Value)
	case "decode_uri":
		decoded, err = url.PathUnescape(value)
	case "decode_base64":
		var bytes []byte
		bytes, err = base64.StdEncoding.Strict().DecodeString(value)
		decoded = string(bytes)
	case "decode_hex":
		var bytes []byte
		bytes, err = hex.DecodeString(value)
		decoded = string(bytes)
	}
	return decoded, err == nil && utf8.ValidString(decoded)
}

func embeddedScalar(raw, path string) (any, bool) {
	// 先检查结构深度；字符串里的括号由转义状态排除。
	depth, quoted, escaped := 0, false, false
	for _, character := range raw {
		switch {
		case escaped:
			escaped = false
		case quoted && character == '\\':
			escaped = true
		case character == '"':
			quoted = !quoted
		case !quoted && (character == '{' || character == '['):
			depth++
			if depth > 8 {
				return nil, false
			}
		case !quoted && (character == '}' || character == ']'):
			depth--
		}
	}
	if !json.Valid([]byte(raw)) {
		return nil, false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	for segment := range strings.SplitSeq(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value = object[segment]
	}
	switch value.(type) {
	case string, json.Number, bool:
		return value, true
	default:
		return nil, false
	}
}
