package normalizer

import (
	"fmt"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// encodings 是支持的来源字符集。键是配置里写的名字，值是解码器。
//
// 只列真实设备会发的字符集：国产设备多为 GBK/GB18030，台系 Big5，
// 日韩设备各自一套，余下按 Windows-1252 兜底。UTF-8 不需要解码器。
var encodings = map[string]encoding.Encoding{
	"gbk":          simplifiedchinese.GBK,
	"gb18030":      simplifiedchinese.GB18030,
	"big5":         traditionalchinese.Big5,
	"shift_jis":    japanese.ShiftJIS,
	"euc-jp":       japanese.EUCJP,
	"euc-kr":       korean.EUCKR,
	"windows-1252": charmap.Windows1252,
	"iso-8859-1":   charmap.ISO8859_1,
}

// resolveEncoding 在构造期解析字符集名称；UTF-8 与空值都表示不解码。
func resolveEncoding(name string) (encoding.Encoding, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "", "utf-8", "utf8":
		return nil, nil
	}

	decoded, ok := encodings[normalized]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported source encoding %q", ErrInvalidRule, name)
	}

	return decoded, nil
}

// decode 把来源字符集转成 UTF-8；未配置字符集时原样返回。
//
// 转码必须发生在解析之前：按错误字符集解析出来的字段是乱码，
// 后续任何映射都救不回来。
func decode(source encoding.Encoding, raw []byte) ([]byte, error) {
	if source == nil {
		return raw, nil
	}

	decoded, err := source.NewDecoder().Bytes(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: could not decode from the configured encoding", ErrDecodeFailed)
	}

	return decoded, nil
}
