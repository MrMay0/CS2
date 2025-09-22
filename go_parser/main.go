package main

import (
    "encoding/csv"
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"

    dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
    "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
    "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

var (
    matchID        = "match001"
    roundID        = 0
    roundStartTick = 0
    playerMap      = make(map[uint64]*PlayerRecord)

    playersCSV   *csv.Writer
    roundsCSV    *csv.Writer
    purchasesCSV *csv.Writer
    killsCSV     *csv.Writer
    damagesCSV   *csv.Writer
    grenadesCSV  *csv.Writer
    economyCSV   *csv.Writer
    duelsCSV     *csv.Writer
    gameCSV      *csv.Writer
)

type PlayerRecord struct {
    SteamID   uint64
    Name      string
    StartSide string
    FinalSide string
    Kills     int
    Deaths    int
}

type RoundInfo struct {
    RoundID   int
    StartTick int
    EndTick   int
    Duration  float64
    Winner    string
    Reason    string
    Alive     string
}

func main() {
    // ---------- SETUP ----------
    inputFile := "../data/raw_demos/match1.dem"
    f, err := os.Open(inputFile)
    if err != nil {
        panic(err)
    }
    defer f.Close()

    parser := dem.NewParser(f)
    defer parser.Close()

    // ---------- CREATE OUTPUT DIR ----------
    outDir := filepath.Join("../data/parsed", matchID)
    if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
        panic(err)
    }

    // ---------- INIT CSV ----------
    playersCSV = initCSV(filepath.Join(outDir, "players.csv"),
        []string{"match_id", "steamid", "name", "start_side", "kills", "deaths"})
    roundsCSV = initCSV(filepath.Join(outDir, "rounds.csv"),
        []string{"match_id", "round_id", "start_tick", "end_tick", "duration", "winner", "reason", "alive_players"})
    purchasesCSV = initCSV(filepath.Join(outDir, "purchases.csv"),
        []string{"match_id", "round_id", "steamid", "name", "item"})
    killsCSV = initCSV(filepath.Join(outDir, "kills.csv"),
        []string{"match_id", "round_id", "tick", "killer_steamid", "killer_name", "victim_steamid", "victim_name", "weapon", "headshot", "x", "y", "z", "assist", "assist_player_name"})
    damagesCSV = initCSV(filepath.Join(outDir, "damages.csv"),
        []string{"match_id", "round_id", "attacker_steamid", "attacker_name", "victim_steamid", "victim_name", "weapon", "damage", "armor_damage", "health_damage"})
    grenadesCSV = initCSV(filepath.Join(outDir, "grenades.csv"),
        []string{"match_id", "round_id", "thrower_steamid", "thrower_name", "grenade_type", "x", "y", "z"})
    economyCSV = initCSV(filepath.Join(outDir, "economy.csv"),
        []string{"match_id", "round_id", "steamid", "name", "team", "money"})
    duelsCSV = initCSV(filepath.Join(outDir, "duels.csv"),
        []string{"match_id", "round_id", "p1_steamid", "p1_name", "p2_steamid", "p2_name", "winner_steamid", "winner_name"})
    gameCSV = initCSV(filepath.Join(outDir, "game.csv"),
        []string{"match_id", "map_name", "drristo_team_is_winner", "round_count", "duration_s"})

    // ---------- TRACK MATCH INFO ----------
    var mapName string
    var rounds []RoundInfo

    const drristoSteamID uint64 = 76561198870364933

    // ---------- EVENT HANDLERS ----------
    parser.RegisterEventHandler(func(e events.MatchStart) {
        fmt.Println("Match started:", matchID)
        if hdr := parser.Header(); hdr != (common.DemoHeader{}) {
            mapName = hdr.MapName
        }
    })

    parser.RegisterEventHandler(func(e events.PlayerTeamChange) {
        if e.Player == nil {
            return
        }
        team := correctedTeamToString(e.NewTeam)
        if rec, ok := playerMap[e.Player.SteamID64]; ok {
            rec.FinalSide = team
            if rec.StartSide == "" && (team == "T" || team == "CT") {
                rec.StartSide = team
            }
        } else {
            playerMap[e.Player.SteamID64] = &PlayerRecord{
                SteamID:   e.Player.SteamID64,
                Name:      e.Player.Name,
                StartSide: team,
                FinalSide: team,
            }
        }
    })

    parser.RegisterEventHandler(func(e events.RoundStart) {
        roundID++
        roundStartTick = parser.GameState().IngameTick()
        if roundID == 0 {
            return
        }
        for _, pl := range parser.GameState().Participants().Playing() {
            writeRow(economyCSV, []string{
                matchID, strconv.Itoa(roundID),
                strconv.FormatUint(pl.SteamID64, 10), pl.Name,
                teamToString(pl.Team),
                strconv.Itoa(pl.Money()),
            })
        }
    })

    parser.RegisterEventHandler(func(e events.RoundEnd) {
        if roundID == 0 {
            return
        }
        endTick := parser.GameState().IngameTick()
        duration := float64(endTick-roundStartTick) / 64.0
        winnerTeam := teamToString(e.Winner)
        reason := fmt.Sprintf("%v", e.Reason)

        // collect alive players
        aliveList := []string{}
        for _, pl := range parser.GameState().Participants().Playing() {
            if pl.IsAlive() {
                aliveList = append(aliveList, fmt.Sprintf("%s(%d)", pl.Name, pl.SteamID64))
            }
        }
        aliveStr := strings.Join(aliveList, ";")

        ri := RoundInfo{
            RoundID:   roundID,
            StartTick: roundStartTick,
            EndTick:   endTick,
            Duration:  duration,
            Winner:    winnerTeam,
            Reason:    reason,
            Alive:     aliveStr,
        }
        rounds = append(rounds, ri)

        writeRow(roundsCSV, []string{
            matchID, strconv.Itoa(ri.RoundID),
            strconv.Itoa(ri.StartTick), strconv.Itoa(ri.EndTick),
            fmt.Sprintf("%.3f", ri.Duration),
            ri.Winner, ri.Reason, ri.Alive,
        })
    })

    var purchasesSeen = make(map[string]bool)

    parser.RegisterEventHandler(func(e events.ItemEquip) {
        if roundID == 0 || e.Weapon == nil || e.Player == nil {
            return
        }
        item := e.Weapon.String()
        if strings.Contains(item, "Knife") || item == "C4" {
            return
        }
        key := fmt.Sprintf("%s_%d_%d_%s", matchID, roundID, e.Player.SteamID64, item)
        if purchasesSeen[key] {
            return
        }
        purchasesSeen[key] = true

        writeRow(purchasesCSV, []string{
            matchID, strconv.Itoa(roundID),
            strconv.FormatUint(e.Player.SteamID64, 10), e.Player.Name,
            item,
        })
    })

    parser.RegisterEventHandler(func(e events.Kill) {
        if roundID == 0 || e.Killer == nil || e.Victim == nil {
            return
        }
        if rec, ok := playerMap[e.Killer.SteamID64]; ok {
            rec.Kills++
        }
        if rec, ok := playerMap[e.Victim.SteamID64]; ok {
            rec.Deaths++
        }

        vpos := e.Victim.Position()
        assist := false
        assistName := ""
        if e.Assister != nil {
            assist = true
            assistName = e.Assister.Name
        }

        writeRow(killsCSV, []string{
            matchID, strconv.Itoa(roundID),
            strconv.Itoa(parser.GameState().IngameTick()),
            strconv.FormatUint(e.Killer.SteamID64, 10), e.Killer.Name,
            strconv.FormatUint(e.Victim.SteamID64, 10), e.Victim.Name,
            e.Weapon.String(), strconv.FormatBool(e.IsHeadshot),
            fmt.Sprintf("%.3f", vpos.X), fmt.Sprintf("%.3f", vpos.Y), fmt.Sprintf("%.3f", vpos.Z),
            strconv.FormatBool(assist), assistName,
        })

        writeRow(duelsCSV, []string{
            matchID, strconv.Itoa(roundID),
            strconv.FormatUint(e.Killer.SteamID64, 10), e.Killer.Name,
            strconv.FormatUint(e.Victim.SteamID64, 10), e.Victim.Name,
            strconv.FormatUint(e.Killer.SteamID64, 10), e.Killer.Name,
        })
    })

    parser.RegisterEventHandler(func(e events.PlayerHurt) {
        if roundID == 0 || e.Attacker == nil || e.Player == nil {
            return
        }
        writeRow(damagesCSV, []string{
            matchID, strconv.Itoa(roundID),
            strconv.FormatUint(e.Attacker.SteamID64, 10), e.Attacker.Name,
            strconv.FormatUint(e.Player.SteamID64, 10), e.Player.Name,
            e.Weapon.String(),
            strconv.Itoa(e.HealthDamage + e.ArmorDamage),
            strconv.Itoa(e.ArmorDamage),
            strconv.Itoa(e.HealthDamage),
        })
    })

    parser.RegisterEventHandler(func(e events.GrenadeProjectileThrow) {
        if roundID == 0 || e.Projectile == nil || e.Projectile.Thrower == nil {
            return
        }
        ppos := e.Projectile.Position()
        writeRow(grenadesCSV, []string{
            matchID, strconv.Itoa(roundID),
            strconv.FormatUint(e.Projectile.Thrower.SteamID64, 10), e.Projectile.Thrower.Name,
            e.Projectile.WeaponInstance.String(),
            fmt.Sprintf("%.3f", ppos.X), fmt.Sprintf("%.3f", ppos.Y), fmt.Sprintf("%.3f", ppos.Z),
        })
    })

    // ---------- PARSE ----------
    if err := parser.ParseToEnd(); err != nil {
        panic(err)
    }

    // ---------- AGGREGATE FINAL GAME-LEVEL METRICS ----------
    finalScoreCT := 0
    finalScoreT := 0
    var totalDuration float64
    for _, r := range rounds {
        if r.Winner == "CT" {
            finalScoreCT++
        } else if r.Winner == "T" {
            finalScoreT++
        }
        totalDuration += r.Duration
    }
    roundCount := len(rounds)

    drristoTeam := ""
    if rec, ok := playerMap[drristoSteamID]; ok {
        drristoTeam = rec.FinalSide
    } else {
        if parts := parser.GameState().Participants(); parts != nil {
            for _, p := range parts.All() {
                if p != nil && p.SteamID64 == drristoSteamID {
                    drristoTeam = teamToString(p.Team)
                    break
                }
            }
        }
    }

    winner := "T"
    if finalScoreCT > finalScoreT {
        winner = "CT"
    }

    drristoWin := "0"
    if drristoTeam != "" && drristoTeam == winner {
        drristoWin = "1"
    }

    for _, rec := range playerMap {
        writeRow(playersCSV, []string{
            matchID, strconv.FormatUint(rec.SteamID, 10),
            rec.Name, rec.StartSide,
            strconv.Itoa(rec.Kills), strconv.Itoa(rec.Deaths),
        })
    }

    writeRow(gameCSV, []string{
        matchID,
        mapName,
        drristoWin,
        strconv.Itoa(roundCount),
        fmt.Sprintf("%.3f", totalDuration),
    })

    // flush
    playersCSV.Flush()
    roundsCSV.Flush()
    purchasesCSV.Flush()
    killsCSV.Flush()
    damagesCSV.Flush()
    grenadesCSV.Flush()
    economyCSV.Flush()
    duelsCSV.Flush()
    gameCSV.Flush()

    fmt.Printf("Parsed demo and wrote CSVs to %s\n", outDir)
}

// ---------- HELPERS ----------
func initCSV(path string, header []string) *csv.Writer {
    f, err := os.Create(path)
    if err != nil {
        panic(err)
    }
    w := csv.NewWriter(f)
    w.Write(header)
    return w
}

func writeRow(w *csv.Writer, row []string) {
    _ = w.Write(row)
}

func teamToString(team common.Team) string {
    switch team {
    case common.TeamTerrorists:
        return "T"
    case common.TeamCounterTerrorists:
        return "CT"
    default:
        return "SPEC"
    }
}

func correctedTeamToString(team common.Team) string {
    switch team {
    case common.TeamTerrorists:
        return "CT"
    case common.TeamCounterTerrorists:
        return "T"
    default:
        return "SPEC"
    }
}