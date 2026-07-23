package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/retry"
	"motor-autonomo/internal/storage/sqlite"
)

const (
	boundedContentionWriters     = 4
	boundedContentionMaxAttempts = 12
	boundedContentionBusyTimeout = 10 * time.Millisecond
	boundedContentionBaseBackoff = 15 * time.Millisecond
)

type boundedContentionResult struct {
	Writer        string `json:"writer"`
	Attempts      int    `json:"attempts"`
	Busy          int    `json:"busy"`
	Conflicts     int    `json:"conflicts"`
	BackoffMillis int64  `json:"backoff_millis"`
	Succeeded     bool   `json:"succeeded"`
	Error         string `json:"error,omitempty"`
}

func TestSQLiteSubprocessBoundedContentionRetryDistribution(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runtime.sqlite")
	leaderStart := filepath.Join(dir, "leader-start")
	followersStart := filepath.Join(dir, "followers-start")
	lockHeld := filepath.Join(dir, "lock-held")

	commands := make([]*os.Process, 0, boundedContentionWriters)
	cmds := make([]interface{ Wait() error }, 0, boundedContentionWriters)
	for writer := 0; writer < boundedContentionWriters; writer++ {
		name := fmt.Sprintf("writer-%d", writer)
		mode := "contend-follower"
		startPath := followersStart
		if writer == 0 {
			mode = "contend-leader"
			startPath = leaderStart
		}
		readyPath := filepath.Join(dir, name+"-ready")
		resultPath := filepath.Join(dir, name+"-result.json")
		cmd := sqliteHelperCommand(mode, dbPath, readyPath, name)
		cmd.Env = append(cmd.Env,
			"MOTOR_AUTONOMO_SQLITE_CONTENTION_START="+startPath,
			"MOTOR_AUTONOMO_SQLITE_CONTENTION_LOCK_HELD="+lockHeld,
			"MOTOR_AUTONOMO_SQLITE_CONTENTION_RESULT="+resultPath,
		)
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %s: %v", name, err)
		}
		commands = append(commands, cmd.Process)
		cmds = append(cmds, cmd)
		waitForFile(t, readyPath, 5*time.Second)
	}
	defer func() {
		for _, process := range commands {
			_ = process.Kill()
		}
	}()

	writeSignalFile(t, leaderStart)
	waitForFile(t, lockHeld, 5*time.Second)
	writeSignalFile(t, followersStart)
	for writer, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("writer-%d: %v", writer, err)
		}
	}

	totalBusy := 0
	totalConflicts := 0
	totalAttempts := 0
	for writer := 0; writer < boundedContentionWriters; writer++ {
		name := fmt.Sprintf("writer-%d", writer)
		resultPath := filepath.Join(dir, name+"-result.json")
		body, err := os.ReadFile(resultPath)
		if err != nil {
			t.Fatalf("read %s result: %v", name, err)
		}
		var result boundedContentionResult
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode %s result: %v", name, err)
		}
		if !result.Succeeded || result.Error != "" {
			t.Fatalf("%s did not converge: %+v", name, result)
		}
		if result.Attempts < 1 || result.Attempts > boundedContentionMaxAttempts {
			t.Fatalf("%s attempts = %d, want 1..%d", name, result.Attempts, boundedContentionMaxAttempts)
		}
		t.Logf("%s attempts=%d busy=%d conflicts=%d backoff_ms=%d", name, result.Attempts, result.Busy, result.Conflicts, result.BackoffMillis)
		totalBusy += result.Busy
		totalConflicts += result.Conflicts
		totalAttempts += result.Attempts
	}
	if totalBusy == 0 {
		t.Fatal("contention campaign observed no SQLITE_BUSY outcomes")
	}
	if totalConflicts == 0 {
		t.Fatal("contention campaign observed no stale-checkpoint CAS conflicts")
	}
	if totalAttempts > boundedContentionWriters*boundedContentionMaxAttempts {
		t.Fatalf("total attempts = %d, exceeds bounded ceiling", totalAttempts)
	}
	t.Logf("aggregate writers=%d attempts=%d busy=%d conflicts=%d ceiling=%d", boundedContentionWriters, totalAttempts, totalBusy, totalConflicts, boundedContentionWriters*boundedContentionMaxAttempts)

	reopened, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen contention database: %v", err)
	}
	defer reopened.Close()
	if err := reopened.View(context.Background(), func(reader port.Reader) error {
		for writer := 0; writer < boundedContentionWriters; writer++ {
			key := fmt.Sprintf("writer-%d", writer)
			if _, err := reader.IdempotencyRecord(domain.IdempotencyKey(key)); err != nil {
				return fmt.Errorf("idempotency record %q: %w", key, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func runSQLiteBoundedContentionHelper(t *testing.T, mode string) {
	dbPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_DB")
	writer := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_KEY")
	readyPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_SUBPROCESS_READY")
	startPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_CONTENTION_START")
	resultPath := os.Getenv("MOTOR_AUTONOMO_SQLITE_CONTENTION_RESULT")
	if dbPath == "" || writer == "" || readyPath == "" || startPath == "" || resultPath == "" {
		t.Fatal("bounded contention helper requires database, writer, ready, start, and result paths")
	}

	options := sqlite.Options{BusyTimeout: boundedContentionBusyTimeout}
	if mode == "contend-leader" {
		lockHeld := os.Getenv("MOTOR_AUTONOMO_SQLITE_CONTENTION_LOCK_HELD")
		options.Failpoint = func(point sqlite.Failpoint) {
			if point != sqlite.FailpointBeforeDurableCommit {
				return
			}
			if err := os.WriteFile(lockHeld, []byte("held\n"), 0o600); err != nil {
				panic(err)
			}
			time.Sleep(120 * time.Millisecond)
		}
	}
	store, err := sqlite.OpenWithOptions(dbPath, options)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	writeSignalFile(t, readyPath)
	waitForFile(t, startPath, 5*time.Second)

	// The operation is safe to repeat because ReserveIdempotency is keyed by
	// writer and the store rejects a divergent replay. Keep retry ownership in
	// this caller rather than hiding it in the SQLite adapter.
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(writer))
	jitter := fixedJitterSource{value: uint64(hash.Sum32() % 37)}
	report, runErr := retry.Do(
		context.Background(),
		retry.Policy{
			MaxAttempts: boundedContentionMaxAttempts,
			BaseDelay:   boundedContentionBaseBackoff,
			MaxDelay:    120 * time.Millisecond,
			MaxJitter:   36 * time.Millisecond,
		},
		retry.SystemSleeper{},
		jitter,
		func(err error) (string, bool) {
			switch {
			case errors.Is(err, port.ErrConflict):
				return "conflict", true
			case isSQLiteBusy(err), errors.Is(err, context.DeadlineExceeded):
				return "busy", true
			default:
				return "fatal", false
			}
		},
		func(_ context.Context, _ int) error {
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			return store.Update(ctx, reserveIdempotency(writer))
		},
	)
	result := boundedContentionResult{
		Writer:        writer,
		Attempts:      report.Attempts,
		Busy:          report.Classes["busy"],
		Conflicts:     report.Classes["conflict"],
		BackoffMillis: report.SleepTotal.Milliseconds(),
		Succeeded:     runErr == nil,
	}
	if runErr != nil {
		result.Error = runErr.Error()
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

type fixedJitterSource struct {
	value uint64
}

func (source fixedJitterSource) Uint64() (uint64, error) {
	return source.value, nil
}

func writeSignalFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("write signal %s: %v", path, err)
	}
}
