package mka

import (
	"encoding/binary"
	"math"
)

type StoredTrack struct {
	Number            uint64
	Type              uint64
	CodecID           string
	CodecDelayNS      uint64
	SeekPreRollNS     uint64
	DefaultDurationNS uint64
}

type StoredBlock struct {
	TrackID     uint64
	TimestampNS int64
	Payload     []byte
}

type MatroskaFile struct {
	Tracks []StoredTrack
	Blocks []StoredBlock
}

func ParseMatroska(data []byte) (MatroskaFile, error) {
	roots, err := DecodeElements(data)
	if err != nil {
		return MatroskaFile{}, err
	}

	result := MatroskaFile{}
	for _, root := range roots {
		if !EqualID(root.ID, idSegment) {
			continue
		}
		segmentChildren, err := DecodeElements(root.Binary)
		if err != nil {
			return MatroskaFile{}, err
		}
		for _, child := range segmentChildren {
			switch {
			case EqualID(child.ID, idTracks):
				if err := parseTracks(child, &result); err != nil {
					return MatroskaFile{}, err
				}
			case EqualID(child.ID, idCluster):
				if err := parseCluster(child, &result); err != nil {
					return MatroskaFile{}, err
				}
			}
		}
	}
	return result, nil
}

func parseTracks(tracks Element, result *MatroskaFile) error {
	entries, err := DecodeElements(tracks.Binary)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		children, err := DecodeElements(entry.Binary)
		if err != nil {
			return err
		}
		stored := StoredTrack{}
		for _, child := range children {
			switch {
			case EqualID(child.ID, idTrackNumber):
				stored.Number = DecodeUint(child)
			case EqualID(child.ID, idTrackType):
				stored.Type = DecodeUint(child)
			case EqualID(child.ID, idCodecID):
				stored.CodecID = string(child.Binary)
			case EqualID(child.ID, idCodecDelay):
				stored.CodecDelayNS = DecodeUint(child)
			case EqualID(child.ID, idSeekPreRoll):
				stored.SeekPreRollNS = DecodeUint(child)
			case EqualID(child.ID, idDefaultDuration):
				stored.DefaultDurationNS = DecodeUint(child)
			}
		}
		result.Tracks = append(result.Tracks, stored)
	}
	return nil
}

func parseCluster(cluster Element, result *MatroskaFile) error {
	children, err := DecodeElements(cluster.Binary)
	if err != nil {
		return err
	}
	var clusterTimestamp int64
	for _, child := range children {
		if EqualID(child.ID, idClusterTimestamp) {
			clusterTimestamp = int64(DecodeUint(child))
			break
		}
	}
	for _, child := range children {
		if !EqualID(child.ID, idSimpleBlock) {
			continue
		}
		trackID, idLength, err := readVINT(child.Binary)
		if err != nil {
			return err
		}
		relative := int64(int16(binary.BigEndian.Uint16(child.Binary[idLength : idLength+2])))
		result.Blocks = append(result.Blocks, StoredBlock{
			TrackID:     trackID,
			TimestampNS: clusterTimestamp + relative,
			Payload:     append([]byte(nil), child.Binary[idLength+3:]...),
		})
	}
	return nil
}

func DecodeFloat(element Element) float64 {
	return math.Float64frombits(binary.BigEndian.Uint64(element.Binary))
}
