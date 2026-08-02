package common

// LocalizedError carries a stable translation key across service boundaries.
// The API layer translates it using the request language while preserving the
// wrapped cause for errors.Is/errors.As and server-side diagnostics.
type LocalizedError struct {
	Key   string
	Args  map[string]any
	Cause error
}

func NewLocalizedError(key string, args ...map[string]any) error {
	localized := &LocalizedError{Key: key}
	if len(args) > 0 {
		localized.Args = args[0]
	}
	return localized
}

func WrapLocalizedError(cause error, key string, args ...map[string]any) error {
	localized := &LocalizedError{Key: key, Cause: cause}
	if len(args) > 0 {
		localized.Args = args[0]
	}
	return localized
}

func (e *LocalizedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Key
}

func (e *LocalizedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
