package maintenance

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/output"
)

// Scheduler manages database maintenance tasks like VACUUM and message cleanup
type Scheduler struct {
	db                   *sql.DB
	logger               output.Logger
	vacuumInterval       time.Duration
	messageRetentionDays int
	ticker               *time.Ticker
	done                 chan struct{}
	wg                   sync.WaitGroup
	lastVacuumTime       time.Time
	vacuumMutex          sync.Mutex
	isRunning            bool
	runningMutex         sync.Mutex
}

// New creates a new maintenance scheduler
func New(db *sql.DB, logger output.Logger, vacuumInterval time.Duration, messageRetentionDays int) *Scheduler {
	return &Scheduler{
		db:                   db,
		logger:               logger,
		vacuumInterval:       vacuumInterval,
		messageRetentionDays: messageRetentionDays,
		done:                 make(chan struct{}),
	}
}

// Start begins the maintenance scheduler
// It runs VACUUM operations at the configured interval
func (s *Scheduler) Start() error {
	s.runningMutex.Lock()
	if s.isRunning {
		s.runningMutex.Unlock()
		return fmt.Errorf("scheduler is already running")
	}
	s.isRunning = true
	s.runningMutex.Unlock()

	s.logger.Info("Starting database maintenance scheduler (VACUUM every %v)", s.vacuumInterval)

	// Create ticker for periodic VACUUM
	s.ticker = time.NewTicker(s.vacuumInterval)

	// Start the maintenance goroutine
	s.wg.Add(1)
	go s.run()

	return nil
}

// Stop stops the maintenance scheduler
func (s *Scheduler) Stop() error {
	s.runningMutex.Lock()
	if !s.isRunning {
		s.runningMutex.Unlock()
		return fmt.Errorf("scheduler is not running")
	}
	s.isRunning = false
	s.runningMutex.Unlock()

	s.logger.Info("Stopping database maintenance scheduler...")

	// Signal the goroutine to stop
	close(s.done)

	// Wait for the goroutine to finish
	s.wg.Wait()

	// Stop the ticker
	if s.ticker != nil {
		s.ticker.Stop()
	}

	s.logger.Success("Database maintenance scheduler stopped")
	return nil
}

// run is the main maintenance loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	for {
		select {
		case <-s.done:
			return
		case <-s.ticker.C:
			// Run VACUUM during low-activity periods (Requirement 28.3)
			// For now, we run it on schedule. In production, this could be enhanced
			// to detect low-activity periods (e.g., early morning hours)
			if err := s.runVacuum(); err != nil {
				s.logger.Error("VACUUM operation failed: %v", err)
			}

			// Run message cleanup (Requirement 14.6, 14.7)
			if err := s.runCleanup(); err != nil {
				s.logger.Error("Message cleanup failed: %v", err)
			}
		}
	}
}

// runVacuum performs a VACUUM operation on the database
// VACUUM reclaims unused space and optimizes the database file
func (s *Scheduler) runVacuum() error {
	s.vacuumMutex.Lock()
	defer s.vacuumMutex.Unlock()

	startTime := time.Now()
	s.logger.Info("Starting database VACUUM operation...")

	// Create a context with a reasonable timeout for VACUUM
	// VACUUM can take a while on large databases, so we use a generous timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Execute VACUUM in a goroutine so we can monitor timeout
	errChan := make(chan error, 1)
	go func() {
		_, err := s.db.ExecContext(ctx, "VACUUM")
		errChan <- err
	}()

	// Wait for VACUUM to complete or timeout
	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("VACUUM failed: %w", err)
		}

		duration := time.Since(startTime)
		s.vacuumMutex.Lock()
		s.lastVacuumTime = time.Now()
		s.vacuumMutex.Unlock()

		// Log successful VACUUM with duration (Requirement 28.4)
		s.logger.Success("VACUUM completed successfully in %.2f seconds", duration.Seconds())
		return nil

	case <-ctx.Done():
		return fmt.Errorf("VACUUM operation timed out after 5 minutes")
	}
}

// GetLastVacuumTime returns the time of the last successful VACUUM operation
func (s *Scheduler) GetLastVacuumTime() time.Time {
	s.vacuumMutex.Lock()
	defer s.vacuumMutex.Unlock()
	return s.lastVacuumTime
}

// IsRunning returns whether the scheduler is currently running
func (s *Scheduler) IsRunning() bool {
	s.runningMutex.Lock()
	defer s.runningMutex.Unlock()
	return s.isRunning
}

// runCleanup performs message cleanup based on retention policy
// This implements Requirement 14.6 and 14.7 - automatic cleanup of old messages
func (s *Scheduler) runCleanup() error {
	// We need to create a temporary DB wrapper to call CleanupOldMessages
	// Since we only have the raw *sql.DB, we'll execute the cleanup directly
	startTime := time.Now()
	s.logger.Info("Starting message cleanup (retention: %d days)...", s.messageRetentionDays)

	// Count messages before cleanup
	countBefore := 0
	err := s.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&countBefore)
	if err != nil {
		return fmt.Errorf("failed to count messages before cleanup: %w", err)
	}

	// Delete messages older than retention period
	query := `
		DELETE FROM messages 
		WHERE timestamp < datetime('now', '-' || ? || ' days')
	`
	result, err := s.db.Exec(query, s.messageRetentionDays)
	if err != nil {
		s.logger.Error("Message cleanup failed: %v", err)
		return fmt.Errorf("failed to cleanup old messages: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// Count messages after cleanup
	countAfter := 0
	err = s.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&countAfter)
	if err != nil {
		return fmt.Errorf("failed to count messages after cleanup: %w", err)
	}

	duration := time.Since(startTime)

	// Log cleanup operation (Requirement 14.6)
	s.logger.Success("Message cleanup completed in %.2f seconds: deleted %d messages (%d → %d remaining)",
		duration.Seconds(), rowsAffected, countBefore, countAfter)

	return nil
}
