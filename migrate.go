package audiotracks

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// EditTable is the legacy edit-table payload accepted by the migrator.
type EditTable struct {
	ProjectID string           `json:"projectId"`
	Title     string           `json:"title,omitempty"`
	Tracks    []EditTableTrack `json:"tracks"`
}

// EditTableTrack keeps the fields that editing-table exporters commonly use.
// Encoding aliases and snake_case names are handled by UnmarshalJSON.
type EditTableTrack struct {
	ID       string
	Name     string
	ClipName string
	Role     string
	Type     string
	Language string
	Default  *bool
	Audible  *bool
	Muted    *bool
}

// MigrateJSON decodes and migrates an edit-table document.
func MigrateJSON(data []byte) (Project, error) {
	var table EditTable
	if err := json.Unmarshal(data, &table); err != nil {
		return Project{}, fmt.Errorf("decode edit table: %w", err)
	}
	return Migrate(table)
}

// Migrate maps legacy tracks to canonical audio tracks. A track with role
// "audio" is not silently accepted: its semantic type must be recoverable
// from an explicit type field or from its name. All problems are aggregated
// and no project is returned if any track is invalid.
func Migrate(table EditTable) (Project, error) {
	problems := &ValidationError{}
	project := Project{
		ID:     table.ProjectID,
		Title:  table.Title,
		Tracks: make([]Track, 0, len(table.Tracks)),
	}

	for _, legacy := range table.Tracks {
		track := Track{
			ID:      legacy.ID,
			Name:    firstNonEmpty(legacy.Name, legacy.ClipName),
			Default: boolValue(legacy.Default),
			Audible: true,
		}
		track.Type = resolveTrackType(legacy, problems)
		track.Language = Language(strings.TrimSpace(legacy.Language))
		if strings.TrimSpace(string(track.Language)) == "" &&
			(track.Type == TrackMusic || track.Type == TrackEffects) {
			track.Language = LanguageUndetermined
		}
		track.Audible = resolveAudibility(legacy, problems)
		project.Tracks = append(project.Tracks, track)
	}

	normalized, validation := normalizeAndValidate(project)
	if validation != nil {
		problems.merge(validation)
	}
	if len(problems.Problems) > 0 {
		return Project{}, problems
	}
	return normalized, nil
}

func resolveTrackType(track EditTableTrack, problems *ValidationError) TrackType {
	roleType, roleSet, roleErr := classifyRole(track.Role)
	typeType, typeSet, typeErr := classifyRole(track.Type)

	switch {
	case roleSet && typeSet && roleType != typeType:
		problems.add(CodeConflictingType, track.ID, "",
			fmt.Sprintf("track %q declares conflicting types %q and %q",
				track.ID, roleType, typeType))
		return roleType
	case roleSet && typeErr != nil:
		problems.add(CodeInvalidTrackType, track.ID, "",
			fmt.Sprintf("track %q has an unsupported type: %v", track.ID, typeErr))
		return roleType
	case typeSet && roleErr != nil:
		problems.add(CodeInvalidTrackType, track.ID, "",
			fmt.Sprintf("track %q has an unsupported type: %v", track.ID, roleErr))
		return typeType
	case roleSet:
		return roleType
	case typeSet:
		return typeType
	}

	if roleErr != nil || typeErr != nil {
		problems.add(CodeInvalidTrackType, track.ID, "",
			fmt.Sprintf("track %q has an unsupported type: %v", track.ID,
				firstError(roleErr, typeErr)))
		return ""
	}

	inferred, ok := inferTrackType(track.Name, track.ClipName)
	if ok {
		return inferred
	}
	problems.add(CodeUnmappedTrackType, track.ID, "",
		fmt.Sprintf("track %q is generic audio and its semantic type cannot be inferred from name %q",
			track.ID, firstNonEmpty(track.Name, track.ClipName)))
	return ""
}

func classifyRole(raw string) (TrackType, bool, error) {
	if IsGenericAudioRole(raw) {
		return "", false, nil
	}
	trackType, err := ParseTrackType(raw)
	return trackType, err == nil, err
}

func resolveAudibility(track EditTableTrack, problems *ValidationError) bool {
	if track.Audible != nil && track.Muted != nil {
		if *track.Audible == *track.Muted {
			problems.add(CodeConflictingAudible, track.ID, "",
				fmt.Sprintf("track %q has contradictory audible=%t and muted=%t",
					track.ID, *track.Audible, *track.Muted))
		}
		return *track.Audible
	}
	if track.Audible != nil {
		return *track.Audible
	}
	if track.Muted != nil {
		return !*track.Muted
	}
	return true
}

func inferTrackType(names ...string) (TrackType, bool) {
	combined := strings.ToLower(strings.Join(names, " "))
	var found TrackType
	matches := 0
	for _, candidate := range roleNamePatterns {
		if candidate.pattern.MatchString(combined) {
			found = candidate.trackType
			matches++
		}
	}
	return found, matches == 1
}

var roleNamePatterns = []struct {
	trackType TrackType
	pattern   *regexp.Regexp
}{
	{TrackDialogue, regexp.MustCompile(`(?i)(^|[^a-z])(dialogue|dialog|dlg|dx|对白|台词)([^a-z]|$)`)},
	{TrackMusic, regexp.MustCompile(`(?i)(^|[^a-z])(music|mus|mx|bgm|score|配乐|音乐)([^a-z]|$)`)},
	{TrackEffects, regexp.MustCompile(`(?i)(^|[^a-z])(effects|effect|fx|sfx|foley|ambience|amb|音效|效果)([^a-z]|$)`)},
	{TrackDub, regexp.MustCompile(`(?i)(^|[^a-z])(dub|dubbing|voice[ -]?over|vo|配音)([^a-z]|$)`)},
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}
