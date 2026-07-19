package dolt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"

	_ "github.com/go-sql-driver/mysql"
)

// ServerFailpoint separates the SQL working-set commit from the versioned Dolt
// commit. The measured spike must observe this boundary instead of treating a
// multi-statement CLI invocation as one durability primitive.
type ServerFailpoint string

const (
	FailpointBeforeSQLCommit ServerFailpoint = "before_sql_commit"
	FailpointAfterSQLCommit  ServerFailpoint = "after_sql_commit"
	FailpointAfterDoltCommit ServerFailpoint = "after_dolt_commit"
)

type ServerOptions struct {
	Failpoint     func(ServerFailpoint)
	StartTimeout  time.Duration
	ShutdownGrace time.Duration
}

// ServerStore owns one persistent dolt sql-server process and uses database/sql
// for all measured operations. The repository remains reopenable by a fresh
// process after Close.
type ServerStore struct {
	mu            sync.RWMutex
	binary        string
	repository    string
	db            *sql.DB
	core          *memory.Store
	command       *exec.Cmd
	processDone   chan error
	logPath       string
	failpoint     func(ServerFailpoint)
	shutdownGrace time.Duration
	closed        bool
}

func OpenServer(binary, repository string) (*ServerStore, error) {
	return OpenServerWithOptions(binary, repository, ServerOptions{})
}

