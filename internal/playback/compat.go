package playback

// mp4Container is the exact ffprobe format_name string for the mp4 family
// (mov,mp4,m4a,3gp,3g2,mj2) — the only container this MVP treats as
// direct-play compatible. Even h264/aac content inside an mkv/avi/webm
// container is transcoded, since browsers generally cannot demux those
// containers via a plain <video> element regardless of what's inside them.
const mp4Container = "mov,mp4,m4a,3gp,3g2,mj2"

// IsDirectPlayCompatible reports whether a media item with the given
// (scan-time-probed) codec/container info can be served directly via HTTP
// range requests, or needs transcoding to HLS. Deliberately the narrowest
// matrix that covers "a typical h264/aac mp4 rip plays with zero CPU
// cost": a false negative here just costs some transcode CPU, a false
// positive means a broken player, so this stays conservative rather than
// maximizing direct-play coverage (design spec §3).
func IsDirectPlayCompatible(videoCodec, audioCodec, container string) bool {
	if videoCodec != "h264" {
		return false
	}
	if audioCodec != "aac" && audioCodec != "mp3" {
		return false
	}
	return container == mp4Container
}
