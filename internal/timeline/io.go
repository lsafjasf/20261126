package timeline

import (
	"encoding/json"
	"io"
	"os"
)

type Input struct {
	ProjectName    string   `json:"project_name"`
	SequenceRate   Rate     `json:"sequence_rate"`
	DurationFrames int64    `json:"duration_frames"`
	Rates          []Rate   `json:"rates"`
	Markers        []Marker `json:"markers"`
}

func ReadInput(path string) (Timeline, error) {
	var reader io.Reader
	if path == "" || path == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(path)
		if err != nil {
			return Timeline{}, err
		}
		defer file.Close()
		reader = file
	}
	var input Input
	if err := json.NewDecoder(reader).Decode(&input); err != nil {
		return Timeline{}, err
	}
	return Timeline{
		ProjectName:    input.ProjectName,
		SequenceRate:   input.SequenceRate,
		DurationFrames: input.DurationFrames,
		Rates:          input.Rates,
		Markers:        input.Markers,
	}, nil
}

func WriteJSON(report Report, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func WriteMarkdown(markdown string, path string) error {
	data := []byte(markdown)
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, '\n')
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
