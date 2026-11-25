package audioedit

import (
	"fmt"
	"sync"
)

// Editor owns a project and its derived table. Every successful mutation
// rebuilds the table for all clips; derived positions are never patched
// incrementally. A failed mutation is transactional and leaves both the
// project and table unchanged.
type Editor struct {
	mu      sync.RWMutex
	project Project
	table   Table
}

// NewEditor validates and compiles a project.
func NewEditor(project Project) (*Editor, error) {
	project = cloneProject(project)
	table, err := Compile(project)
	if err != nil {
		return nil, err
	}
	return &Editor{project: project, table: table}, nil
}

// Update applies fn to a copy of the project and recomputes the whole table.
func (e *Editor) Update(fn func(*Project) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	next := cloneProject(e.project)
	if err := fn(&next); err != nil {
		return err
	}
	table, err := Compile(next)
	if err != nil {
		return err
	}
	table.Revision = e.table.Revision + 1
	e.project = next
	e.table = table
	return nil
}

// Project returns a deep copy of the current declarative project.
func (e *Editor) Project() Project {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return cloneProject(e.project)
}

// Table returns a copy of the current derived sample-position table.
func (e *Editor) Table() Table {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return cloneTable(e.table)
}

// Revision reports how many successful recomputations have happened.
func (e *Editor) Revision() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.table.Revision
}

// SetClipRange changes one clip's presentation range. Positions of that clip
// and every following clip are recomputed.
func (e *Editor) SetClipRange(clipID string, start, end int64) error {
	return e.Update(func(project *Project) error {
		for i := range project.Clips {
			if project.Clips[i].ID == clipID {
				project.Clips[i].SourceStart = start
				project.Clips[i].SourceEnd = end
				return nil
			}
		}
		return fmt.Errorf("unknown clip %q", clipID)
	})
}

// SetCodecDelay changes a source's priming/pre-skip declaration and rebuilds
// every segment that depends on the source timing.
func (e *Editor) SetCodecDelay(sourceID string, samples int64) error {
	return e.Update(func(project *Project) error {
		for i := range project.Sources {
			if project.Sources[i].ID == sourceID {
				value := samples
				project.Sources[i].CodecDelay = &value
				return nil
			}
		}
		return fmt.Errorf("unknown source %q", sourceID)
	})
}

// SetSourceEdit changes the presentation range exposed by a container edit.
func (e *Editor) SetSourceEdit(sourceID string, start, end int64) error {
	return e.Update(func(project *Project) error {
		for i := range project.Sources {
			if project.Sources[i].ID == sourceID {
				project.Sources[i].Edit.MediaStart = start
				project.Sources[i].Edit.MediaEnd = end
				return nil
			}
		}
		return fmt.Errorf("unknown source %q", sourceID)
	})
}

// SetGapSamples changes the project-wide gap between clips. Every clip after
// the first gets a recomputed timeline position.
func (e *Editor) SetGapSamples(samples int64) error {
	return e.Update(func(project *Project) error {
		project.Params.GapSamples = samples
		return nil
	})
}

// SetProjectSampleRate changes the output time base. All output lengths and
// timeline positions are rescaled and recomputed.
func (e *Editor) SetProjectSampleRate(sampleRate int64) error {
	return e.Update(func(project *Project) error {
		project.Params.SampleRate = sampleRate
		return nil
	})
}

func cloneProject(project Project) Project {
	cloned := project
	cloned.Sources = append([]Source(nil), project.Sources...)
	for i := range cloned.Sources {
		cloned.Sources[i].CodecDelay = cloneInt64(project.Sources[i].CodecDelay)
		cloned.Sources[i].FrameSamples = cloneInt64(project.Sources[i].FrameSamples)
		cloned.Sources[i].EncodedSamples = cloneInt64(project.Sources[i].EncodedSamples)
		cloned.Sources[i].Packets = cloneInt64(project.Sources[i].Packets)
	}
	cloned.Clips = append([]Clip(nil), project.Clips...)
	return cloned
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTable(table Table) Table {
	cloned := table
	cloned.Segments = append([]Position(nil), table.Segments...)
	return cloned
}
