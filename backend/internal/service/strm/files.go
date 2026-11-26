// files.go：负责受根目录句柄约束的路径校验、内容读取和文件原子发布，检测路径替换及覆盖竞态。

package strm

import (
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// maxSTRMContent 限制登记文件的最大读取和生成长度，避免将任意大文件作为 STRM 处理。
const maxSTRMContent = 16 * 1024

// validPathPart 拒绝空名、点路径、分隔符、控制字符和超长或非法 UTF-8 名称。
func validPathPart(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 255 && utf8.ValidString(name) &&
		!strings.ContainsAny(name, `/\`) && strings.IndexFunc(name, unicode.IsControl) == -1
}

// validArtifactPath 校验规范的本地相对 .strm 路径，逐层拒绝非法路径片段。
func validArtifactPath(relative string) bool {
	return filepath.Ext(relative) == ".strm" && validOutputPath(relative)
}

// validOutputPath 校验规范的本地相对路径，供 STRM 与附件共用逐层安全约束。
func validOutputPath(relative string) bool {
	if !filepath.IsLocal(relative) || filepath.Clean(relative) != relative {
		return false
	}
	for part := range strings.SplitSeq(relative, string(filepath.Separator)) {
		if !validPathPart(part) {
			return false
		}
	}
	return true
}

// openOutputRoot 解析已存在的绝对输出路径别名并核对目录身份；返回句柄由调用者关闭。
func openOutputRoot(path string) (*os.Root, string, error) {
	if !filepath.IsAbs(path) {
		return nil, "", ErrConflict
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, "", err
	}
	root, err := openIdentifiedDirectory(os.Lstat, os.OpenRoot, canonical)
	if err != nil {
		return nil, "", err
	}
	return root, canonical, nil
}

// openIdentifiedDirectory 先 Lstat 再打开并核验同一目录，失败时关闭新句柄，成功后转交调用者。
func openIdentifiedDirectory(lstat func(string) (fs.FileInfo, error), open func(string) (*os.Root, error), name string) (*os.Root, error) {
	info, err := lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.Join(ErrConflict, unix.ENOTDIR)
	}
	directory, err := open(name)
	if err != nil {
		return nil, err
	}
	if err := matchOutputDirectory(directory, info); err != nil {
		return nil, errors.Join(err, directory.Close())
	}
	return directory, nil
}

// matchOutputDirectory 比较目录句柄和预检查文件信息，检测路径被替换或指向非目录。
func matchOutputDirectory(root *os.Root, info fs.FileInfo) error {
	actual, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !info.IsDir() || !os.SameFile(info, actual) {
		return ErrConflict
	}
	return nil
}

// openArtifactDirectory 沿受限根句柄逐层打开产物父目录，按需创建并关闭中间句柄；不接受子目录链接。
func openArtifactDirectory(root *os.Root, relative string, create bool) (*os.Root, error) {
	if !validArtifactPath(relative) {
		return nil, ErrConflict
	}
	return openOutputDirectory(root, relative, create)
}

// openOutputDirectory 逐层打开已校验相对路径的父目录，所有子目录必须为真实目录。
func openOutputDirectory(root *os.Root, relative string, create bool) (*os.Root, error) {
	if !validOutputPath(relative) {
		return nil, ErrConflict
	}
	directory, err := root.OpenRoot(".")
	if err != nil || filepath.Dir(relative) == "." {
		return directory, err
	}
	// 每层核验真实目录后使用目录句柄继续访问，把路径别名排除在归属判断之外。
	for name := range strings.SplitSeq(filepath.Dir(relative), string(filepath.Separator)) {
		next, err := openOutputSubdirectory(directory, name, create)
		closeErr := directory.Close()
		if err != nil {
			return nil, errors.Join(err, closeErr)
		}
		if closeErr != nil {
			return nil, errors.Join(closeErr, next.Close())
		}
		directory = next
	}
	return directory, nil
}

// openOutputSubdirectory 核验真实子目录；允许创建时同步父目录并重新检查新目录身份。
func openOutputSubdirectory(parent *os.Root, name string, create bool) (*os.Root, error) {
	directory, err := openIdentifiedDirectory(parent.Lstat, parent.OpenRoot, name)
	if !create || !errors.Is(err, fs.ErrNotExist) {
		return directory, err
	}
	if err := parent.Mkdir(name, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	if err := syncDirectory(parent); err != nil {
		return nil, err
	}
	return openIdentifiedDirectory(parent.Lstat, parent.OpenRoot, name)
}

// syncDirectory 同步目录元数据并关闭临时文件句柄，将两者错误一并返回。
func syncDirectory(directory *os.Root) error {
	file, err := directory.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

// readArtifactFile 通过已校验的父目录句柄读取单个 STRM，负责关闭父目录句柄。
func readArtifactFile(root *os.Root, relative string) (_ string, resultErr error) {
	directory, err := openArtifactDirectory(root, relative, false)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	return readArtifactLeaf(directory, filepath.Base(relative))
}

// verifyArtifactDirectory 复核根路径和产物父目录仍对应已打开句柄，防止改名后的旧目录被误登记。
func verifyArtifactDirectory(root, directory *os.Root, canonical, relative string) (resultErr error) {
	if !validArtifactPath(relative) {
		return ErrConflict
	}
	return verifyOutputDirectory(root, directory, canonical, relative)
}

// verifyOutputDirectory 复核根路径及相对父目录的句柄身份，避免向被移动的旧目录发布。
func verifyOutputDirectory(root, directory *os.Root, canonical, relative string) (resultErr error) {
	// Root 句柄会继续指向改名后的目录。登记前确认此句柄仍对应登记路径。
	rootInfo, err := os.Lstat(canonical)
	if err != nil {
		return err
	}
	if err := matchOutputDirectory(root, rootInfo); err != nil {
		return err
	}
	current, err := openOutputDirectory(root, relative, false)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, current.Close()) }()
	currentInfo, err := current.Stat(".")
	if err != nil {
		return err
	}
	openedInfo, err := directory.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(currentInfo, openedInfo) {
		return ErrConflict
	}
	return nil
}

// readArtifactLeaf 有界读取同一普通文件，禁用末级链接和阻塞打开，拒绝替换竞态及超长内容。
func readArtifactLeaf(directory *os.Root, name string) (_ string, resultErr error) {
	info, err := directory.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSTRMContent {
		return "", ErrConflict
	}
	// 禁止跟随末级链接。使用非阻塞打开，避免文件被替换成管道时阻塞任务。
	file, err := directory.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ELOOP) {
		return "", ErrConflict
	}
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	actual, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return "", ErrConflict
	}
	content, err := io.ReadAll(io.LimitReader(file, maxSTRMContent+1))
	if len(content) > maxSTRMContent {
		return "", ErrConflict
	}
	return string(content), err
}

// existingContent 读取已有内容；仅在文件不存在时返回空指针，其他异常保持错误语义。
func existingContent(directory *os.Root, name string) (*string, error) {
	content, err := readArtifactLeaf(directory, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &content, nil
}

// publishArtifact 先写入并同步临时文件，再以硬链接或原子交换发布；覆盖歧义时保留暂存内容。
func publishArtifact(directory *os.Root, name string, previous *string, content string) (resultErr error) {
	temporary := ".strm-" + rand.Text() + ".tmp"
	file, err := directory.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	removeTemporary := true
	defer func() {
		if !removeTemporary {
			return
		}
		if err := directory.Remove(temporary); err != nil && !errors.Is(err, fs.ErrNotExist) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	_, writeErr := file.WriteString(content)
	if writeErr != nil {
		return errors.Join(writeErr, file.Close())
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	// 初次生成用硬链接原子发布。覆盖使用原子交换，先保留实际被替换的数据。
	if previous == nil {
		if err := directory.Link(temporary, name); errors.Is(err, fs.ErrExist) {
			return ErrConflict
		} else if err != nil {
			return err
		}
	} else {
		removeTemporary, err = replaceArtifact(directory, temporary, name, *previous, content)
		if err != nil {
			return err
		}
	}
	return syncDirectory(directory)
}

// replaceArtifact 校验原内容后原子交换目标与临时文件，并核对被置换内容；必要时尝试恢复。
// 返回值指示调用者是否可删除临时文件，歧义时必须保留以免丢失并发写入。
func replaceArtifact(directory *os.Root, temporary, name, previous, content string) (_ bool, resultErr error) {
	current, err := existingContent(directory, name)
	if err != nil {
		return true, err
	}
	if current == nil || *current != previous {
		return true, ErrConflict
	}
	handle, err := directory.Open(".")
	if err != nil {
		return true, err
	}
	defer func() { resultErr = errors.Join(resultErr, handle.Close()) }()
	fd := int(handle.Fd())
	exchange := func() error {
		return unix.Renameat2(fd, temporary, fd, name, unix.RENAME_EXCHANGE)
	}
	if err := exchange(); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, ErrConflict
		}
		return true, err
	}
	displaced, err := existingContent(directory, temporary)
	if err == nil && displaced != nil && *displaced == previous {
		return true, nil
	}

	// 文件可能在预检查后被替换过。恢复原路径时保留暂存文件，避免覆盖并发写入。
	current, currentErr := existingContent(directory, name)
	if currentErr != nil || current == nil || *current != content {
		return false, errors.Join(ErrConflict, err, currentErr, syncDirectory(directory))
	}
	return false, errors.Join(ErrConflict, err, exchange(), syncDirectory(directory))
}
