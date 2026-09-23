package simulatoradmin

import (
	"context"
	"net/http"
	"time"
)

func contextWithTimeout(request *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = time.Second
	}
	return context.WithTimeout(request.Context(), timeout)
}
