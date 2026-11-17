package normalizer_test

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// 回归：GBK 报文必须在解析前转成 UTF-8，否则中文字段是乱码。
func TestGBKPayloadIsDecodedBeforeParsing(t *testing.T) {
	engine, err := normalizer.Compile(normalizer.Mapping{
		ID:        "gbk-kv",
		Name:      "gbk-kv",
		Parse:     parser.Spec{Mode: parser.ModeKV},
		FieldMap:  map[string]string{"msg": "message"},
		Encoding:  "gbk",
		Constants: map[string]any{"class_uid": 2004},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 用 GBK 编码一条含中文的报文，模拟国产设备。
	gkb := []byte("msg=暴力破解登录尝试")
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes(gkb)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply(encoded, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("解码后解析失败: %v", err)
	}

	message, _ := event.Fields["message"].(string)
	if message != "暴力破解登录尝试" {
		t.Fatalf("中文乱码: %q", message)
	}
}

// 未配置字符集时按 UTF-8 原样处理，不能引入任何额外转换。
func TestUTF8PayloadPassesThrough(t *testing.T) {
	engine, err := normalizer.Compile(normalizer.Mapping{
		ID:        "utf8-kv",
		Name:      "utf8-kv",
		Parse:     parser.Spec{Mode: parser.ModeKV},
		FieldMap:  map[string]string{"msg": "message"},
		Constants: map[string]any{"class_uid": 2004},
	})
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte("msg=暴力破解"), time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if event.Fields["message"] != "暴力破解" {
		t.Fatalf("UTF-8 被破坏: %q", event.Fields["message"])
	}
}

// 未知字符集必须在加载期拒绝，不能等运行期每条报文才失败。
func TestUnknownEncodingIsRejectedAtCompile(t *testing.T) {
	_, err := normalizer.Compile(normalizer.Mapping{
		ID:        "bad-enc",
		Name:      "bad-enc",
		Parse:     parser.Spec{Mode: parser.ModeKV},
		FieldMap:  map[string]string{"msg": "message"},
		Encoding:  "koi8-r",
		Constants: map[string]any{"class_uid": 2004},
	})
	if !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("错误 = %v，期望 ErrInvalidRule", err)
	}
}
