// Package metrics provides non-blocking pipeline telemetry collection.
// A metrics failure never halts the pipeline — all public methods recover
// from panics.
package metrics

import (
	"sync"
	"time"
)

// Metrics is the snapshot of pipeline execution metrics.
type Metrics struct {
	TotalDurationMS  int64            `json:"total_duration_ms"`
	StageDurations   map[string]int64 `json:"stage_durations"`
	CommandsExecuted int64            `json:"commands_executed"`
	FilesModified    int64            `json:"files_modified"`
	Retries          int64            `json:"retries"`
	Interventions    int64            `json:"human_interventions"`
}

// Collector accumulates pipeline metrics with thread-safe counters.
type Collector struct {
	mu             sync.Mutex
	pipelineStart  time.Time
	stageStarts    map[string]time.Time
	stageDurations map[string]int64
	commands       int64
	files          int64
	retries        int64
	interventions  int64
}

var (
	defaultCollector *Collector
	defaultOnce      sync.Once
)

// Default returns the singleton metrics collector.
func Default() *Collector {
	defaultOnce.Do(func() {
		defaultCollector = &Collector{}
	})
	return defaultCollector
}

// PipelineStart records the pipeline start time.
func (c *Collector) PipelineStart() {
	defer func() { recover() }()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pipelineStart = time.Now()
	c.stageStarts = make(map[string]time.Time)
	c.stageDurations = make(map[string]int64)
}

// StageStart records the start of a named stage.
func (c *Collector) StageStart(name string) {
	defer func() { recover() }()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stageStarts == nil {
		c.stageStarts = make(map[string]time.Time)
	}
	c.stageStarts[name] = time.Now()
}

// StageEnd records the end of a named stage and computes its duration.
func (c *Collector) StageEnd(name string) {
	defer func() { recover() }()
	c.mu.Lock()
	defer c.mu.Unlock()
	start, ok := c.stageStarts[name]
	if !ok {
		return
	}
	dur := time.Since(start).Milliseconds()
	if c.stageDurations == nil {
		c.stageDurations = make(map[string]int64)
	}
	c.stageDurations[name] = dur
	delete(c.stageStarts, name)
}

// RecordCommand increments the commands executed counter.
func (c *Collector) RecordCommand() {
	defer func() { recover() }()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands++
}

// RecordRetry increments the retry counter.
func (c *Collector) RecordRetry() {
	defer func() { recover() }()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.retries++
}

// RecordFileModification increments the files modified counter.
func (c *Collector) RecordFileModification() {
	defer func() { recover() }()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files++
}

// RecordIntervention increments the human interventions counter.
func (c *Collector) RecordIntervention() {
	defer func() { recover() }()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interventions++
}

// Snapshot returns a point-in-time copy of the collected metrics.
func (c *Collector) Snapshot() Metrics {
	defer func() { recover() }()
	c.mu.Lock()
	defer c.mu.Unlock()

	var totalMS int64
	if !c.pipelineStart.IsZero() {
		totalMS = time.Since(c.pipelineStart).Milliseconds()
	}

	stages := make(map[string]int64, len(c.stageDurations))
	for k, v := range c.stageDurations {
		stages[k] = v
	}

	return Metrics{
		TotalDurationMS:  totalMS,
		StageDurations:   stages,
		CommandsExecuted: c.commands,
		FilesModified:    c.files,
		Retries:          c.retries,
		Interventions:    c.interventions,
	}
}
