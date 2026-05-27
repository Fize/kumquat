package safe

import (
	"fmt"
	"runtime/debug"

	ctrl "sigs.k8s.io/controller-runtime"
)

// Go starts a goroutine with panic recovery. If the function panics,
// the panic is logged with a stack trace instead of crashing the process.
func Go(fn func()) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				logger := ctrl.Log.WithName("safe.Go")
				logger.Error(fmt.Errorf("panic in goroutine: %v", err),
					"goroutine panicked",
					"panic", fmt.Sprintf("%v", err),
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
