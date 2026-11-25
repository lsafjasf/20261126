package bwf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func ParseProject(path string) (*ProjectMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxMetaBytes {
		return nil, fmt.Errorf("project metadata is too large")
	}
	p, err := parseProject(raw)
	if err != nil {
		return nil, err
	}
	p.Path = path
	return p, nil
}

func parseProject(raw []byte) (*ProjectMeta, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse project JSON: %w", err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("project metadata must be a JSON object")
	}
	p := &ProjectMeta{}
	p.Name = lookupString(obj, "name", "projectname", "title")
	p.Notes = lookupString(obj, "notes", "note", "comment")
	p.ExpectedFileName = lookupString(obj, "expectedfilename", "filename")
	if n, ok := lookupUint(obj, "samplerate", "sampleratehz"); ok {
		p.SampleRate = uint32(n)
	}
	p.TimecodeRate = lookupString(obj, "timecoderate", "framerate", "rate")
	p.StartTimecode = lookupString(obj, "starttimecode", "timecodestart", "timecodeorigin")
	if b, ok := lookupBool(obj, "dropframe"); ok {
		p.DropFrame = &b
	}
	if n, ok := lookupInt(obj, "startsample", "startframes", "originsample"); ok {
		p.StartSample = &n
	}
	if n, ok := lookupUint(obj, "timereference", "bwftimereference", "bewttimereference"); ok {
		p.TimeReference = &n
	}
	if n, ok := lookupInt(obj, "durationsamples", "lengthsamples"); ok {
		p.DurationSamples = &n
	}
	if f, ok := lookupFloat(obj, "durationseconds", "durationsec"); ok {
		p.DurationSeconds = &f
	}
	return p, nil
}

func flattenJSON(v any, prefix string, out map[string][]string) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			flattenJSON(t[k], path, out)
		}
	case []any:
		for i, e := range t {
			flattenJSON(e, fmt.Sprintf("%s.%d", prefix, i), out)
		}
	default:
		key := normalizeKey(lastPathPart(prefix))
		out[key] = append(out[key], fmt.Sprintf("%v", v))
	}
}

func flattenProject(obj map[string]any) map[string][]string {
	out := make(map[string][]string)
	flattenJSON(obj, "", out)
	for k, vals := range out {
		if len(vals) > 1 {
			out[k] = vals
		}
	}
	return out
}

func lastPathPart(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i+1:]
	}
	return path
}

func normalizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("_", "", "-", "", " ", "").Replace(s)
	return s
}

func lookupRaw(obj map[string]any, keys ...string) (any, bool) {
	flat := flattenProject(obj)
	for _, wanted := range keys {
		if vals, ok := flat[normalizeKey(wanted)]; ok && len(vals) > 0 {
			return vals[0], true
		}
	}
	return nil, false
}

func lookupString(obj map[string]any, keys ...string) string {
	v, ok := lookupRaw(obj, keys...)
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func lookupInt(obj map[string]any, keys ...string) (int64, bool) {
	v, ok := lookupRaw(obj, keys...)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprintf("%v", v)), 10, 64)
	return n, err == nil
}

func lookupUint(obj map[string]any, keys ...string) (uint64, bool) {
	v, ok := lookupRaw(obj, keys...)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(fmt.Sprintf("%v", v)), 10, 64)
	return n, err == nil
}

func lookupFloat(obj map[string]any, keys ...string) (float64, bool) {
	v, ok := lookupRaw(obj, keys...)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprintf("%v", v)), 64)
	return f, err == nil
}

func lookupBool(obj map[string]any, keys ...string) (bool, bool) {
	v, ok := lookupRaw(obj, keys...)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v))) {
	case "true", "yes", "1", "df":
		return true, true
	case "false", "no", "0", "ndf":
		return false, true
	}
	return false, false
}
