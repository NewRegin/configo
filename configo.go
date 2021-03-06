package configo

import (
	"github.com/asaskevich/govalidator"
	"github.com/shafreeck/toml"
	"github.com/shafreeck/toml/ast"

	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

const (
	fieldTagName = "cfg"
)

func init() {
	toml.SetValue = fieldValidate
	govalidator.TagMap["netaddr"] = func(addr string) bool {
		if h, p, err := net.SplitHostPort(addr); err == nil {
			if h != "" && !(govalidator.IsDNSName(h) || govalidator.IsIP(h)) {
				return false
			}
			if p != "" && !govalidator.IsPort(p) {
				return true
			}
			return true
		}
		return false
	}
}

func fieldValidate(field string, rv reflect.Value, av ast.Value, tag *toml.CfgTag) error {
	if tag == nil {
		return nil
	}
	val, ok := av.(*ast.String)
	if tag.Check != "" && ok {
		rules := strings.Split(tag.Check, " ")
		return validate(field, val.Value, rules)
	}
	return nil
}
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String, reflect.Array:
		return v.Len() == 0
	case reflect.Map, reflect.Slice:
		return v.Len() == 0 || v.IsNil()
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return reflect.DeepEqual(v.Interface(), reflect.Zero(v.Type()).Interface())
}

func extractTag(tag string) *toml.CfgTag {
	tags := strings.SplitN(tag, ",", 4)
	cfg := &toml.CfgTag{}
	switch c := len(tags); c {
	case 1:
		cfg.Name = strings.TrimSpace(tags[0])
	case 2:
		cfg.Name = strings.TrimSpace(tags[0])
		cfg.Value = strings.TrimSpace(tags[1])
	case 3:
		cfg.Name = strings.TrimSpace(tags[0])
		cfg.Value = strings.TrimSpace(tags[1])
		cfg.Check = strings.TrimSpace(tags[2])
	case 4:
		cfg.Name = strings.TrimSpace(tags[0])
		cfg.Value = strings.TrimSpace(tags[1])
		cfg.Check = strings.TrimSpace(tags[2])
		cfg.Description = strings.TrimSpace(tags[3])
	default:
		return nil
	}
	return cfg
}

func validate(key, value string, rules []string) error {
	for _, rule := range rules {
		validate, ok := govalidator.TagMap[rule]
		if !ok {
			return fmt.Errorf("validate rule %q is not supported", rule)
			continue
		}
		if !validate(value) {
			return fmt.Errorf("value of %q validate failed, %q does not match rule %q", key, value, rule)
		}
	}
	return nil
}

//parse a toml array
func unmarshalArray(key, value string, v interface{}) error {
	//construct a valid toml array
	data := key + " = " + value
	if err := toml.Unmarshal([]byte(data), v); err != nil {
		return err
	}
	return nil
}

func applyDefaultValue(fv reflect.Value, ft reflect.StructField, rv reflect.Value) error {
	tag := extractTag(ft.Tag.Get(fieldTagName))

	//Default value is not supported
	if tag.Value == "required" {
		return fmt.Errorf("value of %q is required", ft.Name)
	}

	//No default value supplied
	if tag.Value == "" {
		return nil
	}

	//Validate the default value
	//reflect.Slice will be validated by unmarshalArray
	if tag.Check != "" && fv.Kind() != reflect.Slice {
		rules := strings.Split(tag.Check, " ")
		if err := validate(ft.Name, tag.Value, rules); err != nil {
			return err
		}
	}

	//Set the default value
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64:
		if v, err := strconv.ParseInt(tag.Value, 10, 64); err != nil {
			return err
		} else {
			fv.SetInt(v)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64:
		if v, err := strconv.ParseUint(tag.Value, 10, 64); err != nil {
			return err
		} else {
			fv.SetUint(v)
		}
	case reflect.Float32, reflect.Float64:
		if v, err := strconv.ParseFloat(tag.Value, 64); err != nil {
			return err
		} else {
			fv.SetFloat(v)
		}
	case reflect.Bool:
		if v, err := strconv.ParseBool(tag.Value); err != nil {
			return err
		} else {
			fv.SetBool(v)
		}
	case reflect.String:
		fv.SetString(tag.Value)
	case reflect.Slice:
		v := rv.Addr().Interface()
		if err := unmarshalArray(ft.Name, tag.Value, v); err != nil {
			return err
		}
	default:
		return fmt.Errorf("set default value of type %s is not supported yet", ft.Type)
	}
	return nil
}

//Notice toCamelCase is copied from github.com/naoina/toml
// toCamelCase returns a copy of the string s with all Unicode letters mapped to their camel case.
// It will convert to upper case previous letter of '_' and first letter, and remove letter of '_'.
func toUnderscore(s string) string {
	if s == "" {
		return ""
	}
	result := make([]rune, 0, len(s))

	result = append(result, unicode.ToLower(rune(s[0])))
	for _, r := range s[1:] {
		if unicode.ToUpper(r) == r {
			result = append(result, '_', unicode.ToLower(r))
			continue
		}
		result = append(result, r)
	}
	return string(result)
}

func hasField(t *ast.Table, field reflect.StructField) bool {
	tag := extractTag(field.Tag.Get(fieldTagName))
	if tag != nil && tag.Name != "" {
		if _, found := t.Fields[tag.Name]; found {
			return found
		}
		return false
	}

	name := field.Name
	for _, n := range []string{name, strings.ToLower(name), toUnderscore(name)} {
		if _, found := t.Fields[n]; found {
			return found
		}
	}
	return false
}

func ApplyDefault(t *ast.Table, rv reflect.Value) error {
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	rt := rv.Type()

	if kind := rt.Kind(); kind == reflect.Struct {
		for i := 0; i < rt.NumField(); i++ {
			ft := rt.Field(i)
			fv := rv.Field(i)
			for fv.Kind() == reflect.Ptr {
				fv = fv.Elem()
			}
			if fv.Kind() == reflect.Struct {
				var subt *ast.Table
				if f := t.Fields[ft.Name]; f != nil {
					subt = f.(*ast.Table)
				}
				if err := ApplyDefault(subt, fv); err != nil {
					return err
				}
				continue
			}
			if isEmptyValue(fv) {
				if t == nil || !hasField(t, ft) {
					if err := applyDefaultValue(fv, ft, rv); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func Unmarshal(data []byte, v interface{}) error {
	table, err := toml.Parse(data)
	if err != nil {
		return err
	}

	if err := toml.UnmarshalTable(table, v); err != nil {
		return err
	}

	if err := ApplyDefault(table, reflect.ValueOf(v)); err != nil {
		return err
	}
	return nil
}
