// output.go：负责输出目录的只读可用性预检和执行期逐层创建，核对有效权限与目录句柄身份。

package strm

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
)

// OutputPreview 描述当前配置路径的可用性。
type OutputPreview struct {
	Status  string
	Message string
}

// outputRootError 携带管理员可见的输出目录状态，并保留底层错误供分类。
type outputRootError struct {
	preview OutputPreview
	cause   error
}

// Error 返回可安全展示的输出目录说明。
func (e *outputRootError) Error() string { return e.preview.Message }

// Unwrap 暴露底层原因供 errors.Is 和 errors.As 判断。
func (e *outputRootError) Unwrap() error { return e.cause }

// classifyOutputError 将目录缺失、类型冲突和有效权限问题转换为预检状态。
func classifyOutputError(err error) *outputRootError {
	preview := OutputPreview{Status: "unavailable", Message: "无法访问 STRM 输出目录，请检查路径、符号链接及挂载状态"}
	switch {
	case errors.Is(err, unix.ENOTDIR):
		preview = OutputPreview{Status: "not_directory", Message: "STRM 输出路径中存在非目录项，请修改输出目录"}
	case errors.Is(err, fs.ErrPermission), errors.Is(err, unix.EROFS):
		preview = OutputPreview{Status: "permission_denied", Message: "应用无法写入 STRM 输出目录，请检查权限和挂载是否允许写入"}
	case errors.Is(err, fs.ErrNotExist):
		preview = OutputPreview{Status: "missing", Message: "STRM 输出目录或上级目录不存在，请检查路径及挂载状态"}
	}
	return &outputRootError{preview: preview, cause: err}
}

// inspectOutputRoot 只读检查输出路径或最近已存在父目录，不创建探测文件或缺失目录。
func inspectOutputRoot(path string) OutputPreview {
	root, _, missing, err := findOutputAncestor(path)
	if err != nil {
		return classifyOutputError(err).preview
	}
	if err := root.Close(); err != nil {
		return classifyOutputError(err).preview
	}
	if len(missing) != 0 {
		return OutputPreview{Status: "will_create", Message: "输出目录尚未创建，全量同步执行时会自动创建"}
	}
	return OutputPreview{Status: "ready", Message: "输出目录可用"}
}

// prepareOutputRoot 从已核验父目录逐层创建输出路径，响应取消并复核路径绑定；成功句柄由调用者关闭。
func prepareOutputRoot(ctx context.Context, path string) (*os.Root, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	root, canonical, missing, err := findOutputAncestor(path)
	if err != nil {
		return nil, "", classifyOutputError(err)
	}
	// 仅从已打开的上级目录逐层创建，避免路径被替换为符号链接后写入其他位置。
	for _, name := range missing {
		if err := ctx.Err(); err != nil {
			return nil, "", errors.Join(err, root.Close())
		}
		next, err := openOutputSubdirectory(root, name, true)
		closeErr := root.Close()
		if err != nil {
			return nil, "", classifyOutputError(errors.Join(err, closeErr))
		}
		if closeErr != nil {
			return nil, "", classifyOutputError(errors.Join(closeErr, next.Close()))
		}
		root, canonical = next, filepath.Join(canonical, name)
	}
	// 目录句柄会跨重命名保留身份，生成前核对路径仍指向该目录。
	if err := verifyOutputBinding(root, path); err != nil {
		return nil, "", classifyOutputError(errors.Join(err, root.Close()))
	}
	if len(missing) != 0 {
		if err := checkOutputAccess(root); err != nil {
			return nil, "", classifyOutputError(errors.Join(err, root.Close()))
		}
	}
	return root, canonical, nil
}

// findOutputAncestor 寻找并打开最近已存在且可写的父目录，返回规范路径和待创建片段；拒绝断链及不安全穿越。
func findOutputAncestor(path string) (*os.Root, string, []string, error) {
	if !filepath.IsAbs(path) {
		return nil, "", nil, ErrConflict
	}
	hasParentTraversal := slices.Contains(strings.Split(path, string(filepath.Separator)), "..")
	if !hasParentTraversal {
		path = filepath.Clean(path)
	}
	var missing []string
	var info fs.FileInfo
	var err error
	candidate := path
	// Lstat 将断开的符号链接视为已存在的路径，避免误判为待创建目录。
	for {
		info, err = os.Lstat(candidate)
		if err == nil {
			break
		}
		parent := filepath.Dir(candidate)
		if !errors.Is(err, fs.ErrNotExist) || parent == candidate {
			return nil, "", nil, err
		}
		name := filepath.Base(candidate)
		if hasParentTraversal || !validPathPart(name) {
			return nil, "", nil, ErrConflict
		}
		missing = append(missing, name)
		candidate = parent
	}
	root, canonical, err := openOutputRoot(candidate)
	if err != nil {
		if info.Mode()&os.ModeSymlink != 0 && errors.Is(err, fs.ErrNotExist) {
			err = ErrConflict
		}
		return nil, "", nil, err
	}
	// 普通目录如果在解析时被换成链接，必须拒绝；原有别名则核对它仍指向已打开的目录。
	if info.IsDir() {
		err = matchOutputDirectory(root, info)
	} else {
		err = verifyOutputBinding(root, candidate)
	}
	if err != nil {
		return nil, "", nil, errors.Join(err, root.Close())
	}
	if err := checkOutputAccess(root); err != nil {
		return nil, "", nil, errors.Join(err, root.Close())
	}
	slices.Reverse(missing)
	return root, canonical, missing, nil
}

// checkOutputAccess 按服务进程有效身份检查目录写入与遍历权限，不进行测试写入。
func checkOutputAccess(root *os.Root) error {
	// 使用服务进程的有效身份检查目录，预览不创建探测文件。执行时仍以实际写入结果为准。
	handle, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(unix.Faccessat(int(handle.Fd()), ".", unix.W_OK|unix.X_OK, unix.AT_EACCESS), handle.Close())
}

// verifyOutputBinding 核验当前路径仍指向已打开的同一目录，允许已经确认的根路径别名。
func verifyOutputBinding(root *os.Root, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return matchOutputDirectory(root, info)
}
