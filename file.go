package goappleads

import (
	"io"
	"iter"
	"log/slog"
	"os"
)

// FromFile parses CSV file at path using parse and yields rows.
// Errors opening the file are logged and iteration stops.
func FromFile[T any](path string, parse func(io.Reader) iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		f, err := os.Open(path)
		if err != nil {
			slog.Error("failed to open file", "path", path, "error", err)
			return
		}
		defer f.Close()

		for v := range parse(f) {
			if !yield(v) {
				return
			}
		}
	}
}
