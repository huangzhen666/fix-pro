package media

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAvatarValidation(t *testing.T) {
	webp, e := os.ReadFile("testdata/avatar.webp")
	if e != nil {
		t.Fatal(e)
	}
	var jpegData bytes.Buffer
	jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil)
	var valid bytes.Buffer
	png.Encode(&valid, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	for _, tc := range []struct {
		name string
		b    []byte
		ok   bool
	}{{"jpeg", jpegData.Bytes(), true}, {"webp", webp, true}, {"png", valid.Bytes(), true}, {"spoof", []byte("not an image"), false}, {"corrupt", []byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10}, false}, {"oversize", bytes.Repeat([]byte{1}, (5<<20)+1), false}} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			writer := multipart.NewWriter(&b)
			part, _ := writer.CreateFormFile("file", "avatar.png")
			part.Write(tc.b)
			writer.Close()
			r := httptest.NewRequest("POST", "/", &b)
			r.Header.Set("Content-Type", writer.FormDataContentType())
			if e := r.ParseMultipartForm(1 << 20); e != nil {
				t.Fatal(e)
			}
			defer r.MultipartForm.RemoveAll()
			f, fh, _ := r.FormFile("file")
			f.Close()
			e := ValidateAvatar(fh)
			if (e == nil) != tc.ok {
				t.Fatal("unexpected validation", e)
			}
		})
	}
}
