package ssastyle

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// jsonPair 是 JSON 对象中的一个键值对；与 map 不同，重复键的每次出现都会保留。
type jsonPair struct {
	Key string
	Val json.RawMessage
}

// objectPairs 用流式 Decoder 解析 JSON 对象，保留键的出现顺序与重复键。
// 这是识别“重复字段 + 后出现 null 覆盖”的前提：普通 map 反序列化会
// 在解析阶段就丢掉前一个值。
func objectPairs(data []byte) ([]jsonPair, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return nil, fmt.Errorf("期望 JSON 对象，得到 %v", tok)
	}
	var pairs []jsonPair
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := kt.(string)
		if !ok {
			return nil, fmt.Errorf("JSON 键不是字符串: %v", kt)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("键 %q 的值无法读取: %w", key, err)
		}
		pairs = append(pairs, jsonPair{Key: key, Val: raw})
	}
	if _, err := dec.Token(); err != nil { // 消费 '}'
		return nil, err
	}
	return pairs, nil
}

// rawIsJSONNull 判断原始值是否为 null。
func rawIsJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}
