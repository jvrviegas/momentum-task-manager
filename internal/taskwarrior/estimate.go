package taskwarrior

import (
	"context"
	"fmt"
	"strings"

	"github.com/jvrviegas/momentum/internal/domain"
)

// EstimateUDAState describes the read-only Taskwarrior configuration probe.
type EstimateUDAState string

const (
	EstimateUDAConfigured  EstimateUDAState = "configured"
	EstimateUDAMissing     EstimateUDAState = "missing"
	EstimateUDAWrongType   EstimateUDAState = "wrong_type"
	EstimateUDAUnavailable EstimateUDAState = "unavailable"
)

const estimateUDASetup = "add these lines to every Taskwarrior client configuration:\nuda.estimate.type=duration\nuda.estimate.label=Estimate"

// EstimateUDAError is returned before an estimate mutation when Momentum
// cannot prove that Taskwarrior has the required duration UDA.
type EstimateUDAError struct {
	State EstimateUDAState
	Type  string
	Cause error
}

func (e *EstimateUDAError) Error() string {
	if e == nil {
		return "estimate UDA readiness is unknown"
	}
	switch e.State {
	case EstimateUDAMissing:
		return "estimate UDA is not configured; " + estimateUDASetup + "; no estimate mutation was attempted"
	case EstimateUDAWrongType:
		return fmt.Sprintf("estimate UDA has type %q, but Momentum requires duration; %s; no estimate mutation was attempted", e.Type, estimateUDASetup)
	case EstimateUDAUnavailable:
		if e.Cause != nil {
			return "could not verify estimate UDA readiness: " + e.Cause.Error() + "; no estimate mutation was attempted"
		}
		return "could not verify estimate UDA readiness; no estimate mutation was attempted"
	default:
		if e.Cause != nil {
			return e.Cause.Error()
		}
		return "estimate UDA is not ready"
	}
}

func (e *EstimateUDAError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// EstimateUDAReadiness performs the single safe configuration query used by
// estimate-bearing mutations and doctor. It never reads task content.
func (c *CommandClient) EstimateUDAReadiness(ctx context.Context) (EstimateUDAState, error) {
	result, err := c.run(ctx, "estimate-uda", "_get", "rc.uda.estimate.type")
	if err != nil {
		return EstimateUDAUnavailable, &EstimateUDAError{State: EstimateUDAUnavailable, Cause: err}
	}
	typeName := strings.TrimSpace(result.Stdout)
	if typeName == "" {
		return EstimateUDAMissing, &EstimateUDAError{State: EstimateUDAMissing}
	}
	if typeName != "duration" {
		return EstimateUDAWrongType, &EstimateUDAError{State: EstimateUDAWrongType, Type: typeName}
	}
	return EstimateUDAConfigured, nil
}

func validateNewEstimate(value *domain.Estimate) error {
	if value == nil {
		return nil
	}
	if err := value.ValidateForCreation(); err != nil {
		return fmt.Errorf("invalid estimate: %w", err)
	}
	if value.TaskwarriorValue() == "" {
		return fmt.Errorf("invalid estimate: cannot serialize Taskwarrior value")
	}
	return nil
}

func validateEstimateChange(change domain.EstimateChange) error {
	if change.Kind != domain.Set {
		return nil
	}
	if change.Value == nil {
		return fmt.Errorf("invalid estimate change: set value is missing")
	}
	return validateNewEstimate(change.Value)
}