func OpenServerWithOptions(binary, repository string, options ServerOptions) (*ServerStore, error) {
	if strings.TrimSpace(binary) == "" {
		return nil, errors.New("dolt binary path is required")
	}
	if strings.TrimSpace(repository) == "" {
		return nil, errors.New("dolt repository path is required")
	}
	if err := os.MkdirAll(repository, 0o755); err != nil {
		return nil, fmt.Errorf("create dolt repository directory: %w", err)
	}
	store := &ServerStore{
		binary: binary, repository: repository, failpoint: options.Failpoint,
		shutdownGrace: options.ShutdownGrace,
	}
	if store.shutdownGrace <= 0 {
		store.shutdownGrace = 5 * time.Second
	}
	if _, err := os.Stat(filepath.Join(repository, ".dolt")); errors.Is(err, os.ErrNotExist) {
		cmd := exec.Command(binary, "init", "--name", "Motor Autonomo Runtime", "--email", "runtime@localhost.invalid")
		cmd.Dir = repository
		if output, runErr := cmd.CombinedOutput(); runErr != nil {
			return nil, fmt.Errorf("initialize dolt repository: %w: %s", runErr, boundedOutput(output))
		}
	} else if err != nil {
		return nil, fmt.Errorf("inspect dolt repository: %w", err)
	}
	if err := store.start(options.StartTimeout); err != nil {
		store.stopProcess()
		return nil, err
	}
	if err := store.configureAndLoad(); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

func (s *ServerStore) start(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("reserve dolt sql-server port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return fmt.Errorf("release dolt sql-server port: %w", err)
	}
	configPath := filepath.Join(filepath.Dir(s.repository), "sql-server.yaml")
	config := fmt.Sprintf("log_level: warning\nbehavior:\n  autocommit: true\n  dolt_transaction_commit: false\nlistener:\n  host: 127.0.0.1\n  port: %d\ndata_dir: %s\ncfg_dir: %s\n", port, yamlQuote(filepath.Dir(s.repository)), yamlQuote(filepath.Join(filepath.Dir(s.repository), ".doltcfg")))
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		return fmt.Errorf("write dolt sql-server config: %w", err)
	}
	logFile, err := os.CreateTemp(filepath.Dir(s.repository), "dolt-sql-server-*.log")
	if err != nil {
		return fmt.Errorf("create dolt sql-server log: %w", err)
	}
	s.logPath = logFile.Name()
	s.command = exec.Command(s.binary, "sql-server", "--config", configPath)
	s.command.Dir = filepath.Dir(s.repository)
	s.command.Stdout = logFile
	s.command.Stderr = logFile
	if err := s.command.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start dolt sql-server: %w", err)
	}
	s.processDone = make(chan error, 1)
	go func() {
		err := s.command.Wait()
		logFile.Close()
		s.processDone <- err
	}()
	database := filepath.Base(filepath.Clean(s.repository))
	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%d)/%s?parseTime=true&multiStatements=true", port, database)
	s.db, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open dolt sql-server connection: %w", err)
	}
	s.db.SetMaxOpenConns(1)
	deadline := time.Now().Add(timeout)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		err = s.db.PingContext(ctx)
		cancel()
		if err == nil {
			return nil
		}
		select {
		case processErr := <-s.processDone:
			return fmt.Errorf("dolt sql-server exited before ready: %v: %s", processErr, s.serverLog())
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("dolt sql-server readiness timeout: %w: %s", err, s.serverLog())
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *ServerStore) configureAndLoad() error {
	var tableCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'runtime_checkpoint'`).Scan(&tableCount); err != nil {
		return fmt.Errorf("inspect dolt server checkpoint schema: %w", err)
	}
	if tableCount == 0 {
		if _, err := s.db.Exec(`CREATE TABLE runtime_checkpoint (
			id BIGINT PRIMARY KEY,
			format_version BIGINT NOT NULL,
			payload LONGBLOB NOT NULL
		)`); err != nil {
			return fmt.Errorf("configure dolt server checkpoint: %w", err)
		}
		if _, err := s.db.Exec(`CALL DOLT_ADD('-A')`); err != nil {
			return fmt.Errorf("stage dolt server schema: %w", err)
		}
		if _, err := s.db.Exec(`CALL DOLT_COMMIT('--skip-empty', '-m', 'initialize runtime checkpoint')`); err != nil {
			return fmt.Errorf("commit dolt server schema: %w", err)
		}
	}
	var formatVersion int
	var payload []byte
	err := s.db.QueryRow(`SELECT format_version, payload FROM runtime_checkpoint WHERE id = ?`, checkpointID).Scan(&formatVersion, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		s.core = memory.New()
		return nil
	}
	if err != nil {
		return fmt.Errorf("load dolt server checkpoint: %w", err)
	}
	if !memory.SupportsExternalCheckpointFormat(formatVersion) {
		return fmt.Errorf("load dolt server checkpoint: %w: got %d, support %d", memory.ErrUnsupportedCheckpointFormat, formatVersion, memory.CheckpointFormatVersion)
	}
	if err := memory.ValidateExternalCheckpoint(formatVersion, payload); err != nil {
		return fmt.Errorf("validate dolt server checkpoint: %w", err)
	}
	core, err := memory.NewFromBinary(payload)
	if err != nil {
		return fmt.Errorf("restore dolt server checkpoint: %w", err)
	}
	s.core = core
	return nil
}

func (s *ServerStore) View(ctx context.Context, fn func(port.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return errors.New("dolt server store is closed")
	}
	return s.core.View(ctx, fn)
}

func (s *ServerStore) LongTermMemory(key string) (domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return domain.LongTermMemory{}, errors.New("store is closed")
	}
	return s.core.LongTermMemory(key)
}

func (s *ServerStore) ListMemoriesByScope(scope domain.MemoryScope) ([]domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("store is closed")
	}
	return s.core.ListMemoriesByScope(scope)
}

func (s *ServerStore) ListExpiredMemories(now time.Time) ([]domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("store is closed")
	}
	return s.core.ListExpiredMemories(now)
}

func (s *ServerStore) SaveMemory(mem domain.LongTermMemory) error {
	return s.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveMemory(mem)
	})
}

func (s *ServerStore) DeleteMemory(id domain.MemoryID) error {
	return s.Update(context.Background(), func(tx port.Transaction) error {
		return tx.DeleteMemory(id)
	})
}

func (s *ServerStore) Update(ctx context.Context, fn func(port.Transaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("dolt server store is closed")
	}
	before, err := s.core.MarshalBinary()
	if err != nil {
		return err
	}
	working, err := memory.NewFromBinary(before)
	if err != nil {
		return err
	}
	if err := working.Update(ctx, fn); err != nil {
		return err
	}
	payload, err := working.MarshalBinary()
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dolt server checkpoint: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_checkpoint(id, format_version, payload)
		VALUES(?, ?, ?)
		ON DUPLICATE KEY UPDATE format_version=VALUES(format_version), payload=VALUES(payload)`, checkpointID, memory.CheckpointFormatVersion, payload); err != nil {
		return fmt.Errorf("write dolt server checkpoint: %w", err)
	}
	if s.failpoint != nil {
		s.failpoint(FailpointBeforeSQLCommit)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dolt SQL working set: %w", err)
	}
	if s.failpoint != nil {
		s.failpoint(FailpointAfterSQLCommit)
	}
	if _, err := s.db.ExecContext(ctx, `CALL DOLT_ADD('-A')`); err != nil {
		return fmt.Errorf("stage dolt checkpoint: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CALL DOLT_COMMIT('--skip-empty', '-m', 'runtime checkpoint')`); err != nil {
		return fmt.Errorf("commit dolt checkpoint: %w", err)
	}
	if s.failpoint != nil {
		s.failpoint(FailpointAfterDoltCommit)
	}
	s.core = working
	return nil
}

