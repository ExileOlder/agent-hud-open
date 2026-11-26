package markatlas

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
)

// Encode removes untouched transparent borders and shares identical crops.
// The index keeps every original frame position and the runtime blends the
// same straight-alpha PNG channels as the original grid.
func Encode(w io.Writer, grid image.Image) error {
	return encode(w, grid, Columns*Cell)
}

func encode(w io.Writer, grid image.Image, width int) error {
	bounds := grid.Bounds()
	if bounds.Dx() != Columns*Cell || bounds.Dy()%Cell != 0 {
		return errors.New("invalid mark atlas grid")
	}
	n := Columns * bounds.Dy() / Cell
	index := make([]byte, headerSize+n*recordSize)
	copy(index, magic)
	binary.LittleEndian.PutUint16(index[4:], uint16(n))
	type crop struct {
		origin image.Point
		pixels []byte
		size   image.Point
	}
	var crops []crop
	seen := map[string]image.Point{}
	x, y, row := 0, 0, 0
	for i := range n {
		top := bounds.Min.Add(image.Pt(i%Columns*Cell, i/Columns*Cell))
		var cell [Cell * Cell * 4]byte
		left, right, bottom, upper := Cell, 0, 0, Cell
		for cy := range Cell {
			for cx := range Cell {
				p := color.NRGBAModel.Convert(grid.At(top.X+cx, top.Y+cy)).(color.NRGBA)
				if p.A == 0 {
					continue
				}
				k := (cy*Cell + cx) * 4
				cell[k], cell[k+1], cell[k+2], cell[k+3] = p.R, p.G, p.B, p.A
				left, right = min(left, cx), max(right, cx+1)
				upper, bottom = min(upper, cy), max(bottom, cy+1)
			}
		}
		if left == Cell {
			continue
		}
		cw, ch := right-left, bottom-upper
		raw := make([]byte, 2, 2+cw*ch*4)
		raw[0], raw[1] = byte(cw), byte(ch)
		for cy := upper; cy < bottom; cy++ {
			raw = append(raw, cell[(cy*Cell+left)*4:(cy*Cell+right)*4]...)
		}
		key := string(raw)
		at, ok := seen[key]
		if !ok {
			if x+cw > width {
				x, y, row = 0, y+row, 0
			}
			at = image.Pt(x, y)
			seen[key] = at
			crops = append(crops, crop{at, raw[2:], image.Pt(cw, ch)})
			x += cw
			row = max(row, ch)
		}
		rec := index[headerSize+i*recordSize:]
		binary.LittleEndian.PutUint16(rec, uint16(at.X))
		binary.LittleEndian.PutUint16(rec[2:], uint16(at.Y))
		rec[4], rec[5], rec[6], rec[7] = byte(cw), byte(ch), byte(left), byte(upper)
	}
	im := image.NewNRGBA(image.Rect(0, 0, width, max(1, y+row)))
	for _, c := range crops {
		for cy := range c.size.Y {
			at := im.PixOffset(c.origin.X, c.origin.Y+cy)
			copy(im.Pix[at:at+c.size.X*4], c.pixels[cy*c.size.X*4:(cy+1)*c.size.X*4])
		}
	}
	if _, err := w.Write(index); err != nil {
		return err
	}
	return (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(w, im)
}

// EncodeGroup encodes equal, contiguous sets of frames for each provider.
func EncodeGroup(w io.Writer, grid image.Image, parts int) error {
	if parts == 1 {
		return Encode(w, grid)
	}
	b := grid.Bounds()
	if parts < 2 || parts > 255 || b.Dx() != Cell*Columns || b.Dy() == 0 || b.Dy()%(parts*Cell) != 0 {
		return errors.New("invalid atlas group grid")
	}
	height := b.Dy() / parts
	head := make([]byte, 5+4*(parts+1))
	copy(head, groupMagic)
	head[4] = byte(parts)
	var body bytes.Buffer
	for i := range parts {
		area := image.Rect(b.Min.X, b.Min.Y+i*height, b.Max.X, b.Min.Y+(i+1)*height)
		var best []byte
		var candidate bytes.Buffer
		// Width controls how related tiles line up in PNG's scanline filter.
		// Choose at build time; the runtime format is unchanged.
		for _, width := range [...]int{Cell * 4, Cell * 6, Cell * 8} {
			candidate.Reset()
			if err := encode(&candidate, viewport{grid, area}, width); err != nil {
				return err
			}
			if best == nil || candidate.Len() < len(best) {
				best = append(best[:0], candidate.Bytes()...)
			}
		}
		body.Write(best)
		binary.LittleEndian.PutUint32(head[5+4*(i+1):], uint32(body.Len()))
	}
	if _, err := w.Write(head); err != nil {
		return err
	}
	_, err := body.WriteTo(w)
	return err
}

type viewport struct {
	image.Image
	area image.Rectangle
}

func (v viewport) Bounds() image.Rectangle { return v.area }
