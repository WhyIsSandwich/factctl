package main

import (
	"fmt"
	"os"
	"github.com/WhyIsSandwich/factctl/internal/jsonc"
)

func main() {
	f, err := os.Open("test-config.json")
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer f.Close()

	var cfg struct {
		Mods struct {
			Enabled []string          `json:"enabled"`
			Sources map[string]string `json:"sources"`
		} `json:"mods"`
	}

	if err := jsonc.Parse(f, &cfg); err != nil {
		fmt.Printf("Error parsing: %v\n", err)
	} else {
		fmt.Printf("Sources: %v\n", cfg.Mods.Sources)
		fmt.Printf("Enabled: %v\n", cfg.Mods.Enabled)
	}
}
