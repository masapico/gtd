package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// SVGTemplate creates the crisp SVG favicon with dark-mode adaptability.
const SVGContent = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
  <defs>
    <linearGradient id="bgGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#007bc0"/>
      <stop offset="100%" stop-color="#004c78"/>
    </linearGradient>
    <linearGradient id="bgGradDark" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#0096e6"/>
      <stop offset="100%" stop-color="#005d96"/>
    </linearGradient>
    <filter id="shadow" x="-10%" y="-10%" width="120%" height="120%">
      <feDropShadow dx="0" dy="8" stdDeviation="12" flood-color="#00182b" flood-opacity="0.35"/>
    </filter>
  </defs>
  <style>
    .bg { fill: url(#bgGrad); }
    .border { stroke: rgba(255, 255, 255, 0.22); }
    .check { stroke: #ffffff; }
    .sparkle { fill: #7cd4fd; }
    @media (prefers-color-scheme: dark) {
      .bg { fill: url(#bgGradDark); }
      .border { stroke: rgba(255, 255, 255, 0.3); }
      .sparkle { fill: #a3e5ff; }
    }
  </style>
  
  <!-- Squircle Base -->
  <rect class="bg" width="512" height="512" rx="116" />
  
  <!-- Subtle Highlight Edge -->
  <rect class="border" x="8" y="8" width="496" height="496" rx="108" fill="none" stroke-width="10"/>

  <!-- Productivity Sparkle Dot -->
  <circle class="sparkle" cx="396" cy="116" r="22" filter="url(#shadow)" />

  <!-- Bold Crisp Checkmark -->
  <path class="check" d="M 136 264 L 216 348 L 376 168" 
        fill="none" 
        stroke-width="58" 
        stroke-linecap="round" 
        stroke-linejoin="round" 
        filter="url(#shadow)" />
</svg>
`

// Vec2 represents a 2D float point
type Vec2 struct {
	X, Y float64
}

// Distance from point p to line segment [a, b]
func distSegment(p, a, b Vec2) float64 {
	pa := Vec2{p.X - a.X, p.Y - a.Y}
	ba := Vec2{b.X - a.X, b.Y - a.Y}
	h := (pa.X*ba.X + pa.Y*ba.Y) / (ba.X*ba.X + ba.Y*ba.Y)
	if h < 0 {
		h = 0
	} else if h > 1 {
		h = 1
	}
	dx := pa.X - ba.X*h
	dy := pa.Y - ba.Y*h
	return math.Sqrt(dx*dx + dy*dy)
}

// Signed distance to rounded rectangle centered at (cx, cy)
func distRoundedRect(p, center, size Vec2, radius float64) float64 {
	dx := math.Abs(p.X-center.X) - (size.X/2 - radius)
	dy := math.Abs(p.Y-center.Y) - (size.Y/2 - radius)

	ax := math.Max(dx, 0)
	ay := math.Max(dy, 0)
	outsideDist := math.Sqrt(ax*ax+ay*ay) - radius

	insideDist := math.Min(math.Max(dx, dy), 0) - radius
	if dx > 0 || dy > 0 {
		return outsideDist
	}
	return insideDist
}

// RenderBitmap generates a rendered RGBA image at specified resolution
func RenderBitmap(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / 512.0

	center := Vec2{float64(size) / 2, float64(size) / 2}
	boxSize := Vec2{float64(size), float64(size)}
	radius := 116.0 * scale

	// Checkmark geometry scaled
	p1 := Vec2{136.0 * scale, 264.0 * scale}
	p2 := Vec2{216.0 * scale, 348.0 * scale}
	p3 := Vec2{376.0 * scale, 168.0 * scale}
	strokeRadius := (58.0 * scale) / 2.0

	// Sparkle dot geometry
	dotCenter := Vec2{396.0 * scale, 116.0 * scale}
	dotRadius := 22.0 * scale

	// Supersampling factor for crystal-clear antialiasing
	samples := 4
	step := 1.0 / float64(samples)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var rSum, gSum, bSum, aSum float64

			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)*step
					py := float64(y) + (float64(sy)+0.5)*step
					p := Vec2{px, py}

					dBox := distRoundedRect(p, center, boxSize, radius)
					if dBox > 0 {
						continue // Outside squircle
					}

					// Base gradient: #007bc0 (top) to #004c78 (bottom)
					tGrad := py / float64(size)
					if tGrad < 0 {
						tGrad = 0
					} else if tGrad > 1 {
						tGrad = 1
					}
					bgR := 0.0 + (0.0-0.0)*tGrad
					bgG := 123.0 + (76.0-123.0)*tGrad
					bgB := 192.0 + (120.0-192.0)*tGrad
					bgA := 255.0

					// Checkmark distance
					dCheck1 := distSegment(p, p1, p2) - strokeRadius
					dCheck2 := distSegment(p, p2, p3) - strokeRadius
					dCheck := math.Min(dCheck1, dCheck2)

					// Sparkle dot distance
					dDot := math.Sqrt((p.X-dotCenter.X)*(p.X-dotCenter.X)+(p.Y-dotCenter.Y)*(p.Y-dotCenter.Y)) - dotRadius

					// Composite Checkmark (#ffffff)
					if dCheck <= 0 {
						bgR, bgG, bgB, bgA = 255.0, 255.0, 255.0, 255.0
					} else if dCheck < 1.0 {
						// Smooth blend edge
						alpha := 1.0 - dCheck
						bgR = bgR*(1.0-alpha) + 255.0*alpha
						bgG = bgG*(1.0-alpha) + 255.0*alpha
						bgB = bgB*(1.0-alpha) + 255.0*alpha
					}

					// Composite Sparkle Dot (#7cd4fd) if checkmark isn't covering it
					if dCheck > 0 {
						if dDot <= 0 {
							bgR, bgG, bgB = 124.0, 212.0, 253.0
						} else if dDot < 1.0 {
							alpha := 1.0 - dDot
							bgR = bgR*(1.0-alpha) + 124.0*alpha
							bgG = bgG*(1.0-alpha) + 212.0*alpha
							bgB = bgB*(1.0-alpha) + 253.0*alpha
						}
					}

					// Squircle outer edge smoothing
					edgeAlpha := 1.0
					if dBox > -1.0 {
						edgeAlpha = -dBox
						if edgeAlpha < 0 {
							edgeAlpha = 0
						}
					}

					rSum += bgR * edgeAlpha
					gSum += bgG * edgeAlpha
					bSum += bgB * edgeAlpha
					aSum += bgA * edgeAlpha
				}
			}

			totalSamples := float64(samples * samples)
			a := aSum / totalSamples
			if a > 0 {
				r := (rSum / totalSamples)
				g := (gSum / totalSamples)
				b := (bSum / totalSamples)
				img.SetRGBA(x, y, color.RGBA{
					R: uint8(math.Round(r)),
					G: uint8(math.Round(g)),
					B: uint8(math.Round(b)),
					A: uint8(math.Round(a)),
				})
			} else {
				img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}
	return img
}

// EncodeICO writes multiple PNG images into a standard Windows .ico file
func EncodeICO(images []*image.RGBA) ([]byte, error) {
	var pngBuffers [][]byte
	for _, img := range images {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		pngBuffers = append(pngBuffers, buf.Bytes())
	}

	buf := new(bytes.Buffer)

	// ICONDIR header: 6 bytes
	// Reserved: 0 (2 bytes)
	// Type: 1 (2 bytes, 1 for Icon, 2 for Cursor)
	// Count: number of images (2 bytes)
	binary.Write(buf, binary.LittleEndian, uint16(0))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(len(images)))

	// Calculate header + entries size: 6 + 16 * count
	offset := uint32(6 + 16*len(images))

	// Write ICONDIRENTRY (16 bytes per image)
	for i, img := range images {
		bounds := img.Bounds()
		w := bounds.Dx()
		h := bounds.Dy()

		bWidth := byte(w)
		if w >= 256 {
			bWidth = 0
		}
		bHeight := byte(h)
		if h >= 256 {
			bHeight = 0
		}

		buf.WriteByte(bWidth)           // Width
		buf.WriteByte(bHeight)          // Height
		buf.WriteByte(0)                // Color count
		buf.WriteByte(0)                // Reserved
		binary.Write(buf, binary.LittleEndian, uint16(1))  // Color planes
		binary.Write(buf, binary.LittleEndian, uint16(32)) // Bit count (32 bits per pixel)
		binary.Write(buf, binary.LittleEndian, uint32(len(pngBuffers[i]))) // Size of image data
		binary.Write(buf, binary.LittleEndian, offset)                      // Offset of image data

		offset += uint32(len(pngBuffers[i]))
	}

	// Write image PNG data
	for _, pngData := range pngBuffers {
		buf.Write(pngData)
	}

	return buf.Bytes(), nil
}

func main() {
	outDir := "static"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("🎨 Generating GTD Favicon assets in", outDir, "...")

	// 1. Write SVG
	svgPath := filepath.Join(outDir, "favicon.svg")
	if err := os.WriteFile(svgPath, []byte(SVGContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write favicon.svg: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✅ Created favicon.svg (Vector with Dark Mode support)")

	// 2. Render PNGs
	sizes := []int{16, 32, 48, 180}
	rendered := make(map[int]*image.RGBA)

	for _, size := range sizes {
		img := RenderBitmap(size)
		rendered[size] = img

		// Write specific PNG files
		var filename string
		switch size {
		case 16:
			filename = "favicon-16x16.png"
		case 32:
			filename = "favicon-32x32.png"
		case 180:
			filename = "apple-touch-icon.png"
		}

		if filename != "" {
			p := filepath.Join(outDir, filename)
			f, err := os.Create(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create %s: %v\n", filename, err)
				os.Exit(1)
			}
			if err := png.Encode(f, img); err != nil {
				f.Close()
				fmt.Fprintf(os.Stderr, "Failed to encode %s: %v\n", filename, err)
				os.Exit(1)
			}
			f.Close()
			fmt.Printf("  ✅ Created %s (%dx%d PNG)\n", filename, size, size)
		}
	}

	// 3. Multi-resolution ICO (16x16, 32x32, 48x48)
	icoImages := []*image.RGBA{
		rendered[16],
		rendered[32],
		rendered[48],
	}
	icoBytes, err := EncodeICO(icoImages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode favicon.ico: %v\n", err)
		os.Exit(1)
	}

	icoPath := filepath.Join(outDir, "favicon.ico")
	if err := os.WriteFile(icoPath, icoBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write favicon.ico: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✅ Created favicon.ico (Multi-size 16/32/48 ICO)")

	fmt.Println("🎉 Favicon generation completed successfully!")
}

