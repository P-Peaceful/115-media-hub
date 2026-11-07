package auth

// StorageError exposes only allowlisted operational diagnostics. Its underlying
// driver error remains inspectable with errors.Is/As but is never rendered in logs.
type StorageError struct {
	Operation string
	Reason    string
	Cause     error
}

func (e *StorageError) Error() string { return e.Operation + ": " + e.Reason }
func (e *StorageError) Unwrap() error { return e.Cause }
