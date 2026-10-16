package asset

import (
	"image"
	"io"
	"net/http"

	// Decoders for image.DecodeConfig: it reads only the header.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// imageType describes one allowed file type.
type imageType struct {
	mime   string
	ext    string
	format string // the name image.DecodeConfig reports
}

var imageTypes = []imageType{
	{"image/png", "png", "png"},
	{"image/jpeg", "jpg", "jpeg"},
	{"image/webp", "webp", "webp"},
	{"image/gif", "gif", "gif"},
}

func typeByMIME(mime string) (imageType, bool) {
	for _, t := range imageTypes {
		if t.mime == mime {
			return t, true
		}
	}
	return imageType{}, false
}

// detectType finds the type by the content of the first bytes; the file name
// and any declared type are ignored.
func detectType(head []byte) (imageType, bool) {
	return typeByMIME(http.DetectContentType(head))
}

// imageSize reads the dimensions from the header of f, starting at its
// beginning, without decoding the pixels. ok is false when the header is
// unreadable or is not of the expected type.
func imageSize(f io.ReadSeeker, t imageType) (width, height int, ok bool) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, false
	}
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || format != t.format {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}
