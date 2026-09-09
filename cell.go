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
	nextToolSequence  int
}

type CellState string

const (
	CellRunning   CellState = "running"
	CellCompleted CellState = "completed"
	CellCancelled CellState = "cancelled"
)

// CellManager owns cell identity and issues nested calls. A later lesson will
// add yielded cells and a real runtime; this version is deliberately in-process.
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

func (m *CellManager) Complete(cellID string) error {
	return m.transition(cellID, CellCompleted)
}

func (m *CellManager) Cancel(cellID string) error {
	return m.transition(cellID, CellCancelled)
}

func (m *CellManager) transition(cellID string, next CellState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cell, ok := m.cells[cellID]
	if !ok {
		return fmt.Errorf("unknown cell: %s", cellID)
	}
	if cell.State != CellRunning {
		return fmt.Errorf("cell %s is already %s", cellID, cell.State)
	}
	cell.State = next
	return nil
}
