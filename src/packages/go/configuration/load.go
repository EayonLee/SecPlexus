// Package configuration 提供模块复用的 Viper 加载及严格类型解码。
package configuration

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// Read 以原生 YAML 解码器读取唯一文档，再交由 Viper 管理配置来源。
// 先解析 YAML，再用成熟组件逐值展开环境参数；不执行命令，错误不包含配置值。
func Read(filename string) (*viper.Viper, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.New("cannot open config.yaml; provide an existing --config file")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || info.Size() > 64*1024 {
		return nil, errors.New("configuration exceeds 64 KiB or cannot be read")
	}
	decoder := yaml.NewDecoder(io.LimitReader(file, 64*1024+1))
	var values map[string]any
	if err := decoder.Decode(&values); err != nil {
		return nil, errors.New("invalid YAML configuration")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("configuration must contain one YAML document")
	}
	if _, err := expandValues(values, "", &expand.Config{Env: expand.ListEnviron(os.Environ()...)}); err != nil {
		return nil, err
	}
	v := viper.New()
	v.AllowEmptyEnv(true)
	if err := v.MergeConfigMap(values); err != nil {
		return nil, errors.New("invalid configuration structure")
	}
	return v, nil
}

// expandValues 只展开已解析的标量，避免密码中的换行或引号改变 YAML 结构。
func expandValues(value any, field string, environment *expand.Config) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		for key, entry := range value {
			expanded, err := expandValues(entry, strings.TrimPrefix(field+"."+key, "."), environment)
			if err != nil {
				return nil, err
			}
			value[key] = expanded
		}
	case []any:
		for index, entry := range value {
			expanded, err := expandValues(entry, fmt.Sprintf("%s[%d]", field, index), environment)
			if err != nil {
				return nil, err
			}
			value[index] = expanded
		}
	case string:
		if !strings.Contains(value, "$") {
			return value, nil
		}
		word, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Document(strings.NewReader(value))
		if err != nil {
			return nil, fmt.Errorf("%s: invalid configuration variable expression; use parameter expansion only", field)
		}
		expanded, err := expand.Document(environment, word)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid configuration variable expression; use parameter expansion only", field)
		}
		return expanded, nil
	}
	return value, nil
}

// Decode 使用组件的标准解码 Hook 转换环境标量，并拒绝未知字段。
func Decode(v *viper.Viper, target any) error {
	var metadata mapstructure.Metadata
	err := v.Unmarshal(target, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		mapstructure.StringToBasicTypeHookFunc(),
	)), func(c *mapstructure.DecoderConfig) {
		c.TagName = "yaml"
		c.WeaklyTypedInput = false
		c.ErrorUnused = false
		c.Metadata = &metadata
	})
	if err != nil {
		var field *mapstructure.DecodeError
		if errors.As(err, &field) {
			// 只使用组件的字段元数据，不拼接可能包含密码原值的原始错误。
			name := field.Name()
			if name == "" {
				name = "configuration"
			}
			var parse *mapstructure.ParseError
			var conversion *mapstructure.UnconvertibleTypeError
			if errors.As(err, &parse) {
				return fmt.Errorf("%s: invalid type; expected %s", name, parse.Expected.Type())
			}
			if errors.As(err, &conversion) {
				return fmt.Errorf("%s: invalid type; expected %s", name, conversion.Expected.Type())
			}
			return fmt.Errorf("%s: invalid type; check the YAML field declaration", name)
		}
		return errors.New("invalid configuration type; check the YAML field declarations")
	}
	if len(metadata.Unused) > 0 {
		sort.Strings(metadata.Unused)
		return errors.New("unknown configuration fields: " + strings.Join(metadata.Unused, ", "))
	}
	return nil
}

// Validate 使用成熟组件校验配置结构的必需项、范围及列表；不输出配置值。
func Validate(target any) error {
	engine := validator.New(validator.WithRequiredStructEnabled())
	engine.RegisterTagNameFunc(func(field reflect.StructField) string { return strings.SplitN(field.Tag.Get("yaml"), ",", 2)[0] })
	if err := engine.Struct(target); err != nil {
		var invalid validator.ValidationErrors
		if errors.As(err, &invalid) {
			fields := make([]string, 0, len(invalid))
			for _, field := range invalid {
				name := field.Namespace()
				root := reflect.TypeOf(target)
				if root.Kind() == reflect.Pointer {
					root = root.Elem()
				}
				if root.Name() != "" {
					name = strings.TrimPrefix(name, root.Name()+".")
				}
				name = strings.TrimPrefix(name, ".")
				constraint := field.Tag()
				if field.Param() != "" {
					constraint += "=" + field.Param()
				}
				fields = append(fields, name+" ("+constraint+")")
			}
			return errors.New("invalid configuration fields: " + strings.Join(fields, ", "))
		}
		return errors.New("invalid configuration structure")
	}
	return nil
}
