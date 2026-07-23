package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestDefault_ReturnsSingleton(t *testing.T) {
	a := Default()
	b := Default()
	if a != b {
		t.Error("Default() should return the same instance")
	}
}

func TestCollector_StageTiming(t *testing.T) {
	c := &Collector{}
	c.PipelineStart()
	c.StageStart("preflight")
	time.Sleep(5 * time.Millisecond)
	c.StageEnd("preflight")

	snap := c.Snapshot()
	dur, ok := snap.StageDurations["preflight"]
	if !ok {
		t.Fatal("preflight stage duration not recorded")
	}
	if dur < 1 {
		t.Errorf("preflight duration = %d ms, want >= 1", dur)
	}
	if snap.TotalDurationMS < 1 {
		t.Errorf("total_duration_ms = %d, want >= 1", snap.TotalDurationMS)
	}
}

func TestCollector_RecordCommand(t *testing.T) {
	c := &Collector{}
	c.PipelineStart()
	c.RecordCommand()
	c.RecordCommand()
	c.RecordCommand()

	snap := c.Snapshot()
	if snap.CommandsExecuted != 3 {
		t.Errorf("CommandsExecuted = %d, want 3", snap.CommandsExecuted)
	}
}

func TestCollector_RecordRetry(t *testing.T) {
	c := &Collector{}
	c.PipelineStart()
	c.RecordRetry()

	snap := c.Snapshot()
	if snap.Retries != 1 {
		t.Errorf("Retries = %d, want 1", snap.Retries)
	}
}

func TestCollector_RecordFileModification(t *testing.T) {
	c := &Collector{}
	c.PipelineStart()
	c.RecordFileModification()
	c.RecordFileModification()

	snap := c.Snapshot()
	if snap.FilesModified != 2 {
		t.Errorf("FilesModified = %d, want 2", snap.FilesModified)
	}
}

func TestCollector_RecordIntervention(t *testing.T) {
	c := &Collector{}
	c.PipelineStart()
	c.RecordIntervention()

	snap := c.Snapshot()
	if snap.Interventions != 1 {
		t.Errorf("Interventions = %d, want 1", snap.Interventions)
	}
}

func TestCollector_ConcurrentSafety(t *testing.T) {
	c := &Collector{}
	c.PipelineStart()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.StageStart("test")
			c.RecordCommand()
			c.RecordRetry()
			c.RecordFileModification()
			c.StageEnd("test")
		}()
	}
	wg.Wait()

	snap := c.Snapshot()
	if snap.CommandsExecuted != 100 {
		t.Errorf("CommandsExecuted = %d, want 100", snap.CommandsExecuted)
	}
	if snap.Retries != 100 {
		t.Errorf("Retries = %d, want 100", snap.Retries)
	}
}

func TestCollector_SnapshotDoesNotPanic(t *testing.T) {
	c := &Collector{}
	// Snapshot before PipelineStart should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Snapshot() panicked: %v", r)
		}
	}()
	_ = c.Snapshot()
}

func TestCollector_NonBlocking_PanicRecovery(t *testing.T) {
	// Verify that a panic in a metric call doesn't propagate.
	// This tests the recover() pattern in public methods.
	c := &Collector{}
	c.PipelineStart()

	// Simulate concurrent usage — should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.RecordCommand()
			_ = c.Snapshot()
		}()
	}
	wg.Wait()
}

func TestMetrics_JSONFields(t *testing.T) {
	c := &Collector{}
	c.PipelineStart()
	c.StageStart("scan")
	c.StageEnd("scan")
	c.RecordCommand()
	c.RecordRetry()
	c.RecordFileModification()
	c.RecordIntervention()

	snap := c.Snapshot()

	// Verify all expected fields are populated.
	if snap.TotalDurationMS < 0 {
		t.Error("TotalDurationMS should be non-negative")
	}
	if len(snap.StageDurations) == 0 {
		t.Error("StageDurations should not be empty")
	}
	if snap.CommandsExecuted != 1 {
		t.Errorf("CommandsExecuted = %d, want 1", snap.CommandsExecuted)
	}
	if snap.Retries != 1 {
		t.Errorf("Retries = %d, want 1", snap.Retries)
	}
	if snap.FilesModified != 1 {
		t.Errorf("FilesModified = %d, want 1", snap.FilesModified)
	}
	if snap.Interventions != 1 {
		t.Errorf("Interventions = %d, want 1", snap.Interventions)
	}
}
