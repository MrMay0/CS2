package main

import (
	"fmt"
	"log"
	"os"

	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	events "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: demoparser <demo_file>")
		return
	}

	demoPath := os.Args[1]

	f, err := os.Open(demoPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	p := dem.NewParser(f)
	defer p.Close()

	// Quand un joueur se connecte
	p.RegisterEventHandler(func(e events.PlayerConnect) {
		fmt.Printf("[EVENT] Player connected: %s (SteamID: %d)\n", e.Player.Name, e.Player.SteamID64)
	})

	// Quand un round commence
	p.RegisterEventHandler(func(e events.RoundStart) {
		fmt.Println("[EVENT] Round started")
	})

	// Quand un kill a lieu
	p.RegisterEventHandler(func(e events.Kill) {
		if e.Killer != nil && e.Victim != nil {
			fmt.Printf("[EVENT] %s killed %s with %s\n", e.Killer.Name, e.Victim.Name, e.Weapon)
		}
	})

	// Parse toute la démo
	err = p.ParseToEnd()
	if err != nil {
		log.Fatal(err)
	}

	// Tickrate
	fmt.Printf("Tickrate: %.2f\n", p.TickRate())
}