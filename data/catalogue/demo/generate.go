//go:build ignore

// Regenerate the geometric pixel-art examples: go run data/catalogue/demo/generate.go
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

var navy = color.NRGBA{18, 29, 49, 255}
var sea = color.NRGBA{26, 68, 89, 255}
var stone = color.NRGBA{117, 137, 145, 255}
var light = color.NRGBA{224, 210, 156, 255}
var gold = color.NRGBA{232, 172, 67, 255}

func box(img *image.NRGBA, x, y, w, h int, c color.NRGBA) {
	draw.Draw(img, image.Rect(x, y, x+w, y+h), image.NewUniform(c), image.Point{}, draw.Src)
}
func save(name string, img *image.NRGBA) {
	f, err := os.Create(filepath.Join("data/catalogue/demo/images", name+".png"))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
func main() {
	if err := os.MkdirAll("data/catalogue/demo/images", 0755); err != nil {
		panic(err)
	}
	quay := image.NewNRGBA(image.Rect(0, 0, 160, 96))
	box(quay, 0, 0, 160, 96, navy)
	for _, r := range [][4]int{{8, 14, 28, 3}, {82, 8, 35, 3}, {120, 22, 30, 3}} {
		box(quay, r[0], r[1], r[2], r[3], color.NRGBA{58, 76, 94, 255})
	}
	box(quay, 0, 48, 160, 48, sea)
	for y := 52; y < 90; y += 7 {
		for x := (y % 4) * 7; x < 160; x += 24 {
			box(quay, x, y, 12, 1, color.NRGBA{69, 113, 128, 255})
		}
	}
	box(quay, 110, 23, 12, 37, stone)
	box(quay, 107, 20, 18, 5, light)
	box(quay, 109, 14, 14, 6, gold)
	box(quay, 112, 10, 8, 4, navy)
	box(quay, 115, 28, 3, 5, navy)
	box(quay, 115, 39, 3, 5, navy)
	box(quay, 0, 72, 93, 24, color.NRGBA{80, 71, 60, 255})
	box(quay, 0, 70, 94, 3, stone)
	for x := 4; x < 90; x += 14 {
		box(quay, x, 73, 1, 23, color.NRGBA{46, 46, 46, 255})
	}
	box(quay, 12, 61, 4, 14, light)
	box(quay, 78, 61, 4, 14, light)
	save("quay", quay)
	hall := image.NewNRGBA(image.Rect(0, 0, 160, 96))
	box(hall, 0, 0, 160, 96, navy)
	box(hall, 0, 76, 160, 20, color.NRGBA{66, 66, 67, 255})
	for y := 0; y < 76; y += 12 {
		box(hall, 0, y, 160, 1, color.NRGBA{52, 64, 77, 255})
		for x := (y / 12 % 2) * 15; x < 160; x += 30 {
			box(hall, x, y, 1, 12, color.NRGBA{52, 64, 77, 255})
		}
	}
	box(hall, 78, 19, 40, 57, stone)
	box(hall, 82, 22, 32, 54, color.NRGBA{116, 82, 53, 255})
	box(hall, 108, 48, 2, 3, gold)
	for n := 0; n < 6; n++ {
		box(hall, 18+n*5, 70-n*9, 28, 6, color.NRGBA{104, 118, 125, 255})
	}
	save("hall", hall)
	workshop := image.NewNRGBA(image.Rect(0, 0, 160, 96))
	box(workshop, 0, 0, 160, 96, color.NRGBA{53, 55, 58, 255})
	box(workshop, 10, 10, 35, 27, stone)
	box(workshop, 13, 13, 29, 21, sea)
	box(workshop, 26, 10, 3, 27, stone)
	box(workshop, 10, 22, 35, 3, stone)
	box(workshop, 22, 59, 119, 7, color.NRGBA{148, 105, 61, 255})
	box(workshop, 26, 66, 6, 30, color.NRGBA{91, 69, 48, 255})
	box(workshop, 131, 66, 6, 30, color.NRGBA{91, 69, 48, 255})
	for x := 70; x < 130; x += 13 {
		box(workshop, x, 22, 3, 21, stone)
		box(workshop, x-3, 20, 9, 5, stone)
	}
	save("workshop", workshop)
	lantern := image.NewNRGBA(image.Rect(0, 0, 160, 96))
	box(lantern, 0, 0, 160, 96, navy)
	box(lantern, 0, 22, 160, 45, sea)
	for x := 0; x < 160; x += 39 {
		box(lantern, x, 0, 3, 72, stone)
	}
	box(lantern, 0, 65, 160, 5, stone)
	box(lantern, 0, 85, 160, 11, color.NRGBA{76, 81, 87, 255})
	save("lantern", lantern)
	keeper := image.NewNRGBA(image.Rect(0, 0, 24, 64))
	box(keeper, 8, 3, 10, 12, color.NRGBA{197, 152, 112, 255})
	box(keeper, 6, 2, 14, 4, navy)
	box(keeper, 9, 0, 10, 3, navy)
	box(keeper, 6, 16, 15, 28, color.NRGBA{54, 104, 113, 255})
	box(keeper, 3, 18, 4, 24, color.NRGBA{54, 104, 113, 255})
	box(keeper, 7, 44, 5, 17, navy)
	box(keeper, 16, 44, 5, 17, navy)
	box(keeper, 5, 60, 9, 4, stone)
	box(keeper, 15, 60, 9, 4, stone)
	save("keeper", keeper)
	monster := image.NewNRGBA(image.Rect(0, 0, 48, 24))
	box(monster, 12, 8, 24, 13, color.NRGBA{152, 68, 63, 255})
	box(monster, 7, 5, 8, 10, color.NRGBA{184, 85, 70, 255})
	box(monster, 33, 5, 8, 10, color.NRGBA{184, 85, 70, 255})
	box(monster, 4, 1, 7, 9, color.NRGBA{184, 85, 70, 255})
	box(monster, 38, 1, 7, 9, color.NRGBA{184, 85, 70, 255})
	for _, x := range []int{14, 29} {
		box(monster, x, 2, 5, 7, gold)
		box(monster, x+1, 3, 2, 2, navy)
	}
	for _, x := range []int{10, 20, 30} {
		box(monster, x, 19, 8, 4, color.NRGBA{152, 68, 63, 255})
	}
	save("reef_crab", monster)
	key := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	box(key, 1, 2, 11, 11, gold)
	box(key, 4, 5, 5, 5, color.NRGBA{})
	box(key, 11, 6, 19, 4, gold)
	box(key, 23, 9, 3, 5, gold)
	box(key, 28, 9, 3, 5, gold)
	save("key", key)
	battery := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	box(battery, 4, 4, 17, 18, stone)
	box(battery, 7, 1, 4, 3, gold)
	box(battery, 15, 1, 4, 3, gold)
	box(battery, 8, 9, 9, 7, navy)
	box(battery, 10, 10, 5, 2, gold)
	save("battery", battery)
	beacon := image.NewNRGBA(image.Rect(0, 0, 32, 48))
	box(beacon, 6, 42, 23, 6, stone)
	box(beacon, 13, 24, 9, 18, stone)
	box(beacon, 5, 5, 25, 21, stone)
	box(beacon, 8, 8, 19, 14, gold)
	box(beacon, 2, 1, 30, 5, navy)
	box(beacon, 16, 8, 3, 14, light)
	save("beacon", beacon)
}
