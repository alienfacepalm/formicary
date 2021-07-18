package http

import (
	"context"
	"fmt"
	"plexobject.com/formicary/internal/utils/trace"
	"plexobject.com/formicary/internal/web"
	"sync"

	"plexobject.com/formicary/ants/config"
	"plexobject.com/formicary/ants/executor"
	"plexobject.com/formicary/internal/types"
)

// ExecutorProvider defines base structure for http executor provider
type ExecutorProvider struct {
	executor.BaseExecutorProvider
	client    web.HTTPClient
	executors map[string]*Executor
	lock      sync.RWMutex
}

// NewExecutorProvider creates executor-provider for local http based execution
func NewExecutorProvider(
	config *config.AntConfig,
	client web.HTTPClient) (executor.Provider, error) {
	return &ExecutorProvider{
		BaseExecutorProvider: executor.BaseExecutorProvider{
			AntConfig: config,
		},
		client:    client,
		executors: make(map[string]*Executor),
	}, nil
}

// ListExecutors lists current executors
func (sep *ExecutorProvider) ListExecutors(context.Context) ([]executor.Info, error) {
	sep.lock.RLock()
	defer sep.lock.RUnlock()
	execs := make([]executor.Info, 0)
	for _, e := range sep.executors {
		execs = append(execs, e)
	}
	return execs, nil
}

// AllRunningExecutors returns running executors
func (sep *ExecutorProvider) AllRunningExecutors(ctx context.Context) ([]executor.Info, error) {
	return sep.ListExecutors(ctx)
}

// StopExecutor stops executor
func (sep *ExecutorProvider) StopExecutor(
	_ context.Context,
	id string,
	_ *types.ExecutorOptions) error {
	sep.lock.Lock()
	defer sep.lock.Unlock()
	exec := sep.executors[id]
	if exec == nil {
		return fmt.Errorf("failed to find executor with id %v", id)
	}
	delete(sep.executors, id)
	return exec.Stop()
}

// NewExecutor creates new executor
func (sep *ExecutorProvider) NewExecutor(
	_ context.Context,
	trace trace.JobTrace,
	opts *types.ExecutorOptions) (executor.Executor, error) {
	sep.lock.Lock()
	defer sep.lock.Unlock()
	exec, err := NewHTTPExecutor(sep.AntConfig, trace, sep.client, opts)
	if err != nil {
		return nil, err
	}
	sep.executors[exec.ID] = exec
	return exec, nil
}