// WorkingSetClean reports whether SQL-visible state is fully represented by
// the current Dolt commit. A false result after recovery is an official
// INVALID_PARTIAL outcome even when the logical checkpoint itself is intact.
func (s *ServerStore) WorkingSetClean(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false, errors.New("dolt server store is closed")
	}
	var changes int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dolt_status`).Scan(&changes); err != nil {
		return false, fmt.Errorf("inspect dolt working set: %w", err)
	}
	return changes == 0, nil
}

func (s *ServerStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var dbErr error
	if s.db != nil {
		dbErr = s.db.Close()
		s.db = nil
	}
	processErr := s.stopProcess()
	s.core = nil
	return errors.Join(dbErr, processErr)
}

// CrashProcess kills the owned sql-server without a graceful shutdown. It is
// intentionally separate from Close so the subprocess crash harness can test
// recovery from loss of both the writer and its database server at an exact
// durability boundary. Production runtime code must use Close instead.
func (s *ServerStore) CrashProcess() error {
	if s.command == nil || s.command.Process == nil || s.processDone == nil {
		return errors.New("dolt sql-server process is not running")
	}
	if err := s.command.Process.Kill(); err != nil {
		return fmt.Errorf("kill dolt sql-server abruptly: %w", err)
	}
	if err := <-s.processDone; err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return fmt.Errorf("wait for crashed dolt sql-server: %w", err)
		}
	}
	s.command = nil
	s.processDone = nil
	return nil
}

func (s *ServerStore) stopProcess() error {
	if s.command == nil || s.command.Process == nil || s.processDone == nil {
		return nil
	}
	_ = s.command.Process.Signal(os.Interrupt)
	select {
	case err := <-s.processDone:
		if err != nil && !strings.Contains(err.Error(), "interrupt") {
			return fmt.Errorf("stop dolt sql-server: %w: %s", err, s.serverLog())
		}
		return nil
	case <-time.After(s.shutdownGrace):
		if err := s.command.Process.Kill(); err != nil {
			return fmt.Errorf("kill dolt sql-server: %w", err)
		}
		<-s.processDone
		return nil
	}
}

func (s *ServerStore) serverLog() string {
	content, _ := os.ReadFile(s.logPath)
	return boundedOutput(content)
}

func boundedOutput(output []byte) string {
	message := strings.TrimSpace(string(output))
	if len(message) > 2048 {
		message = message[len(message)-2048:]
	}
	return message
}

func yamlQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
