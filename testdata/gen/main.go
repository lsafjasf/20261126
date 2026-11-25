package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	out := flag.String("out", "testdata", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		panic(err)
	}
	files := map[string][]byte{
		"instrumentation-shifted.caf": shifted(),
		"instrumentation-healthy.caf": healthy(),
		"short-tail.caf":              shortTail(),
	}
	for name, data := range files {
		path := filepath.Join(*out, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			panic(err)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(data))
	}
}

func shifted() []byte {
	sizes := []int64{120, 180, 140, 160}
	desc := descChunk(48000, "aac ", 2, 0, 1024, 2, 0)
	pakt := chunk("pakt", pakt(4, 4096, 0, 0, sizes...))
	strg := stringsChunk(map[uint32]string{10: "instrument/packet=0", 11: "instrument/packet=2"})
	mark := markersChunk(
		marker(10, 1024),
		marker(11, 3072),
	)
	data := dataChunk(sizes, nil, false)
	return caf(desc, pakt, strg, mark, data)
}

func healthy() []byte {
	sizes := []int64{120, 180, 140, 160}
	desc := descChunk(48000, "aac ", 2, 0, 1024, 2, 0)
	pakt := chunk("pakt", pakt(4, 4096, 0, 0, sizes...))
	strg := stringsChunk(map[uint32]string{10: "instrument/packet=0", 11: "instrument/packet=2"})
	mark := markersChunk(
		marker(10, 0),
		marker(11, 2048),
	)
	data := dataChunk(sizes, nil, false)
	return caf(desc, pakt, strg, mark, data)
}

func shortTail() []byte {
	sizes := []int64{40, 55, 70}
	desc := descChunk(48000, "aac ", 0, 0, 1024, 2, 0)
	pakt := chunk("pakt", pakt(3, 3072, 0, 0, sizes...))
	mark := markersChunk(marker(20, 2048))
	data := dataChunk(sizes, map[int]int{2: 45}, true)
	return caf(desc, pakt, mark, data)
}

func caf(chunks ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString("caff")
	_ = binary.Write(&b, binary.BigEndian, uint16(1))
	_ = binary.Write(&b, binary.BigEndian, uint16(0))
	for _, c := range chunks {
		b.Write(c)
	}
	return b.Bytes()
}

func chunk(typ string, payload []byte) []byte {
	var b bytes.Buffer
	b.WriteString(typ)
	_ = binary.Write(&b, binary.BigEndian, int64(len(payload)))
	b.Write(payload)
	return b.Bytes()
}

func descChunk(rate float64, id string, flags, bpp, fpp, ch, bits uint32) []byte {
	p := make([]byte, 32)
	binary.BigEndian.PutUint64(p[0:8], math.Float64bits(rate))
	copy(p[8:12], id)
	binary.BigEndian.PutUint32(p[12:16], flags)
	binary.BigEndian.PutUint32(p[16:20], bpp)
	binary.BigEndian.PutUint32(p[20:24], fpp)
	binary.BigEndian.PutUint32(p[24:28], ch)
	binary.BigEndian.PutUint32(p[28:32], bits)
	return chunk("desc", p)
}

func pakt(count, valid int64, priming, remainder int32, sizes ...int64) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, count)
	_ = binary.Write(&b, binary.BigEndian, valid)
	_ = binary.Write(&b, binary.BigEndian, priming)
	_ = binary.Write(&b, binary.BigEndian, remainder)
	for _, size := range sizes {
		b.Write(varint(size))
	}
	return b.Bytes()
}

func varint(n int64) []byte {
	v := uint64(n)
	var groups []byte
	for {
		g := byte(v & 0x7f)
		v >>= 7
		groups = append(groups, g)
		if v == 0 {
			break
		}
	}
	out := make([]byte, len(groups))
	for i := range groups {
		out[len(groups)-1-i] = groups[i]
	}
	for i := 0; i < len(out)-1; i++ {
		out[i] |= 0x80
	}
	return out
}

func markersChunk(markers ...[]byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(0))
	_ = binary.Write(&b, binary.BigEndian, uint32(len(markers)))
	for _, m := range markers {
		b.Write(m)
	}
	return chunk("mark", b.Bytes())
}

func marker(id uint32, frame float64) []byte {
	m := make([]byte, 28)
	binary.BigEndian.PutUint32(m[0:4], 0)
	binary.BigEndian.PutUint64(m[4:12], math.Float64bits(frame))
	binary.BigEndian.PutUint32(m[12:16], id)
	for i := 16; i < 24; i++ {
		m[i] = 0xff
	}
	return m
}

func stringsChunk(values map[uint32]string) []byte {
	var table, blob bytes.Buffer
	_ = binary.Write(&table, binary.BigEndian, uint32(len(values)))
	ids := make([]uint32, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	offset := int64(0)
	for _, id := range ids {
		value := values[id]
		_ = binary.Write(&table, binary.BigEndian, id)
		_ = binary.Write(&table, binary.BigEndian, offset)
		blob.WriteString(value)
		blob.WriteByte(0)
		offset += int64(len(value) + 1)
	}
	return chunk("strg", append(table.Bytes(), blob.Bytes()...))
}

func dataChunk(sizes []int64, shorten map[int]int, truncatedHeader bool) []byte {
	audio := make([]byte, 0)
	for i, size := range sizes {
		n := int(size)
		if keep, ok := shorten[i]; ok {
			n = keep
		}
		audio = append(audio, make([]byte, n)...)
	}
	declared := int64(len(audio))
	if truncatedHeader {
		var total int64
		for _, size := range sizes {
			total += size
		}
		declared = total
	}
	var b bytes.Buffer
	b.WriteString("data")
	_ = binary.Write(&b, binary.BigEndian, declared+4)
	_ = binary.Write(&b, binary.BigEndian, uint32(0))
	b.Write(audio)
	return b.Bytes()
}
