package platform

// WorkArea uses platform logical coordinates with Y increasing downward.
// Negative desktop origins are preserved. Win32 pixel rectangles must first
// pass through Wails' screen conversion instead of being returned directly.
type WorkArea struct {
	X, Y, Width, Height int
}
