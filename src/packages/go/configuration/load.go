// Package configuration 提供模块复用的 Viper 加载及严格类型解码。
package configuration

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Read 以原生 YAML 解码器读取唯一文档，再交由 Viper 管理配置来源。
// 不展开字符串或执行表达式，错误不包含配置值。
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
	v := viper.New()
	v.AllowEmptyEnv(true)
	if err := v.MergeConfigMap(values); err != nil {
		return nil, errors.New("invalid configuration structure")
	}
	return v, nil
}

// Decode 使用组件的标准解码 Hook 转换环境标量，并拒绝未知字段。
func Decode(v *viper.Viper, target any) error {
	err := v.UnmarshalExact(target, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		mapstructure.StringToBasicTypeHookFunc(),
	)), func(c *mapstructure.DecoderConfig) {
		c.TagName = "yaml"
		c.WeaklyTypedInput = false
	})
	if err != nil {
		return errors.New("invalid configuration type or unknown field")
	}
	return nil
}

// Validate 使用成熟组件校验配置结构的必需项、范围及列表；不输出配置值。
func Validate(target any) error {
	engine := validator.New(validator.WithRequiredStructEnabled())
	if err := engine.Struct(target); err != nil {
		var invalid validator.ValidationErrors
		if errors.As(err, &invalid) {
			fields := make([]string, 0, len(invalid))
			for _, field := range invalid {
				fields = append(fields, field.StructNamespace())
			}
			return errors.New("invalid configuration fields: " + strings.Join(fields, ", "))
		}
		return errors.New("invalid configuration structure")
	}
	return nil
}
