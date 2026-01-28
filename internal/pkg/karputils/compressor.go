package karputils

import (
	"io"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zstd"
	lz4 "github.com/pierrec/lz4/v4"
	"google.golang.org/grpc/encoding"
)

// Snappy
type SnappyCompressor struct{}

func (s SnappyCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return snappy.NewBufferedWriter(w), nil
}

func (s SnappyCompressor) Decompress(r io.Reader) (io.Reader, error) {
	return snappy.NewReader(r), nil
}

func (s SnappyCompressor) Name() string { return "snappy" }

// LZ4
type LZ4Compressor struct{}

func (l LZ4Compressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return lz4.NewWriter(w), nil
}

func (l LZ4Compressor) Decompress(r io.Reader) (io.Reader, error) {
	return lz4.NewReader(r), nil
}

func (l LZ4Compressor) Name() string { return "lz4" }

// Gzip
type GzipCompressor struct {
	level int
}

func NewGzipCompressor(level int) *GzipCompressor {
	return &GzipCompressor{level: level}
}

func (g *GzipCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return gzip.NewWriterLevel(w, g.level)
}

func (g *GzipCompressor) Decompress(r io.Reader) (io.Reader, error) {
	return gzip.NewReader(r)
}

func (g *GzipCompressor) Name() string { return "gzip" }

// Zstd
type ZstdCompressor struct {
	level zstd.EncoderLevel
}

func NewZstdCompressor(level zstd.EncoderLevel) *ZstdCompressor {
	return &ZstdCompressor{level: level}
}

func (z *ZstdCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return zstd.NewWriter(w, zstd.WithEncoderLevel(z.level))
}

func (z *ZstdCompressor) Decompress(r io.Reader) (io.Reader, error) {
	return zstd.NewReader(r)
}

func (z *ZstdCompressor) Name() string { return "zstd" }

func RegisterCompressors() {
	encoding.RegisterCompressor(SnappyCompressor{})
	encoding.RegisterCompressor(LZ4Compressor{})
	encoding.RegisterCompressor(NewGzipCompressor(gzip.BestSpeed))
	encoding.RegisterCompressor(NewZstdCompressor(zstd.SpeedFastest))
}
