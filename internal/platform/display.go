package platform

// WorkArea uses macOS logical points, Y-down from the primary display's top
// edge. Negative origins are preserved for screens above or left of it.
type WorkArea struct {
	X, Y, Width, Height int
}
