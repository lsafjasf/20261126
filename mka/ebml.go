package mka

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

type Element struct {
	ID       []byte
	Children []Element
	Uint     uint64
	String   string
	Binary   []byte
	Float    float64
}

func unsignedVINT(value uint64, length int) []byte {
	data := make([]byte, length)
	for index := length - 1; index >= 0; index-- {
		data[index] = byte(value >> (uint(length-1-index) * 8))
	}
	data[0] |= 1 << (8 - uint(length))
	return data
}

func elementSize(value uint64) int {
	if value == 0 {
		return 1
	}
	for length := 1; length <= 8; length++ {
		if value < uint64(1)<<(7*uint(length)-1) {
			return length
		}
	}
	panic("EBML integer is too large")
}

func encodeUnsigned(value uint64) []byte {
	if value == 0 {
		return []byte{0}
	}
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], value)
	for index, octet := range data {
		if octet != 0 {
			return data[index:]
		}
	}
	return data[:]
}

func (element Element) encodeBody() []byte {
	if element.Children != nil {
		buffer := bytes.Buffer{}
		for _, child := range element.Children {
			buffer.Write(child.Encode())
		}
		return buffer.Bytes()
	}
	if element.Binary != nil {
		return element.Binary
	}
	if element.Float != 0 {
		var data [8]byte
		binary.BigEndian.PutUint64(data[:], math.Float64bits(element.Float))
		return data[:]
	}
	if element.String != "" {
		return []byte(element.String)
	}
	return encodeUnsigned(element.Uint)
}

func (element Element) Encode() []byte {
	body := element.encodeBody()
	size := unsignedVINT(uint64(len(body)), elementSize(uint64(len(body))))
	return bytes.Join([][]byte{element.ID, size, body}, nil)
}

type Reader struct {
	data []byte
	pos  int
}

func readVINT(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	first := data[0]
	length := 0
	for bit := 7; bit >= 0; bit-- {
		if first&(1<<uint(bit)) != 0 {
			length = 8 - bit
			break
		}
	}
	if length == 0 || len(data) < length {
		return 0, 0, errors.New("invalid EBML VINT")
	}
	value := uint64(first & (1<<uint(8-length) - 1))
	for index := 1; index < length; index++ {
		value = value<<8 | uint64(data[index])
	}
	return value, length, nil
}

func DecodeElements(data []byte) ([]Element, error) {
	reader := Reader{data: data}
	elements := make([]Element, 0)
	for reader.pos < len(reader.data) {
		element, err := reader.readElement()
		if err != nil {
			return nil, err
		}
		elements = append(elements, element)
	}
	return elements, nil
}

func (reader *Reader) readElement() (Element, error) {
	idStart := reader.pos
	if _, length, err := readVINT(reader.data[reader.pos:]); err != nil {
		return Element{}, err
	} else {
		reader.pos += length
		id := reader.data[idStart:reader.pos]
		size, sizeLength, err := readVINT(reader.data[reader.pos:])
		if err != nil {
			return Element{}, err
		}
		reader.pos += sizeLength
		end := reader.pos + int(size)
		if end > len(reader.data) {
			return Element{}, io.ErrUnexpectedEOF
		}
		element := Element{ID: id, Binary: append([]byte(nil), reader.data[reader.pos:end]...)}
		reader.pos = end
		return element, nil
	}
}

func EqualID(left, right []byte) bool {
	return bytes.Equal(left, right)
}

func FindElement(elements []Element, id []byte) (Element, bool) {
	for _, element := range elements {
		if EqualID(element.ID, id) {
			return element, true
		}
	}
	return Element{}, false
}

func DecodeUint(element Element) uint64 {
	var value uint64
	for _, octet := range element.Binary {
		value = value<<8 | uint64(octet)
	}
	return value
}
