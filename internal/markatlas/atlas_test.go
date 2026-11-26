package markatlas

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestPackedAtlasPreservesCropsAndAlpha(t *testing.T) {
	grid := image.NewNRGBA(image.Rect(0, 0, Cell*Columns, Cell))
	for i, x := range []int{1, 47} {
		origin := image.Pt(Cell*i+x, 12+i)
		grid.SetNRGBA(origin.X, origin.Y,
			color.NRGBA{R: 240, G: 180, B: 100, A: 103})
		grid.SetNRGBA(origin.X+1, origin.Y+2,
			color.NRGBA{R: 5, G: 19, B: 222, A: 255})
	}
	var buf bytes.Buffer
	if err := Encode(&buf, grid); err != nil {
		t.Fatal(err)
	}
	a, err := Open(buf.Bytes())
	if err != nil || a.Len() != Columns {
		t.Fatalf("load atlas = %v, %v", a.Len(), err)
	}
	first, where := a.frame(0)
	second, other := a.frame(1)
	if first != second || where == other {
		t.Fatal("duplicate crops should share pixels but retain their offsets")
	}
	for i := range Columns {
		dst := image.Rect(0, 0, Cell, Cell)
		background := image.NewUniform(color.RGBA{27, 40, 80, 255})
		old, packed := image.NewRGBA(dst), image.NewRGBA(dst)
		draw.Draw(old, dst, background, image.Point{}, draw.Src)
		draw.Draw(packed, dst, background, image.Point{}, draw.Src)
		draw.Draw(old, dst, grid, image.Pt(Cell*i, 0), draw.Over)
		a.Draw(packed, image.Point{}, i)
		if !bytes.Equal(old.Pix, packed.Pix) {
			t.Fatalf("frame %d changed when composited", i)
		}
	}
}

func TestGroupedAtlasDecodesOnlyTheRequestedActor(t *testing.T) {
	grid := image.NewNRGBA(image.Rect(0, 0, Cell*Columns, 2*Cell))
	grid.SetNRGBA(17, 29, color.NRGBA{R: 210, A: 119})
	grid.SetNRGBA(33, Cell+41, color.NRGBA{G: 200, A: 175})
	var buf bytes.Buffer
	if err := EncodeGroup(&buf, grid, 2); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		part, err := Part(buf.Bytes(), i)
		if err != nil {
			t.Fatal(err)
		}
		atlas, err := Open(part)
		if err != nil || atlas.Len() != Columns {
			t.Fatalf("provider %d: %d frames, error %v", i, atlas.Len(), err)
		}
		bounds := image.Rect(0, 0, Cell, Cell)
		want, got := image.NewRGBA(bounds), image.NewRGBA(bounds)
		draw.Draw(want, bounds, grid, image.Pt(0, Cell*i), draw.Over)
		atlas.Draw(got, image.Point{}, 0)
		if !bytes.Equal(want.Pix, got.Pix) {
			t.Fatalf("provider %d did not preserve its pixels and alpha", i)
		}
	}
	if _, err := Part(buf.Bytes(), 2); err == nil {
		t.Fatal("unexpected provider accepted")
	}
}
