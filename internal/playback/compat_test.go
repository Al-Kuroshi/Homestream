package playback

import "testing"

func TestIsDirectPlayCompatible(t *testing.T) {
	tests := []struct {
		name                              string
		videoCodec, audioCodec, container string
		want                              bool
	}{
		{"h264/aac/mp4 is compatible", "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2", true},
		{"h264/mp3/mp4 is compatible", "h264", "mp3", "mov,mp4,m4a,3gp,3g2,mj2", true},
		{"h265 video is not compatible", "hevc", "aac", "mov,mp4,m4a,3gp,3g2,mj2", false},
		{"vp9 video is not compatible", "vp9", "aac", "mov,mp4,m4a,3gp,3g2,mj2", false},
		{"ac3 audio is not compatible", "h264", "ac3", "mov,mp4,m4a,3gp,3g2,mj2", false},
		{"mkv container is not compatible even with compatible codecs", "h264", "aac", "matroska,webm", false},
		{"avi container is not compatible", "h264", "aac", "avi", false},
		{"empty codec info is not compatible", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDirectPlayCompatible(tt.videoCodec, tt.audioCodec, tt.container)
			if got != tt.want {
				t.Errorf("IsDirectPlayCompatible(%q, %q, %q) = %v, want %v", tt.videoCodec, tt.audioCodec, tt.container, got, tt.want)
			}
		})
	}
}
