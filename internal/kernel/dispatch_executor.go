package kernel

import (
	"context"
	"errors"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// DispatchExecutor routes a READY operation to Local, Web, or Model executors.
// Local path is preferred when eligible so continuity never pays model/web cost.
// Web path handles web.search/web.fetch under ResourceGate when Web is set.
// When Model is nil, non-local non-web ops stay skipped as requires_model.
type DispatchExecutor struct {
	Store port.Store
	Local LocalExecutor
	Web   *WebExecutor   // optional
	Model *ModelExecutor // optional
}

// Execute chooses the path from the operation's stored OperationSpec.
func (d DispatchExecutor) Execute(ctx context.Context, operationID domain.OperationID) (ExecuteResult, error) {
	if d.Store == nil {
		return ExecuteResult{}, errors.New("dispatch executor requires a store")
	}
	if operationID == "" {
		return ExecuteResult{}, errors.New("operation id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var (
		spec    domain.OperationSpec
		opState domain.OperationalState
	)
	err := d.Store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation(operationID)
		if err != nil {
			return err
		}
		opState = op.State
		if op.State.Terminal() || op.State != domain.StateReady {
			return nil
		}
		loaded, err := r.OperationSpec(op.SpecID)
		if err != nil {
			return err
		}
		spec = loaded
		return nil
	})
	if err != nil {
		return ExecuteResult{}, err
	}
	if opState.Terminal() {
		return ExecuteResult{OperationID: operationID, Skipped: true, SkipReason: "terminal"}, nil
	}
	if opState != domain.StateReady {
		return ExecuteResult{OperationID: operationID, Skipped: true, SkipReason: "not_ready"}, nil
	}

	if LocalEligible(spec) {
		return d.Local.Execute(ctx, operationID)
	}
	if d.Web != nil && WebEligible(spec) {
		webResult, err := d.Web.Execute(ctx, operationID)
		if err != nil {
			return ExecuteResult{OperationID: operationID, LeaseRef: webResult.LeaseRef}, err
		}
		return ExecuteResult{
			OperationID: webResult.OperationID,
			Completed:   webResult.Completed,
			Skipped:     webResult.Skipped,
			SkipReason:  webResult.SkipReason,
			LeaseRef:    webResult.LeaseRef,
			ArtifactID:  webResult.ArtifactID,
		}, nil
	}
	if WebEligible(spec) && d.Web == nil {
		return ExecuteResult{OperationID: operationID, Skipped: true, SkipReason: "requires_web"}, nil
	}
	if d.Model != nil && ModelEligible(spec) {
		modelResult, err := d.Model.Execute(ctx, operationID)
		if err != nil {
			return ExecuteResult{OperationID: operationID, LeaseRef: modelResult.LeaseRef}, err
		}
		return ExecuteResult{
			OperationID: modelResult.OperationID,
			Completed:   modelResult.Completed,
			Skipped:     modelResult.Skipped,
			SkipReason:  modelResult.SkipReason,
			LeaseRef:    modelResult.LeaseRef,
		}, nil
	}
	return ExecuteResult{OperationID: operationID, Skipped: true, SkipReason: "requires_model"}, nil
}
