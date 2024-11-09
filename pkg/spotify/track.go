package spotify

import (
	"github.com/zmb3/spotify/v2"
)

type FullerTrack struct {
	Track    *spotify.FullTrack
	Features *spotify.AudioFeatures
}
