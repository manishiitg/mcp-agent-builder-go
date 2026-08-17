//go:build linux

package security

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const landlockBaseFSAccess = unix.LANDLOCK_ACCESS_FS_EXECUTE |
	unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
	unix.LANDLOCK_ACCESS_FS_READ_FILE |
	unix.LANDLOCK_ACCESS_FS_READ_DIR |
	unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
	unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
	unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
	unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
	unix.LANDLOCK_ACCESS_FS_MAKE_REG |
	unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
	unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_SYM

func landlockHandledFS(abi int) uint64 {
	rights := uint64(landlockBaseFSAccess)
	if abi >= 2 {
		rights |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		rights |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 5 {
		rights |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return rights
}

func landlockReadFS(handled uint64) uint64 {
	return handled & (unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR)
}

// RunLandlockLauncher applies policy to the current process and replaces it
// with argv. It is called only by the dedicated launcher binary.
func RunLandlockLauncher(policy LandlockPolicy, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("missing command")
	}
	abi, err := landlockABI()
	if err != nil || abi < 1 {
		return fmt.Errorf("SANDBOX_UNAVAILABLE: Landlock filesystem ABI unavailable: %w", err)
	}
	handled := landlockHandledFS(abi)
	attr := unix.LandlockRulesetAttr{Access_fs: handled}
	rulesetFD, _, errno := unix.Syscall6(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
		0,
		0,
		0,
	)
	if errno != 0 {
		return fmt.Errorf("SANDBOX_UNAVAILABLE: create Landlock ruleset: %w", errno)
	}
	defer unix.Close(int(rulesetFD))

	readAccess := landlockReadFS(handled)
	writeAccess := handled
	for _, path := range landlockSystemReadPaths() {
		if err := addLandlockPathRule(int(rulesetFD), path, readAccess); err != nil {
			return err
		}
	}
	for _, path := range landlockSystemWritePaths() {
		if err := addLandlockPathRule(int(rulesetFD), path, writeAccess); err != nil {
			return err
		}
	}
	for _, path := range policy.ReadPaths {
		if err := addLandlockPathRule(int(rulesetFD), path, readAccess); err != nil {
			return err
		}
	}
	for _, path := range policy.WritePaths {
		if err := addLandlockPathRule(int(rulesetFD), path, writeAccess); err != nil {
			return err
		}
	}

	if err := os.Chdir(policy.WorkDir); err != nil {
		return fmt.Errorf("SANDBOX_UNAVAILABLE: enter working directory: %w", err)
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("SANDBOX_UNAVAILABLE: set no_new_privs: %w", err)
	}
	_, _, errno = unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, rulesetFD, 0, 0)
	if errno != 0 {
		return fmt.Errorf("SANDBOX_UNAVAILABLE: enforce Landlock ruleset: %w", errno)
	}
	if err := syscall.Exec(argv[0], argv, os.Environ()); err != nil {
		return fmt.Errorf("execute sandboxed command: %w", err)
	}
	return nil
}

func addLandlockPathRule(rulesetFD int, path string, allowed uint64) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("SANDBOX_UNAVAILABLE: inspect Landlock path: %w", err)
	}
	if !info.IsDir() {
		allowed &= ^uint64(unix.LANDLOCK_ACCESS_FS_READ_DIR |
			unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
			unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
			unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
			unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
			unix.LANDLOCK_ACCESS_FS_MAKE_REG |
			unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
			unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
			unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
			unix.LANDLOCK_ACCESS_FS_MAKE_SYM |
			unix.LANDLOCK_ACCESS_FS_REFER)
	}
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("SANDBOX_UNAVAILABLE: open Landlock path: %w", err)
	}
	defer unix.Close(fd)
	rule := unix.LandlockPathBeneathAttr{Allowed_access: allowed, Parent_fd: int32(fd)}
	_, _, errno := unix.Syscall6(
		unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFD),
		unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&rule)),
		0,
		0,
		0,
	)
	if errno != 0 {
		return fmt.Errorf("SANDBOX_UNAVAILABLE: add Landlock path rule: %w", errno)
	}
	return nil
}

func landlockSystemReadPaths() []string {
	return existingCanonicalPaths([]string{
		"/bin", "/sbin", "/usr", "/lib", "/lib64",
		"/etc/ssl", "/etc/ca-certificates", "/etc/resolv.conf", "/etc/hosts",
		"/etc/nsswitch.conf", "/etc/passwd", "/etc/group", "/etc/localtime",
		"/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/ld.so.conf.d",
		"/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom", "/dev/tty",
	})
}

func landlockSystemWritePaths() []string {
	return existingCanonicalPaths([]string{
		"/tmp",
		"/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom", "/dev/tty",
	})
}

func existingCanonicalPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		resolved := path
		if canonical, err := filepath.EvalSymlinks(path); err == nil {
			resolved = canonical
		}
		if _, err := os.Stat(resolved); err != nil {
			continue
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		result = append(result, resolved)
	}
	return result
}
