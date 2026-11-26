// A group indexes independently compressed actors. Looking up one provider
// does not inflate any other actor's pixels.
package markatlas

import (
	"encoding/binary"
	"errors"
)

const groupMagic = "BHG1"

func Part(data []byte, part int) ([]byte, error) {
	if len(data) >= 4 && string(data[:4]) == magic && part == 0 {
		return data, nil
	}
	if len(data) < 5 || string(data[:4]) != groupMagic {
		return nil, errors.New("invalid mark atlas group")
	}
	n := int(data[4])
	if part < 0 || part >= n || len(data) < 5+4*(n+1) {
		return nil, errors.New("mark atlas part out of bounds")
	}
	body := data[5+4*(n+1):]
	offset := 5 + 4*part
	left := uint64(binary.LittleEndian.Uint32(data[offset:]))
	right := uint64(binary.LittleEndian.Uint32(data[offset+4:]))
	if left >= right || right > uint64(len(body)) {
		return nil, errors.New("invalid mark atlas part span")
	}
	return body[left:right], nil
}
