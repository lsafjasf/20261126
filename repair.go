package channelmap

import "fmt"

// ReorderChannels returns channels whose output index i is copied from
// sourceOrder[i] in the input. This matches Report.Repair.SourceIndicesForMaskOrder.
func ReorderChannels(channels [][]float64, sourceOrder []int) ([][]float64, error) {
	if len(sourceOrder) == 0 {
		return nil, fmt.Errorf("repair: empty source order")
	}
	if len(sourceOrder) != len(channels) {
		return nil, fmt.Errorf("repair: order length %d does not match channel count %d", len(sourceOrder), len(channels))
	}
	used := make([]bool, len(channels))
	out := make([][]float64, len(channels))
	for outIndex, sourceIndex := range sourceOrder {
		if sourceIndex < 0 || sourceIndex >= len(channels) {
			return nil, fmt.Errorf("repair: source index %d out of range", sourceIndex)
		}
		if used[sourceIndex] {
			return nil, fmt.Errorf("repair: source index %d used more than once", sourceIndex)
		}
		used[sourceIndex] = true
		out[outIndex] = channels[sourceIndex]
	}
	return out, nil
}
