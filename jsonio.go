package ssastyle

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseJSONDocument 解析样式的 JSON 表示。
//
// 支持三种外层形态：
//  1. {"dialect":"ssa","styles":[ StyleSpec, ... ]}
//  2. {"styles":{"Name": StyleSpec, ...}}
//  3. {"Name": StyleSpec, ...}（转换结果常见的扁平映射）
//
// StyleSpec 中除 name/parent/fields/source 外的键都视为样式字段；
// fields 还可以是 [["key", value], ...] 以保留重复字段的每次出现。
func ParseJSONDocument(data []byte, source string) (*Document, error) {
	pairs, err := objectPairs(data)
	if err != nil {
		return nil, fmt.Errorf("%s: json 解析失败: %w", source, err)
	}
	doc := NewDocument(DialectJSON)
	doc.Source = source

	for _, p := range pairs {
		key := p.Key
		switch CanonicalKey(key) {
		case "dialect":
			_ = json.Unmarshal(p.Val, &doc.Dialect)
			continue
		case "source":
			_ = json.Unmarshal(p.Val, &doc.Source)
			continue
		case "styles":
			if err := appendStylesValue(doc, p.Val, source); err != nil {
				return nil, err
			}
			continue
		}
		if isReservedTopKey(key) || strings.HasPrefix(key, "_") {
			continue
		}
		// 形态 3：顶层即 名称 -> StyleSpec；重复样式名也会保留为两条记录。
		st, err := parseNamedSpec(p.Val, key, source)
		if err != nil {
			return nil, err
		}
		doc.Styles = append(doc.Styles, st)
	}
	return doc, nil
}

// appendStylesValue 处理 "styles": [ ... ] 或 { 名称: StyleSpec }。
func appendStylesValue(doc *Document, raw json.RawMessage, source string) error {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		for i, item := range arr {
			st, err := parseStyleSpec(item, source, i)
			if err != nil {
				return err
			}
			doc.Styles = append(doc.Styles, st)
		}
		return nil
	}
	pairs, err := objectPairs(raw)
	if err != nil {
		return fmt.Errorf("%s: styles 必须是数组或对象: %w", source, err)
	}
	for _, p := range pairs {
		if strings.HasPrefix(p.Key, "_") {
			continue
		}
		st, err := parseNamedSpec(p.Val, p.Key, source)
		if err != nil {
			return err
		}
		doc.Styles = append(doc.Styles, st)
	}
	return nil
}

func isReservedTopKey(k string) bool {
	switch CanonicalKey(k) {
	case "dialect", "source", "styles", "options", "legacy", "converted", "strictunmapped":
		return true
	}
	return false
}

func parseNamedSpec(raw json.RawMessage, name, source string) (*Style, error) {
	st, err := parseStyleSpec(raw, source, 0)
	if err != nil {
		return nil, err
	}
	if st.Name == "" {
		st.Name = name
	}
	return st, nil
}

func parseStyleSpec(raw json.RawMessage, source string, idx int) (*Style, error) {
	pairs, err := objectPairs(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: 第 %d 个样式必须是 JSON 对象: %w", source, idx, err)
	}
	st := &Style{Source: source}
	for _, p := range pairs {
		k, v := p.Key, p.Val
		switch k {
		case "name":
			_ = json.Unmarshal(v, &st.Name)
			continue
		case "source":
			_ = json.Unmarshal(v, &st.Source)
			continue
		case "parent":
			applyParentRaw(st, "parent", v)
			continue
		case "fields":
			if err := appendFieldsRaw(st, v); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(k, "_") {
			continue
		}
		if spec, ok := LookupField(k); ok && spec.Key == "basedon" {
			applyParentRaw(st, k, v)
			continue
		}
		val, err := scalarValue(v)
		if err != nil {
			return nil, fmt.Errorf("样式 %q 字段 %q: %w", st.Name, k, err)
		}
		st.appendField(CanonicalKey(k), k, val, st.Source)
	}
	return st, nil
}

// applyParentRaw 处理 parent/BasedOn 键的重复出现：非空值以后出现者为准，
// null/空值只在尚无父样式时记录为空 parent，避免在解析阶段被空值覆盖。
func applyParentRaw(st *Style, key string, v json.RawMessage) {
	st.ParentSet = true
	if rawIsJSONNull(v) {
		if st.Parent == "" {
			st.ParentNull = true
		}
		return
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return
	}
	if strings.TrimSpace(s) == "" {
		if st.Parent == "" {
			st.ParentNull = true
		}
		return
	}
	st.Parent, st.ParentNull = s, false
}

func appendFieldsRaw(st *Style, raw json.RawMessage) error {
	var pairs [][2]json.RawMessage
	if err := json.Unmarshal(raw, &pairs); err == nil {
		for _, p := range pairs {
			var key string
			if err := json.Unmarshal(p[0], &key); err != nil {
				return fmt.Errorf("fields 键必须是字符串: %w", err)
			}
			v, err := scalarValue(p[1])
			if err != nil {
				return fmt.Errorf("字段 %q: %w", key, err)
			}
			st.appendField(CanonicalKey(key), key, v, st.Source)
		}
		return nil
	}
	if objPairs, err := objectPairs(raw); err == nil {
		for _, p := range objPairs {
			v, err := scalarValue(p.Val)
			if err != nil {
				return fmt.Errorf("字段 %q: %w", p.Key, err)
			}
			st.appendField(CanonicalKey(p.Key), p.Key, v, st.Source)
		}
		return nil
	}
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return fmt.Errorf("fields 必须是对象、[[k,v],...] 或 [{key,value},...]: %w", err)
	}
	for _, item := range arr {
		var key string
		for _, kn := range []string{"key", "name"} {
			if kr, ok := item[kn]; ok {
				_ = json.Unmarshal(kr, &key)
				break
			}
		}
		vr, ok := item["value"]
		if !ok {
			return fmt.Errorf("fields 项 %q 缺少 value", key)
		}
		v, err := scalarValue(vr)
		if err != nil {
			return fmt.Errorf("字段 %q: %w", key, err)
		}
		st.appendField(CanonicalKey(key), key, v, st.Source)
	}
	return nil
}

// scalarValue 保留 null 与零值的区别：null -> ExplicitNull；
// 字符串原样；数字/布尔使用其 JSON 词法文本。
func scalarValue(raw json.RawMessage) (Value, error) {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return Null(), nil
	}
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return Value{}, err
		}
		return Literal(s), nil
	}
	switch t[0] {
	case '{', '[':
		return Value{}, fmt.Errorf("样式值必须是标量，得到 %q", string(t[0]))
	}
	return Literal(t), nil
}
