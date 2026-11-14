package syslogchannel

import (
	"bytes"
	"errors"
	"strings"

	"github.com/leodido/go-syslog/v4"
	"github.com/leodido/go-syslog/v4/rfc3164"
	"github.com/leodido/go-syslog/v4/rfc5424"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// RFC5424 与 RFC3164 的解析机无状态，包级保留一份即可。
var (
	rfc5424Machine = rfc5424.NewMachine()
	rfc3164Machine = rfc3164.NewMachine()

	// CEF 解析器无参数，同样复用。
	cefParser, _ = parser.New(parser.Spec{Mode: parser.ModeCEF})
)

// errNotSyslog 表示报文不是 syslog 信封；调用方据此走原文透传。
var errNotSyslog = errors.New("not a syslog message")

// envelope 是两种 syslog 语法共有的字段集合。
type envelope struct {
	base       syslog.Base
	structured *map[string]map[string]string // RFC5424 专有；RFC3164 为 nil。
	version    string
}

// flatten 把一条 syslog 报文摊平成字段表，供规则映射使用。
//
// 先按 RFC5424 解析、失败再按 RFC3164：两者前缀形态不同（5424 带版本号），
// 按顺序试是唯一不引入额外配置的判定方式。
// 若载荷以 CEF: 开头，再把 CEF 字段一并摊平——很多防火墙就是这样发 CEF 的。
func flatten(frame []byte) (map[string]any, error) {
	parsed, err := parseEnvelope(frame)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]any, 16)
	fields["syslog.version"] = parsed.version

	putInt(fields, "syslog.facility", parsed.base.Facility)
	putInt(fields, "syslog.severity", parsed.base.Severity)
	putInt(fields, "syslog.priority", parsed.base.Priority)
	putString(fields, "syslog.hostname", parsed.base.Hostname)
	putString(fields, "syslog.appname", parsed.base.Appname)
	putString(fields, "syslog.procid", parsed.base.ProcID)
	putString(fields, "syslog.msgid", parsed.base.MsgID)
	putString(fields, "message", parsed.base.Message)

	if parsed.base.Timestamp != nil {
		fields["syslog.timestamp"] = parsed.base.Timestamp.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	if parsed.structured != nil {
		for id, params := range *parsed.structured {
			for name, value := range params {
				fields["syslog.sd."+id+"."+name] = value
			}
		}
	}

	// CEF 载荷：把行业字段名一并摊平，规则可直接映射 src/dst。
	if raw := messageText(parsed.base); strings.Contains(raw, "CEF:") {
		cefFields, cefErr := cefParser.Parse([]byte(raw))
		if cefErr == nil {
			for key, value := range cefFields {
				fields["cef."+key] = value
			}
		}
	}

	return fields, nil
}

// parseEnvelope 依次尝试两种 syslog 语法。
// 两种消息都内嵌 syslog.Base，而 syslog.Message 接口没有 Base 方法，
// 因此必须按具体类型断言取出通用字段。
func parseEnvelope(frame []byte) (envelope, error) {
	trimmed := bytes.TrimSpace(frame)

	if message, err := rfc5424Machine.Parse(trimmed); err == nil {
		if typed, ok := message.(*rfc5424.SyslogMessage); ok {
			return envelope{base: typed.Base, structured: typed.StructuredData, version: "rfc5424"}, nil
		}
	}

	if message, err := rfc3164Machine.Parse(trimmed); err == nil {
		if typed, ok := message.(*rfc3164.SyslogMessage); ok {
			return envelope{base: typed.Base, version: "rfc3164"}, nil
		}
	}

	return envelope{}, errNotSyslog
}

// messageText 取原始 message 文本，用于 CEF 前缀判定。
func messageText(base syslog.Base) string {
	if base.Message == nil {
		return ""
	}

	return *base.Message
}

// putString 只在指针非空时写字段：缺失就是缺失，不写哨兵值。
func putString(fields map[string]any, key string, value *string) {
	if value != nil && *value != "" {
		fields[key] = *value
	}
}

// putInt 写入无符号整数指针的值。
func putInt(fields map[string]any, key string, value *uint8) {
	if value != nil {
		fields[key] = int(*value)
	}
}
