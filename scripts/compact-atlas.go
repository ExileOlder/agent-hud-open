// compact-atlas converts recorded frames to the native dock's pixel grid.
package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"

	"github.com/ExileOlder/agent-hud-open/internal/markatlas"

	imagedraw "golang.org/x/image/draw"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: compact-atlas INPUT.png OUTPUT.atlas")
		os.Exit(2)
	}
	if err := compact(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func compact(input, output string) error {
	in, err := os.Open(input)
	if err != nil {
		return err
	}
	defer in.Close()
	source, err := png.Decode(in)
	if err != nil {
		return err
	}
	// Match the native bar's 29px mark radius and the recorder's 8 columns.
	const target, columns = markatlas.Cell, markatlas.Columns
	bounds := source.Bounds()
	cell := bounds.Dx() / columns
	if (cell != target && cell != 64 && cell != 80) || cell*columns != bounds.Dx() || bounds.Dy()%cell != 0 {
		return fmt.Errorf("unexpected recorded atlas size: %v", bounds.Size())
	}
	rows := bounds.Dy() / cell
	if cell != target {
		resized := image.NewRGBA(image.Rect(0, 0, columns*target, rows*target))
		scale := imagedraw.CatmullRom.NewScaler(target, target, cell, cell)
		for y := range rows {
			for x := range columns {
				scale.Scale(resized, image.Rect(x*target, y*target, (x+1)*target, (y+1)*target),
					source, image.Rect(x*cell, y*cell, (x+1)*cell, (y+1)*cell), draw.Over, nil)
			}
		}
		source = resized
	}
	f, err := os.Create(output)
	if err != nil {
		return err
	}
	err = markatlas.Encode(f, source)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
