package ergo

import "io"

// RenderOptions contains presentation capabilities supplied by the CLI
// boundary. A zero width selects the stable 80-column fallback.
type RenderOptions struct {
	Writer io.Writer
	Width  int
	Color  bool
}

func (options RenderOptions) writer() io.Writer {
	if options.Writer == nil {
		return io.Discard
	}
	return options.Writer
}

func (options RenderOptions) width() int {
	if options.Width <= 0 {
		return 80
	}
	return options.Width
}
