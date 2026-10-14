// ctl.go — eonctl: a janela do operador (D15: CLI primeiro; D07: vive
// como `eon ctl`, um binário só). LEITOR por construção: o banco abre
// mode=ro (o DRIVER recusa escrita) e este pacote NUNCA importa core —
// o kernel é processo, não biblioteca, e o leitor não é kernel.
//
// O que aparece aqui é o que já aconteceu: event log é a verdade
// (D03), timeline é citação, inspect é resumo. Nada de inventar
// estado que o kernel não escreveu.
package ctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/storage"
)

// Chaves de kv_meta que o kernel escreve — contrato compartilhado
// core↔ctl dentro do mesmo binário; a bateria de integração em cmd/eon
// pega drift no ato (drives ciclos reais e lê pela ctl).
const (
	kvKeyKernelCycle = "kernel.cycle"
)

// defaultDBPath: o banco canônico dentro do data dir (ARCHITECTURE.md).
func defaultDBPath(dataDir string) string {
	return filepath.Join(dataDir, "core", "eon.db")
}

// Run: `eon ctl <inspect|timeline> --data DIR`. Escreve em w, retorna
// exit code (0 = ok; 1 = erro de leitura; 2 = uso).
func Run(args []string, w io.Writer) int {
	if len(args) == 0 {
		usage(w)
		return 2
	}
	switch args[0] {
	case "inspect":
		return runInspect(args[1:], w)
	case "timeline":
		return runTimeline(args[1:], w)
	default:
		fmt.Fprintf(w, "eon ctl: subcomando desconhecido %q\n", args[0])
		usage(w)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `eon ctl — a janela do operador (read-only)

uso: eon ctl <subcomando> --data DIR

subcomandos:
  inspect                    resumo: missão, ciclos, filas por frente, último desfecho
  timeline [--after N] [--limit N] [--task ID]
                             events do log, do mais novo para o mais antigo
`)
}

// dataDirFrom: flag --data com default $EON_DATA (o env do stack).
func dataDirFrom(args []string, sub string) (string, int, error) {
	fs := flag.NewFlagSet("ctl "+sub, flag.ContinueOnError)
	dir := fs.String("data", os.Getenv("EON_DATA"), "diretório de dados")
	if err := fs.Parse(args); err != nil {
		return "", 2, err
	}
	if *dir == "" {
		*dir = "/data"
	}
	return *dir, 0, nil
}

func runInspect(args []string, w io.Writer) int {
	dataDir, code, err := dataDirFrom(args, "inspect")
	if err != nil {
		fmt.Fprintln(w, err)
		return code
	}
	if err := Inspect(w, defaultDBPath(dataDir)); err != nil {
		fmt.Fprintf(w, "eon ctl inspect: %v\n", err)
		return 1
	}
	return 0
}

func runTimeline(args []string, w io.Writer) int {
	fs := flag.NewFlagSet("ctl timeline", flag.ContinueOnError)
	dir := fs.String("data", os.Getenv("EON_DATA"), "diretório de dados")
	after := fs.Int64("after", 0, "começa DEPOIS deste seq (cursor)")
	limit := fs.Int("limit", 50, "máximo de events (0 = sem teto)")
	task := fs.String("task", "", "filtra por id de agregado (task)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(w, err)
		return 2
	}
	if *dir == "" {
		*dir = "/data"
	}
	if err := Timeline(w, defaultDBPath(*dir), *after, *limit, *task); err != nil {
		fmt.Fprintf(w, "eon ctl timeline: %v\n", err)
		return 1
	}
	return 0
}

