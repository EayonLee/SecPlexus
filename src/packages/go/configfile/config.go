// Package configfile 解析结构化 YAML 中的标量环境占位符。
package configfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Load 仅在标量值中展开 ${NAME:default}，不改变 YAML 结构。
// 显式空环境变量保持为空，占位展开不递归。
func Load(filename string, target any) error {
	file, err := os.Open(filename)
	if err != nil {
		return errors.New("cannot open config.yaml; provide --config with an existing file")
	}

	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return errors.New("configuration exceeds 64 KiB or cannot be read")
	}

	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err = decoder.Decode(&document); err != nil {
		return errors.New("invalid YAML configuration")
	}

	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return errors.New("configuration must contain one YAML document")
	}

	if err = expand(&document); err != nil {
		return err
	}

	data, err = yaml.Marshal(&document)
	if err != nil {
		return errors.New("invalid configuration values")
	}

	decoder = yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	// 解码错误可能包含展开后的凭据，仅返回错误类别。
	if err = decoder.Decode(target); err != nil {
		return errors.New("invalid configuration type, duplicate key or unknown field")
	}

	return nil
}

// expand 解析配置中的环境占位符，不输出配置值。
func expand(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return errors.New("configuration aliases are not supported")
	}

	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if strings.Contains(node.Content[i].Value, "${") {
				return errors.New("configuration keys cannot contain placeholders")
			}

			if err := expand(node.Content[i+1]); err != nil {
				return err
			}
		}

		return nil
	}

	if node.Kind == yaml.ScalarNode && strings.Contains(node.Value, "${") {
		value, err := expandValue(node.Value)
		if err != nil {
			return err
		}

		node.Value = value
		node.Style = 0
		node.Tag = "!!str"
		if _, err := strconv.ParseInt(value, 10, 64); err == nil {
			node.Tag = "!!int"
		} else if value == "true" || value == "false" {
			node.Tag = "!!bool"
		}
	}

	for _, child := range node.Content {
		if err := expand(child); err != nil {
			return err
		}
	}

	return nil
}

// expandValue 读取单项环境覆盖或声明的默认值。
func expandValue(input string) (string, error) {
	var out strings.Builder
	for {
		start := strings.Index(input, "${")
		if start < 0 {
			out.WriteString(input)
			return out.String(), nil
		}

		out.WriteString(input[:start])
		input = input[start+2:]
		end := strings.IndexByte(input, '}')
		if end < 0 {
			return "", errors.New("unclosed environment placeholder")
		}

		name, fallback, hasDefault := strings.Cut(input[:end], ":")
		if !variableName.MatchString(name) || strings.Contains(fallback, "${") {
			return "", errors.New("invalid environment placeholder")
		}

		value, exists := os.LookupEnv(name)
		if !exists {
			if !hasDefault {
				return "", fmt.Errorf("required environment variable %s is missing", name)
			}

			value = fallback
		}

		out.WriteString(value)
		input = input[end+1:]
	}
}
