package main

import (
	"fmt"
	"github.com/WhyIsSandwich/factctl/internal/instance"
)

func main() {
	cfg, err := instance.LoadConfig("test-config.json")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Sources: %v\n", cfg.Mods.Sources)
		fmt.Printf("Enabled: %v\n", cfg.Mods.Enabled)
	}
}
