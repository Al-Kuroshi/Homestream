package mediastore

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

type ProbeResult struct {
	DurationSec float64
	VideoCodec  string
	AudioCodec  string
	Container   string
}

type ffprobeOutput struct {
	Format struct {
		Duration   string `json:"duration"`
		FormatName string `json:"format_name"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

// Probe runs ffprobe against path and returns its technical metadata.
// A non-nil error means the file could not be read as media at all —
// callers (mediastore.Scanner) treat that as "mark this item invalid",
// not as a fatal error for the whole scan.
func Probe(path string) (*ProbeResult, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed for %s: %w", path, err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parsing ffprobe output for %s: %w", path, err)
	}

	duration, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing duration for %s: %w", path, err)
	}

	result := &ProbeResult{
		DurationSec: duration,
		Container:   parsed.Format.FormatName,
	}
	for _, s := range parsed.Streams {
		switch s.CodecType {
		case "video":
			if result.VideoCodec == "" {
				result.VideoCodec = s.CodecName
			}
		case "audio":
			if result.AudioCodec == "" {
				result.AudioCodec = s.CodecName
			}
		}
	}

	return result, nil
}
