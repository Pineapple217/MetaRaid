package config

type Scraper struct {
	SeedArtistId string `yaml:"seedArtistId"`
	WorkerCount  int    `yaml:"workerCount"`
}

func (s *Scraper) SetDefault() {
	s.SeedArtistId = "5D8TBtxnP5GZm9wUBQ8OTc" // Istasha
	s.WorkerCount = 5
}
