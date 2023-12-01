package christmasd

import (
	"image"

	"dev.acmcsuf.com/christmas/lib/leddraw"
)

// LEDController is a controller for LEDs.
type LEDController interface {
	// LEDs returns the LED strip.
	LEDs() leddraw.LEDStrip
	// SetLEDs sets the LED strip.
	SetLEDs(strip leddraw.LEDStrip) error
	// ImageSize returns the size of the image.
	ImageSize() (w, h int)
	// DrawImage draws an image to the LED strip.
	DrawImage(img *image.RGBA) error
}
