package spotify

import (
	"bytes"
	"encoding/gob"

	"github.com/zmb3/spotify/v2"
)

type FullerTrack struct {
	Track    *spotify.FullTrack
	Features *spotify.AudioFeatures
}

func (f *FullerTrack) Serialize() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
