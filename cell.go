package main

import (
	"context"
	"fmt"
	"sync"
)

// Cell is a persistent program-execution unit owned by one originating agent
// tool call. It is not another agent turn.
type Cell struct {
	ID                string
	OriginatingCallID string
	State             CellState
	Output            string
	nextToolSequence  int
}

type CellState string

const (
	CellRunning   CellState = "running"
	CellYielded   CellState = "yielded"
	CellCompleted CellState = "completed"
	CellCancelled CellState = "cancelled"
)

// CellManager owns cell identity, lifecycle, and nested calls. This version is
// deliberately in-process; a later lesson will add a real runtime.
type CellManager struct {
	mu     sync.Mutex
	nextID int
	cells  map[string]*Cell
}

func NewCellManager() *CellManager {
	return &CellManager{cells: make(map[string]*Cell)}
}

func (m *CellManager) Start(originatingCallID string) Cell {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	cell := &Cell{
		ID:                fmt.Sprintf("cell_%d", m.nextID),
		OriginatingCallID: originatingCallID,
		State:             CellRunning,
	}
	m.cells[cell.ID] = cell
	return *cell
}

func (m *CellManager) Snapshot(cellID string) (Cell, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cell, ok := m.cells[cellID]
	if !ok {
		return Cell{}, false
	}
	return *cell, true
}

// StartTool gives a cell-owned nested tool call a unique runtime sequence and
// sends it through the same registry as direct calls.
func (m *CellManager) StartTool(ctx context.Context, tools *ToolRegistry, cellID, toolName, input string) (ToolFuture, error) {
	m.mu.Lock()
	cell, ok := m.cells[cellID]
	if !ok {
		m.mu.Unlock()
		return ToolFuture{}, fmt.Errorf("unknown cell: %s", cellID)
	}
	if cell.State != CellRunning {
		m.mu.Unlock()
		return ToolFuture{}, fmt.Errorf("cell %s is %s", cellID, cell.State)
	}
	cell.nextToolSequence++
	sequence := cell.nextToolSequence
	m.mu.Unlock()

	runtimeToolCallID := fmt.Sprintf("tool_%d", sequence)
	call := ToolCall{
		Name:  toolName,
		Input: input,
		ID:    fmt.Sprintf("%s_%s", cellID, runtimeToolCallID),
		Source: ToolCallSource{
			Kind:              ToolCallCodeMode,
			CellID:            cellID,
			RuntimeToolCallID: runtimeToolCallID,
		},
	}
	return tools.Start(ctx, call), nil
}

func (m *CellManager) Complete(cellID, output string) error {
	return m.finish(cellID, CellCompleted, output, CellRunning)
}

func (m *CellManager) Cancel(cellID string) error {
	return m.transition(cellID, CellCancelled, CellRunning, CellYielded)
}

// Yield preserves the cell and its local tool sequence while handing control
// back to the caller. A yielded cell cannot issue another nested call.
func (m *CellManager) Yield(cellID, output string) error {
	return m.finish(cellID, CellYielded, output, CellRunning)
}

// Wait represents the harness asking a previously yielded cell to continue.
// The cell is running again only after this explicit transition.
func (m *CellManager) Wait(cellID string) error {
	return m.transition(cellID, CellRunning, CellYielded)
}

// OutputForModel returns only a cell-level runtime response. Nested tool
// results are intentionally not exposed here: they belong to the cell's local
// program state until it yields or finishes.
func (m *CellManager) OutputForModel(cellID string) (CellOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cell, ok := m.cells[cellID]
	if !ok {
		return CellOutput{}, fmt.Errorf("unknown cell: %s", cellID)
	}
	if cell.State == CellRunning {
		return CellOutput{}, fmt.Errorf("cell %s is still running", cellID)
	}
	return CellOutput{
		CellID:            cell.ID,
		OriginatingCallID: cell.OriginatingCallID,
		State:             cell.State,
		Content:           cell.Output,
	}, nil
}

type CellOutput struct {
	CellID, OriginatingCallID string
	State                     CellState
	Content                   string
}

func (m *CellManager) finish(cellID string, next CellState, output string, allowed ...CellState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cell, ok := m.cells[cellID]
	if !ok {
		return fmt.Errorf("unknown cell: %s", cellID)
	}
	for _, state := range allowed {
		if cell.State == state {
			cell.State = next
			cell.Output = output
			return nil
		}
	}
	return fmt.Errorf("cell %s is %s", cellID, cell.State)
}

func (m *CellManager) transition(cellID string, next CellState, allowed ...CellState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cell, ok := m.cells[cellID]
	if !ok {
		return fmt.Errorf("unknown cell: %s", cellID)
	}
	for _, state := range allowed {
		if cell.State == state {
			cell.State = next
			return nil
		}
	}
	return fmt.Errorf("cell %s is %s", cellID, cell.State)
}
