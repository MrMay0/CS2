package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	events "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

func mustCreateCSV(path string, header []string) (*os.File, *csv.Writer) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatalf("mkdir failed: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create file: %v", err)
	}
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		log.Fatalf("write header: %v", err)
	}
	w.Flush()
	return f, w
}

func safeName(p interface{}) string {
	// on veut juste un nom lisible; on tente d'extraire Name si possible,
	// sinon on fait fmt.Sprint(p) (robuste face à nil).
	if p == nil {
		return ""
	}
	return fmt.Sprint(p)
}

func main() {
	in := flag.String("in", "../data/raw_demos/match1.dem", "input demo (.dem)")
	outdir := flag.String("out", "../data/parsed", "output directory")
	flag.Parse()

	// base name (sans extension) pour nommer le CSV
	matchName := filepath.Base(*in)
	matchBase := matchName
	if ext := filepath.Ext(matchName); ext != "" {
		matchBase = matchName[:len(matchName)-len(ext)]
	}

	outPath := filepath.Join(*outdir, matchBase+"_kills.csv")
	f, w := mustCreateCSV(outPath, []string{"match_file", "round_id", "tick", "killer", "victim", "weapon", "headshot"})
	defer func() {
		w.Flush()
		f.Close()
	}()

	// open demo
	df, err := os.Open(*in)
	if err != nil {
		log.Fatalf("open demo: %v", err)
	}
	defer df.Close()

	p := dem.NewParser(df)
	defer p.Close()

	currentRound := 0
	// incrémenter round à chaque RoundStart (commence à 1)
	p.RegisterEventHandler(func(e events.RoundStart) {
		currentRound++
	})

	// handler Kill -> écrit une ligne CSV
	p.RegisterEventHandler(func(e events.Kill) {
		// récupérer tick via parser
		tick := p.CurrentFrame()

		killer := ""
		victim := ""
		weapon := fmt.Sprint(e.Weapon)
		headshot := strconv.FormatBool(e.IsHeadshot)

		if e.Killer != nil {
			// e.Killer.Name fonctionne généralement ; sinon fallback à fmt.Sprint
			if n := e.Killer.Name; n != "" {
				killer = n
			} else {
				killer = safeName(e.Killer)
			}
		}
		if e.Victim != nil {
			if n := e.Victim.Name; n != "" {
				victim = n
			} else {
				victim = safeName(e.Victim)
			}
		}

		record := []string{
			matchBase,
			strconv.Itoa(currentRound),
			strconv.Itoa(int(tick)),
			killer,
			victim,
			weapon,
			headshot,
		}
		if err := w.Write(record); err != nil {
			log.Printf("error writing csv: %v", err)
		}
		// flush périodiquement (sécurité si crash)
		w.Flush()
	})

	// ParseToEnd run handlers and exit
	if err := p.ParseToEnd(); err != nil {
		log.Fatalf("parse error: %v", err)
	}

	fmt.Printf("Done. Kills csv written to: %s\n", outPath)
}