package mka

import "bytes"

var (
	idEBML              = []byte{0x1a, 0x45, 0xdf, 0xa3}
	idDocType           = []byte{0x42, 0x82}
	idSegment           = []byte{0x18, 0x53, 0x80, 0x67}
	idInfo              = []byte{0x15, 0x49, 0xa9, 0x66}
	idTimestampScale    = []byte{0x2a, 0xd7, 0xb1}
	idMuxingApp         = []byte{0x4d, 0x80}
	idTracks            = []byte{0x16, 0x54, 0xae, 0x6b}
	idTrackEntry        = []byte{0xae}
	idTrackNumber       = []byte{0xd7}
	idTrackUID          = []byte{0x73, 0xc5}
	idTrackType         = []byte{0x83}
	idCodecID           = []byte{0x86}
	idAudio             = []byte{0xe1}
	idSamplingFrequency = []byte{0xb5}
	idChannels          = []byte{0x9f}
	idDefaultDuration   = []byte{0x23, 0xe3, 0x83}
	idCodecDelay        = []byte{0x56, 0xaa}
	idSeekPreRoll       = []byte{0x56, 0xbb}
	idCluster           = []byte{0x1f, 0x43, 0xb6, 0x75}
	idClusterTimestamp  = []byte{0xe7}
	idSimpleBlock       = []byte{0xa3}
)

func floatElement(id []byte, value uint64) Element {
	return Element{ID: id, Float: float64(value)}
}

func audioElement(track Track) Element {
	return Element{ID: idAudio, Children: []Element{
		floatElement(idSamplingFrequency, uint64(track.Spec.SampleRate)),
		{ID: idChannels, Uint: 1},
	}}
}

func trackElement(track Track) Element {
	return Element{ID: idTrackEntry, Children: []Element{
		{ID: idTrackNumber, Uint: track.Spec.ID},
		{ID: idTrackUID, Uint: track.Spec.ID},
		{ID: idTrackType, Uint: 2},
		{ID: idCodecID, String: string(track.Spec.Codec)},
		audioElement(track),
		{ID: idDefaultDuration, Uint: uint64(track.Packets[0].DurationNS)},
		{ID: idCodecDelay, Uint: uint64(track.CodecDelayNS)},
		{ID: idSeekPreRoll, Uint: uint64(track.SeekPreRollNS)},
	}}
}

func simpleBlock(packet Packet, clusterTimestamp int64) Element {
	relative := packet.RawTimestampNS - clusterTimestamp
	header := bytes.Buffer{}
	header.Write(unsignedVINT(packet.TrackID, 1))
	header.WriteByte(byte(int16(relative) >> 8))
	header.WriteByte(byte(int16(relative)))
	header.WriteByte(0)
	header.Write(packet.Payload)
	return Element{ID: idSimpleBlock, Binary: header.Bytes()}
}

func clusterElement(packets []Packet) Element {
	clusterTimestamp := packets[0].RawTimestampNS
	children := []Element{{ID: idClusterTimestamp, Uint: uint64(clusterTimestamp)}}
	for _, packet := range packets {
		children = append(children, simpleBlock(packet, clusterTimestamp))
	}
	return Element{ID: idCluster, Children: children}
}

func SerializeMatroska(tracks []Track) []byte {
	rootChildren := []Element{
		{ID: idEBML, Children: []Element{
			{ID: idDocType, String: "matroska"},
		}},
	}

	info := Element{ID: idInfo, Children: []Element{
		{ID: idTimestampScale, Uint: 1},
		{ID: idMuxingApp, String: "go-stdlib timing fixture"},
	}}
	trackEntries := make([]Element, 0, len(tracks))
	for _, track := range tracks {
		trackEntries = append(trackEntries, trackElement(track))
	}
	segmentChildren := []Element{info, {ID: idTracks, Children: trackEntries}}
	for _, track := range tracks {
		for _, packet := range track.Packets {
			segmentChildren = append(segmentChildren, clusterElement([]Packet{packet}))
		}
	}

	rootChildren = append(rootChildren, Element{ID: idSegment, Children: segmentChildren})
	document := bytes.Buffer{}
	for _, element := range rootChildren {
		document.Write(element.Encode())
	}
	return document.Bytes()
}
