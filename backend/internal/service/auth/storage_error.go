// storage_error.go：封装白名单式认证存储诊断，同时保留底层错误身份供内部分类。

package auth

// StorageError 仅暴露允许的操作和原因；底层驱动错误可被 errors.Is/As 检查，但不应写入日志。
type StorageError struct {
	Operation string
	Reason    string
	Cause     error
}

// Error 返回操作和白名单原因组成的诊断，不渲染底层驱动错误。
func (e *StorageError) Error() string { return e.Operation + ": " + e.Reason }

// Unwrap 保留底层错误链，供取消、超时和存储原因分类使用。
func (e *StorageError) Unwrap() error { return e.Cause }
