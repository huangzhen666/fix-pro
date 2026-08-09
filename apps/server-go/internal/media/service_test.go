package media

import "testing"

func TestDetectAllowedSignatures(t *testing.T) {
	tests := []struct {
		name, wantType string
		data           []byte
	}{
		{"jpeg", "IMAGE", []byte{0xff, 0xd8, 0xff}},
		{"png", "IMAGE", []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}},
		{"webp", "IMAGE", []byte("RIFFxxxxWEBP")},
		{"mp4", "VIDEO", []byte("xxxxftypxxxx")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got, _ := detect(tt.data)
			if got != tt.wantType {
				t.Fatalf("type=%q want=%q", got, tt.wantType)
			}
		})
	}
}

func TestDetectRejectsExtensionOnly(t *testing.T) {
	if contentType, _, _ := detect([]byte("<script>alert(1)</script>")); contentType != "" {
		t.Fatalf("unexpected content type %q", contentType)
	}
}
