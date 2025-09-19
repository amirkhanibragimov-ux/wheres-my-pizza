package kitchenworker

import "errors"

// Retryable error wrapper to signal requeue=true on broker.
type retryableError struct {
	err error
}

// Error returns the wrapped error's message.
func (e retryableError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e retryableError) Unwrap() error {
	return e.err
}

// Retryable wraps an error to mark it as retryable (requeue=true).
func Retryable(err error) error {
	if err == nil {
		return nil
	}

	return retryableError{err: err}
}

// IsRetryable returns true if the error is marked as retryable.
func IsRetryable(err error) bool {
	var r retryableError
	return errors.As(err, &r)
}
