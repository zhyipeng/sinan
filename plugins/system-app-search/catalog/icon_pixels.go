package catalog

import "image"

const applicationIconSize = 64

// windowsIconPixels reconstructs straight RGBA from two offscreen BGRA renders.
// Kept independent of Win32 so transparency conversion is tested on every OS.
func windowsIconPixels(black, white []byte) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, applicationIconSize, applicationIconSize))
	for i := 0; i < len(img.Pix); i += 4 {
		alpha := 255 - max(0, int(white[i])-int(black[i]), int(white[i+1])-int(black[i+1]), int(white[i+2])-int(black[i+2]))
		if alpha == 0 {
			continue
		}
		for channel := 0; channel < 3; channel++ {
			img.Pix[i+channel] = byte(min(255, (int(black[i+2-channel])*255+alpha/2)/alpha))
		}
		img.Pix[i+3] = byte(alpha)
	}
	return img
}
