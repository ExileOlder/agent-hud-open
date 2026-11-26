// Package markatlas stores cropped animation cells in a lossless atlas.
package markatlas

import (
	"encoding/binary"
	"errors"
	"image"
	"image/draw"

	"github.com/klauspost/compress/zstd"
)

const (
	Cell       = 58
	Columns    = 8
	magic      = "BHA2"
	headerSize = 6
	recordSize = 8
	dimensions = 4
	maxPixels  = 1 << 20
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
	if end+dimensions >= len(data) {
		return Atlas{}, errors.New("truncated mark atlas")
	}
	width := int(binary.LittleEndian.Uint16(data[end:]))
	height := int(binary.LittleEndian.Uint16(data[end+2:]))
	if width == 0 || height == 0 || height > maxPixels/width {
		return Atlas{}, errors.New("invalid mark atlas dimensions")
	}
	size := 4 * width * height
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxMemory(4*maxPixels), zstd.WithDecodeAllCapLimit(true))
	if err != nil {
		return Atlas{}, err
	}
	defer dec.Close()
	raw, err := dec.DecodeAll(data[end+dimensions:], make([]byte, 0, size))
	if err != nil {
		return Atlas{}, err
	}
	if len(raw) != size {
		return Atlas{}, errors.New("invalid mark atlas pixel data")
	}
	im := &image.NRGBA{Pix: raw, Stride: 4 * width, Rect: image.Rect(0, 0, width, height)}
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
