package config

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

func checkFields(node *yaml.Node, target reflect.Type, path string) error {
	switch {
	case node.Tag == "!!null":
		return nil
	case target.Kind() == reflect.Struct && target != reflect.TypeFor[time.Duration]():
		if node.Kind != yaml.MappingNode {
			return invalidValue(node, path)
		}

		seen := map[string]bool{}
		for index := 0; index < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			keyPath := strings.TrimPrefix(path+"."+key.Value, ".")
			field, found := fieldByTag(target, key.Value)
			switch {
			case !found:
				return fmt.Errorf("unknown field %s (line %d)", keyPath, key.Line)
			case seen[key.Value]:
				return fmt.Errorf("duplicate field %s (line %d)", keyPath, key.Line)
			}

			seen[key.Value] = true
			if err := checkFields(value, field.Type, keyPath); err != nil {
				return err
			}
		}
	case target.Kind() == reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return invalidValue(node, path)
		}

		for index, item := range node.Content {
			if err := checkFields(item, target.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case node.Decode(reflect.New(target).Interface()) != nil:
		return invalidValue(node, path)
	}

	return nil
}

func fieldByTag(target reflect.Type, name string) (reflect.StructField, bool) {
	for field := range target.Fields() {
		if field.Tag.Get("yaml") == name {
			return field, true
		}
	}

	return reflect.StructField{}, false
}

func invalidValue(node *yaml.Node, path string) error {
	if path == "" {
		path = "configuration"
	}

	return fmt.Errorf("invalid value for %s (line %d)", path, node.Line)
}
