package tasklet

import (
	"fmt"
	"plexobject.com/formicary/internal/metrics"
	"sync"

	"plexobject.com/formicary/internal/types"
)

// RequestRegistry interface for keeping track of current jobs
type RequestRegistry interface {
	Add(
		req *types.TaskRequest) error
	Cancel(
		key string) error
	CancelJob(
		requestID uint64) error
	Remove(
		req *types.TaskRequest) error
	Count() int
	GetAllocations() (allocations map[uint64]*types.AntAllocation)
}

// RequestRegistryImpl keeps track of in-progress jobs
type RequestRegistryImpl struct {
	commonConfig    *types.CommonConfig
	metricsRegistry *metrics.Registry
	requests        map[string]*types.TaskRequest
	lock            sync.RWMutex
}

// NewRequestRegistry constructor
func NewRequestRegistry(
	commonConfig *types.CommonConfig,
	metricsRegistry *metrics.Registry,
) RequestRegistry {
	return &RequestRegistryImpl{
		commonConfig:    commonConfig,
		metricsRegistry: metricsRegistry,
		requests:        make(map[string]*types.TaskRequest),
	}
}

// Add - adds request
func (r *RequestRegistryImpl) Add(
	req *types.TaskRequest) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	if req == nil {
		return fmt.Errorf("request not specified")
	}
	if req.Key() == "" {
		return fmt.Errorf("request task key not specified")
	}
	if r.requests[req.Key()] != nil {
		return fmt.Errorf("request task key not specified")

	}
	req.Status = types.READY
	r.requests[req.Key()] = req
	return nil
}

// Cancel cancels the request
func (r *RequestRegistryImpl) Cancel(
	key string) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	if key == "" {
		return fmt.Errorf("request task key not specified")
	}
	req := r.requests[key]
	if req == nil {
		return fmt.Errorf("request not found")
	}
	if req.Cancel == nil {
		return fmt.Errorf("request cancel not found")
	}
	req.Cancel()
	req.Cancelled = true
	return nil
}

// CancelJob cancels the request by job ID
func (r *RequestRegistryImpl) CancelJob(
	requestID uint64) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	for _, req := range r.requests {
		if req.JobRequestID == requestID && req.Cancel != nil {
			//debug.PrintStack()
			req.Cancel()
			delete(r.requests, req.Key())
			break
		}
	}
	return nil
}

// Remove - removes request
func (r *RequestRegistryImpl) Remove(
	req *types.TaskRequest) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	if req == nil {
		return fmt.Errorf("request not specified")
	}
	if req.Key() == "" {
		return fmt.Errorf("request task key not specified")
	}
	if r.requests[req.Key()] == nil {
		return fmt.Errorf("request task key not found")

	}
	delete(r.requests, req.Key())
	return nil
}

// Count - number of requests
func (r *RequestRegistryImpl) Count() int {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return len(r.requests)
}

// GetAllocations - returns allocations of requests
func (r *RequestRegistryImpl) GetAllocations() (allocations map[uint64]*types.AntAllocation) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	allocations = make(map[uint64]*types.AntAllocation)
	for _, req := range r.requests {
		allocations[req.JobRequestID] = types.NewAntAllocation(
			r.commonConfig.ID,
			r.commonConfig.GetRequestTopic(),
			req.JobRequestID,
			req.TaskType,
		)
	}
	return
}
