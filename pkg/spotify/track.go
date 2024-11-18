package spotify

import (
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zmb3/spotify/v2"
)

type FullerTrack struct {
	Track    *spotify.FullTrack
	Features *spotify.AudioFeatures
	Artists  []*spotify.FullArtist
}

func (f *FullerTrack) Serialize() ([]byte, error) {
	b, err := msgpack.Marshal(f)
	return b, err
}
