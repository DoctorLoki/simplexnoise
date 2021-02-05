package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
)

func main() {
	listenAddr := flag.String("listen-addr", ":8080", "address to listen for tile requests on")
	flag.Parse()
	log.Fatal(http.ListenAndServe(*listenAddr, tileServer()))
}

func tileServer() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coords, err := extractTileCoords(r.URL.Path)
		if err != nil {
			log.Println(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		tile := renderTile(coords)
		if err := png.Encode(w, tile); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error: " + err.Error()))
			return
		}
	})
}

type TileCoords struct {
	Z, X, Y int
}

var pathRegex = regexp.MustCompile(`^/(\d+)/(\d+)/(\d+)\.png$`)

func extractTileCoords(path string) (TileCoords, error) {
	matches := pathRegex.FindStringSubmatch(path)
	if len(matches) != 4 {
		return TileCoords{}, fmt.Errorf("not enough matches, got %d", len(matches))
	}

	var coords TileCoords
	var err error
	coords.Z, err = strconv.Atoi(matches[1])
	if err != nil {
		return TileCoords{}, fmt.Errorf("extracting z: %v", err)
	}
	coords.X, err = strconv.Atoi(matches[2])
	if err != nil {
		return TileCoords{}, fmt.Errorf("extracting x: %v", err)
	}
	coords.Y, err = strconv.Atoi(matches[3])
	if err != nil {
		return TileCoords{}, fmt.Errorf("extracting y: %v", err)
	}

	max := 1 << uint(coords.Z)
	if coords.X < 0 || coords.X >= max || coords.Y < 0 || coords.Y >= max {
		return TileCoords{}, fmt.Errorf("invalid tile coordinates: %v", coords)
	}

	return coords, nil
}

func renderTile(coords TileCoords) image.Image {
	const tileSize = 256
	extent := tileExtent(coords)
	tile := image.NewRGBA(image.Rect(0, 0, tileSize, tileSize))
	for y := 0; y < tileSize; y++ {
		for x := 0; x < tileSize; x++ {
			c := Vector{
				extent.Min.X + (extent.Max.X-extent.Min.X)*float64(x)/tileSize,
				extent.Min.Y + (extent.Max.X-extent.Min.X)*float64(y)/tileSize,
			}
			value := simplexTorus(c, coords)

			// Draw the pixel.
			colour := colouriseByValue(value)
			tile.SetRGBA(x, y, colour)
		}
	}
	return tile
}

func colouriseByValue(value float64) color.RGBA {
var r, g, b float64
	if value < -0.1 {
		// Dark blue water
		r = 0.0
		g = 0.0
		b = 0.4
	} else if value < 0.2 {
		// Blue water
		maximum := 0.0
		r = 0.1 + (maximum + value)
		g = 0.1 + (maximum + value)
		b = 0.5 + (maximum + value)
	} else if value < 0.201 {
		// Yellow sand
		r = 500 * (0.202 - value)
		g = 500 * (0.202 - value)
		b = 250 * (0.202 - value)
	} else if value < 0.40 {
		// Grasslands
		maximum := 0.40 + 0.20
		r = 1.2 * (maximum - value)
		g = 1.6 * (maximum - value)
		b = 0.8 * (maximum - value)
	} else if value < 0.60 {
		// Greenery
		maximum := 0.60 + 0.30
		r = 0.2 * (maximum - value)
		g = 0.8 * (maximum - value)
		b = 0.1 * (maximum - value)
	} else if value < 0.90 {
		// Mountains
		maximum := 0.90
		minimum := 0.10
		diff := maximum - minimum
		r = 0.8 / diff * (value - minimum)
		g = 0.7 / diff * (value - minimum)
		b = 0.6 / diff * (value - minimum)
	} else if value < 1.2 {
		// Pale snow
		r = 0.8 * value
		g = 0.8 * value
		b = 0.8 * value
	} else {
		// White snow
		r = 1.0
		g = 1.0
		b = 1.0
	}
	return color.RGBA{uint8(r * 0xff), uint8(g * 0xff), uint8(b * 0xff), 0xff}
}

func colourise(value float64) color.RGBA {
	value *= 215 // artistically chosen multiplier
	return hslToRGB(math.Mod(value+360, 360), 0.5, 0.5)
}

