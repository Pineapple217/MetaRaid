package main

import (
	"fmt"

	"github.com/Pineapple217/MetaRaid/pkg/config"
	"gopkg.in/yaml.v3"
)

func main() {
	s := config.Config{}
	s.SetDefault()
	y, _ := yaml.Marshal(&s)
	fmt.Print(string(y))
}
