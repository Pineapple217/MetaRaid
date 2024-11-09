package config

type Spotify struct {
	Clients []struct {
		ClientId     string `yaml:"clientId"`
		ClientSecret string `yaml:"clientSecret"`
	} `yaml:"clients"`
}

func (s *Spotify) SetDefault() {

}