func hslToRGB(hue, saturation, lightness float64) color.RGBA {
	if hue < 0 || hue > 360 {
		panic("hue must be from 0 to 360")
	}
	if saturation < 0 || saturation > 1 {
		panic("saturation must be between 0 and 1")
	}
	if lightness < 0 || lightness > 1 {
		panic("lightness must be between 0 and 1")
	}

	c := (1 - math.Abs(2*lightness-1)) * saturation // chroma
	hueAdj := hue / 60
	x := c * (1 - math.Abs(math.Mod(hueAdj, 2)-1))

	var r, g, b float64
	switch {
	case hueAdj <= 1:
		r, g, b = c, x, 0
	case hueAdj <= 2:
		r, g, b = x, c, 0
	case hueAdj <= 3:
		r, g, b = 0, c, x
	case hueAdj <= 4:
		r, g, b = 0, x, c
	case hueAdj <= 5:
		r, g, b = x, 0, c
	case hueAdj <= 6:
		r, g, b = c, 0, x
	default:
		panic(false)
	}

	m := lightness - 0.5*c
	r += m
	g += m
	b += m

	if r < 0 || r > 1.0 {
		panic(r)
	}
	if g < 0 || g > 1.0 {
		panic(g)
	}
	if b < 0 || b > 1.0 {
		panic(b)
	}

	return color.RGBA{uint8(r * 0xff), uint8(g * 0xff), uint8(b * 0xff), 0xff}
}

// mandelbrot returns 0 for numbers in the mandelbrot set, or the smoothed
// iteration count before escape has been confirmed.
func mandelbrot(c Vector) float64 {
	const maxIter = 1000
	var z Vector
	iterate := func() {
		z = Vector{z.X*z.X - z.Y*z.Y + c.X, 2*z.X*z.Y + c.Y}
	}
	for i := 0; i < maxIter; i++ {
		iterate()
		if z.X*z.X+z.Y*z.Y > 4 {
			iterate()
			iterate()
			modulus := math.Sqrt(z.X*z.X + z.Y*z.Y)
			return float64(i) - math.Log(math.Log(modulus))/math.Log(2)
		}
	}
	return 0
}

func simplexSphere(c Vector, coords TileCoords) float64 {
	// Seamless noise in both directions.
	//
	// Because the vector c runs from -2.0 to +2.0 at zoom level 0
	// we divide by 2 to obtain a value from -1.0 to +1.0 then
	// multiply by Pi to obtain a value from -Pi to +Pi as an
	// appropriate input for Cos or Sin.
	//
	// Wrapping X around a circle allows the noise to be seamless
	// in that direction.
	//r := math.Cos(c.Y / 4 * math.Pi)
	nx := math.Cos(c.X / 2 * math.Pi)
	nz := math.Sin(c.X / 2 * math.Pi)
	ny := c.Y / 2

	return Noise3(nx, ny, nz)
}

func simplexTorus(c Vector, coords TileCoords) float64 {
	// Seamless noise in both directions.
	//
	// Because the vector c runs from -2.0 to +2.0 at zoom level 0
	// we divide by 2 to obtain a value from -1.0 to +1.0 then
	// multiply by Pi to obtain a value from -Pi to +Pi as an
	// appropriate input for Cos or Sin.
	//
	// Wrapping X around a circle and Y around a different circle
	// (using 4D noise) allows the noise to be seamless in both
	// directions.
	nx := math.Cos(c.X / 2 * math.Pi)
	nz := math.Sin(c.X / 2 * math.Pi)
	ny := math.Cos(c.Y / 2 * math.Pi)
	nw := math.Sin(c.Y / 2 * math.Pi)

	value := Noise4(nx, ny, nz, nw)

	scale := float64(1)
	for z := 1; z <= coords.Z+16; z++ {
		scale *= 2
		nx := math.Cos(c.X / 2 * scale * math.Pi)
		nz := math.Sin(c.X / 2 * scale * math.Pi)
		ny := math.Cos(c.Y / 2 * scale * math.Pi)
		nw := math.Sin(c.Y / 2 * scale * math.Pi)

		value += Noise4(nx, ny, nz, nw) / scale
	}

	return value
}

type Vector struct {
	X, Y float64
}

func (v Vector) Sub(u Vector) Vector {
	return Vector{v.X - u.X, v.Y - u.Y}
}

func (v Vector) Scale(f float64) Vector {
	return Vector{v.X * f, v.Y * f}
}

type Extent struct {
	Min, Max Vector
}

func tileExtent(coords TileCoords) Extent {
	extent := Extent{
		Min: Vector{
			float64(coords.X) / float64(uint(1)<<uint(coords.Z)),
			float64(coords.Y) / float64(uint(1)<<uint(coords.Z)),
		},
		Max: Vector{
			float64(coords.X+1) / float64(uint(1)<<uint(coords.Z)),
			float64(coords.Y+1) / float64(uint(1)<<uint(coords.Z)),
		},
	}

	extent.Min = extent.Min.Sub(Vector{0.5, 0.5})
	extent.Max = extent.Max.Sub(Vector{0.5, 0.5})
	extent.Min = extent.Min.Scale(4)
	extent.Max = extent.Max.Scale(4)
	return extent
}