// Inspect: o estado do loop num relance — missão, contador de ciclos,
// contagem de tasks por frente/status, outbox pendente e o último
// desfecho de ciclo (o event kernel.cycle mais recente).
func Inspect(w io.Writer, dbPath string) error {
	store, err := storage.NewStoreReadonly(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	fmt.Fprintf(w, "eonctl — %s (read-only)\n", dbPath)

	if m, err := store.Missions().Get("m-stub"); err == nil {
		fmt.Fprintf(w, "mission:  %s — %q [%s]\n", m.ID, m.Title, m.Status)
	} else {
		fmt.Fprintf(w, "mission:  (ausente: %v)\n", err)
	}

	if raw, err := store.KV().Get(kvKeyKernelCycle); err == nil {
		var n int64
		if err := json.Unmarshal(raw, &n); err == nil {
			fmt.Fprintf(w, "cycles:   %d\n", n)
		}
	}

	for _, front := range domain.Fronts {
		counts, err := store.TaskCounts(front)
		if err != nil {
			return fmt.Errorf("contar tasks de %s: %w", front, err)
		}
		fmt.Fprintf(w, "front %-9s %s\n", front, countsLine(counts))
	}

	pending, err := store.Outbox().Pending(1000)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "outbox:   %d pendente(s)\n", len(pending))

	// último desfecho: o kernel.cycle mais novo (After é por cursor; sem
	// OFFSET, D44 — caminha do fim para trás um batch de cada vez).
	last, err := lastKernelCycle(store)
	if err != nil {
		return err
	}
	if last != nil {
		var p struct {
			Cycle   int64   `json:"cycle"`
			Front   string  `json:"front"`
			Outcome string  `json:"outcome"`
			Task    string  `json:"task"`
			Reward  float64 `json:"reward"`
		}
		if err := json.Unmarshal(last.Payload, &p); err == nil {
			fmt.Fprintf(w, "last:     ciclo %d front=%s outcome=%s task=%s reward=%.1f\n",
				p.Cycle, p.Front, p.Outcome, p.Task, p.Reward)
		}
		fmt.Fprintf(w, "          seq=%d %s\n", last.Seq, last.OccurredAt.Format("2006-01-02T15:04:05Z"))
	} else {
		fmt.Fprintln(w, "last:     (nenhum ciclo registrado ainda)")
	}
	return nil
}

// countsLine: READY · SUCCEEDED · THROTTLED · FAILED — os estados que
// dizem algo sobre a saúde da frente; o resto mora no timeline.
func countsLine(counts map[domain.TaskStatus]int) string {
	get := func(s domain.TaskStatus) string { return strconv.Itoa(counts[s]) }
	return "READY " + get(domain.TaskReady) +
		" · SUCCEEDED " + get(domain.TaskSucceeded) +
		" · THROTTLED " + get(domain.TaskThrottled) +
		" · FAILED " + get(domain.TaskFailed) +
		" · RUNNING " + get(domain.TaskRunning)
}

// lastKernelCycle: o event kernel.cycle de seq mais alto (D44: cursor,
// sem OFFSET — batches de 500 do fim para trás).
func lastKernelCycle(store *storage.Store) (*domain.Event, error) {
	var cursor int64
	batchLimit := 500
	for {
		batch, err := store.Events().After(cursor, batchLimit)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			return nil, nil
		}
		cursor = batch[len(batch)-1].Seq
		for i := len(batch) - 1; i >= 0; i-- {
			if batch[i].Type == domain.EventKernelCycle {
				ev := batch[i]
				return &ev, nil
			}
		}
	}
}

// Timeline: os events do log — do mais NOVO para o mais antigo (o
// operador quer o fim da história primeiro). `after` é o cursor de seq
// (D44); `task` filtra por agregado; `limit` 0 = sem teto. Payload
// cru: citação é o contrato, interpretação é do humano.
func Timeline(w io.Writer, dbPath string, after int64, limit int, taskID string) error {
	store, err := storage.NewStoreReadonly(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "seq\tquando\ttipo\tagregado\tpayload")

	var cursor int64 = after
	for {
		batch, err := store.Events().After(cursor, 500)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		cursor = batch[len(batch)-1].Seq
		for i := len(batch) - 1; i >= 0; i-- {
			e := batch[i]
			if taskID != "" && e.AggregateID != taskID {
				continue
			}
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n",
				e.Seq, e.OccurredAt.Format("2006-01-02 15:04:05"), e.Type,
				e.AggregateType+"/"+e.AggregateID, e.Payload)
			if limit > 0 {
				limit--
				if limit == 0 {
					tw.Flush()
					return nil
				}
			}
		}
	}
	return tw.Flush()
}
