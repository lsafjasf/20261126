package audiotracks

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// UnmarshalJSON accepts camelCase, snake_case and kebab-case spellings used
// by different edit-table exporters.
func (table *EditTable) UnmarshalJSON(data []byte) error {
	raw, err := objectFields(data)
	if err != nil {
		return err
	}
	*table = EditTable{}
	for key, value := range raw {
		switch normalizeJSONKey(key) {
		case "projectid", "productionid", "showid":
			err = setStringField(&table.ProjectID, key, value)
		case "title", "projectname", "name":
			err = setStringField(&table.Title, key, value)
		case "tracks", "audiotracks", "audio":
			err = json.Unmarshal(value, &table.Tracks)
		}
		if err != nil {
			return fmt.Errorf("edit table field %q: %w", key, err)
		}
	}
	return nil
}

// UnmarshalJSON keeps generic audio-container records lossless enough for
// migration while normalizing the many names for the same legacy field.
func (track *EditTableTrack) UnmarshalJSON(data []byte) error {
	raw, err := objectFields(data)
	if err != nil {
		return err
	}
	*track = EditTableTrack{}
	for key, value := range raw {
		switch normalizeJSONKey(key) {
		case "id", "trackid", "trackuuid":
			err = setStringField(&track.ID, key, value)
		case "name", "trackname", "label", "displayname":
			err = setStringField(&track.Name, key, value)
		case "clipname", "clip", "sourceclipname":
			err = setStringField(&track.ClipName, key, value)
		case "role", "trackrole":
			err = setStringField(&track.Role, key, value)
		case "type", "tracktype", "mediatype", "kind":
			err = setStringField(&track.Type, key, value)
		case "language", "lang", "languagecode", "langcode", "locale":
			err = setStringField(&track.Language, key, value)
		case "default", "isdefault", "defaulttrack", "selected":
			err = setBoolField(&track.Default, key, value)
		case "audible", "isaudible", "hearable":
			err = setBoolField(&track.Audible, key, value)
		case "muted", "ismuted", "mute":
			err = setBoolField(&track.Muted, key, value)
		}
		if err != nil {
			return fmt.Errorf("track field %q: %w", key, err)
		}
	}
	return nil
}

func objectFields(data []byte) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	return raw, nil
}

func normalizeJSONKey(key string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(key) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func setStringField(target *string, key string, raw json.RawMessage) error {
	if string(raw) == "null" {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("expected string: %w", err)
	}
	if *target != "" && *target != value {
		return fmt.Errorf("conflicting string aliases %q and %q", *target, value)
	}
	*target = value
	return nil
}

func setBoolField(target **bool, key string, raw json.RawMessage) error {
	if string(raw) == "null" {
		return nil
	}
	value, err := decodeJSONBool(raw)
	if err != nil {
		return err
	}
	if *target != nil && **target != value {
		return fmt.Errorf("conflicting boolean aliases %t and %t", **target, value)
	}
	*target = &value
	return nil
}

func decodeJSONBool(raw json.RawMessage) (bool, error) {
	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, parseErr := strconv.ParseBool(strings.TrimSpace(text))
		if parseErr != nil {
			return false, fmt.Errorf("expected boolean, got %q", text)
		}
		return parsed, nil
	}

	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		switch number {
		case 0:
			return false, nil
		case 1:
			return true, nil
		}
	}
	return false, fmt.Errorf("expected boolean, 0 or 1")
}
