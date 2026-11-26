// Package markatlas stores cropped animation cells in a lossless PNG atlas.
package markatlas

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/draw"
	"image/png"
)

const (
	Cell       = 58
	Columns    = 8
	magic      = "BHA1"
	headerSize = 6
	recordSize = 8
)

// Atlas holds just the packed pixels and a small index into the embedded file.
type Atlas struct {
	index  []byte
	pixels image.Image
}

func Open(data []byte) (Atlas, error) {
	if len(data) < headerSize || string(data[:4]) != magic {
		return Atlas{}, errors.New("invalid mark atlas header")
	}
	end := headerSize + recordSize*int(binary.LittleEndian.Uint16(data[4:]))
	if end >= len(data) {
		return Atlas{}, errors.New("truncated mark atlas")
	}
	im, err := png.Decode(bytes.NewReader(data[end:]))
	if err != nil {
		return Atlas{}, err
	}
	a := Atlas{data[headerSize:end], im}
	for i := range a.Len() {
		r, offset := a.frame(i)
		if !r.In(im.Bounds()) || offset.X+r.Dx() > Cell || offset.Y+r.Dy() > Cell {
			return Atlas{}, errors.New("mark atlas frame out of bounds")
		}
	}
	return a, nil
}

func (a Atlas) Len() int { return len(a.index) / recordSize }

func (a Atlas) frame(i int) (image.Rectangle, image.Point) {
	r := a.index[i*recordSize:]
	x, y := int(binary.LittleEndian.Uint16(r)), int(binary.LittleEndian.Uint16(r[2:]))
	return image.Rect(x, y, x+int(r[4]), y+int(r[5])), image.Pt(int(r[6]), int(r[7]))
}

func (a Atlas) Draw(dst *image.RGBA, origin image.Point, i int) {
	src, offset := a.frame(i)
	origin = origin.Add(offset)
	draw.Draw(dst, image.Rectangle{Min: origin, Max: origin.Add(src.Size())}, a.pixels, src.Min, draw.Over)
}

// Tile owns just one frame, independent of the atlas it came from.
type Tile struct {
	pixels *image.NRGBA
	offset image.Point
}

func (a Atlas) CopyTile(i int) Tile {
	src, offset := a.frame(i)
	im := image.NewNRGBA(image.Rectangle{Max: src.Size()})
	draw.Draw(im, im.Bounds(), a.pixels, src.Min, draw.Src)
	return Tile{im, offset}
}

func (t Tile) Draw(dst *image.RGBA, origin image.Point) {
	origin = origin.Add(t.offset)
	draw.Draw(dst, t.pixels.Bounds().Add(origin), t.pixels, image.Point{}, draw.Over)
}
