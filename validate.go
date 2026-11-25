package audiotracks

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidProject = errors.New("invalid audio project")

const (
	CodeInvalidProjectID   = "invalid_project_id"
	CodeInvalidTrackID     = "invalid_track_id"
	CodeDuplicateTrackID   = "duplicate_track_id"
	CodeMissingLanguage    = "missing_language"
	CodeInvalidLanguage    = "invalid_language"
	CodeVoiceNeedsLanguage = "voice_needs_concrete_language"
	CodeInvalidTrackType   = "invalid_track_type"
	CodeDuplicateDefault   = "duplicate_default"
	CodeMutedDefault       = "muted_default"
	CodeConflictingType    = "conflicting_track_type"
	CodeConflictingAudible = "conflicting_audibility"
	CodeUnmappedTrackType  = "unmapped_track_type"
)

// Problem is one reason why an entire project cannot be saved.
type Problem struct {
	Code     string `json:"code"`
	TrackID  string `json:"trackId,omitempty"`
	Language string `json:"language,omitempty"`
	Message  string `json:"message"`
}

// ValidationError aggregates every detected problem so a caller can fix the
// batch in one pass. None of the tracks are saved when this error is returned.
type ValidationError struct {
	Problems []Problem `json:"problems"`
}

func (e *ValidationError) Error() string {
	if len(e.Problems) == 0 {
		return ErrInvalidProject.Error()
	}
	if len(e.Problems) == 1 {
		return fmt.Sprintf("%s: %s", ErrInvalidProject, e.Problems[0].Message)
	}
	return fmt.Sprintf("%s: %d problems, first: %s",
		ErrInvalidProject, len(e.Problems), e.Problems[0].Message)
}

func (e *ValidationError) Unwrap() error { return ErrInvalidProject }

func (e *ValidationError) HasCode(code string) bool {
	for _, problem := range e.Problems {
		if problem.Code == code {
			return true
		}
	}
	return false
}

func (e *ValidationError) add(code, trackID, language, message string) {
	for _, existing := range e.Problems {
		if existing.Code == code && existing.TrackID == trackID {
			return
		}
	}
	e.Problems = append(e.Problems, Problem{
		Code: code, TrackID: trackID, Language: language, Message: message,
	})
}

func (e *ValidationError) merge(other *ValidationError) {
	for _, problem := range other.Problems {
		e.add(problem.Code, problem.TrackID, problem.Language, problem.Message)
	}
}

// ValidateProject checks the complete project without mutating it.
func ValidateProject(project Project) error {
	_, err := normalizeAndValidate(project)
	if err == nil {
		return nil
	}
	return err
}

// NormalizeProject validates and returns a canonical copy. The input is
// never mutated, which keeps a failed save from leaking partial changes.
func NormalizeProject(project Project) (Project, error) {
	normalized, err := normalizeAndValidate(project)
	if err == nil {
		return normalized, nil
	}
	return normalized, err
}

func normalizeAndValidate(project Project) (Project, *ValidationError) {
	problems := &ValidationError{}
	out := Project{
		ID:     strings.TrimSpace(project.ID),
		Title:  strings.TrimSpace(project.Title),
		Tracks: make([]Track, 0, len(project.Tracks)),
	}
	if out.ID == "" {
		problems.add(CodeInvalidProjectID, "", "", "projectId must not be empty")
	}

	seenIDs := make(map[string]struct{}, len(project.Tracks))
	defaults := make(map[Language]string, len(project.Tracks))
	for _, source := range project.Tracks {
		track := source
		track.ID = strings.TrimSpace(track.ID)
		track.Name = strings.TrimSpace(track.Name)
		if track.ID == "" {
			problems.add(CodeInvalidTrackID, "", "", "track id must not be empty")
		} else if _, exists := seenIDs[track.ID]; exists {
			problems.add(CodeDuplicateTrackID, track.ID, "",
				fmt.Sprintf("track id %q is duplicated", track.ID))
		} else {
			seenIDs[track.ID] = struct{}{}
		}

		if !track.Type.Valid() {
			problems.add(CodeInvalidTrackType, track.ID, "",
				fmt.Sprintf("track %q has unsupported type %q", track.ID, track.Type))
		}
		language, err := ParseLanguage(string(track.Language))
		if err != nil {
			if strings.TrimSpace(string(track.Language)) == "" {
				problems.add(CodeMissingLanguage, track.ID, "",
					fmt.Sprintf("track %q has no language", track.ID))
			} else {
				problems.add(CodeInvalidLanguage, track.ID, string(track.Language),
					fmt.Sprintf("track %q has invalid language %q", track.ID, track.Language))
			}
		} else {
			track.Language = language
			if track.Type.IsVoice() && language.IsNeutral() {
				problems.add(CodeVoiceNeedsLanguage, track.ID, string(language),
					fmt.Sprintf("voice track %q cannot use neutral language %q", track.ID, language))
			}
		}

		if track.Default && !track.Audible {
			problems.add(CodeMutedDefault, track.ID, string(track.Language),
				fmt.Sprintf("default track %q cannot be muted", track.ID))
		}
		if track.Default && language != "" {
			if firstID, exists := defaults[language]; exists {
				problems.add(CodeDuplicateDefault, track.ID, string(language),
					fmt.Sprintf("language %q has multiple defaults: %q and %q",
						language, firstID, track.ID))
			} else {
				defaults[language] = track.ID
			}
		}
		out.Tracks = append(out.Tracks, track)
	}

	if len(problems.Problems) > 0 {
		return Project{}, problems
	}
	return out, nil
}
