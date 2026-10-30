//go:build linux

package server

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// SandboxArg is argv[1] of the backend binary when it runs as the sandbox
// helper: the step between fork and exec of tuisense that confines the
// process. It is never reachable from a visitor's request.
const SandboxArg = "__tuisense_sandbox"

// sandboxPlan is what the server applies to every session process.
type sandboxPlan struct {
	Namespaces bool // user, PID, network, IPC and UTS namespaces
	Landlock   int  // Landlock ABI to require (0 = not used)
	Mode       SandboxMode
}

func (p sandboxPlan) String() string {
	return fmt.Sprintf("mode=%s namespaces=%v landlock_abi=%d", p.Mode, p.Namespaces, p.Landlock)
}

// landlockABI returns the kernel's Landlock ABI version (0 = unavailable).
func landlockABI() int {
	v, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0
	}
	return int(v)
}

// namespaceAttrs isolates a session process from the host and from other
// sessions: its own PID space (it cannot see or signal other processes),
// no network interfaces at all, and its own IPC and hostname. The user
// namespace maps only the server's own user, so it gains no privilege.
func namespaceAttrs(attr *syscall.SysProcAttr) {
	uid, gid := os.Getuid(), os.Getgid()
	attr.Cloneflags = syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC | syscall.CLONE_NEWUTS
	attr.UidMappings = []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}}
	attr.GidMappings = []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}}
	attr.GidMappingsEnableSetgroups = false
}

// probeSandbox decides the plan for mode by starting the helper once.
func probeSandbox(mode SandboxMode, self string) (sandboxPlan, error) {
	plan := sandboxPlan{Mode: mode}
	if mode == SandboxMinimal {
		return plan, nil
	}
	abi := landlockABI()
	ns := runProbe(self, true, abi) == nil
	if !ns && runProbe(self, false, abi) != nil {
		abi = 0 // the helper itself could not apply Landlock
	}
	plan.Namespaces, plan.Landlock = ns, abi
	switch {
	case mode == SandboxStrict && (!ns || abi == 0):
		return plan, fmt.Errorf("strict sandbox unavailable (namespaces=%v, landlock ABI=%d)", ns, abi)
	case mode == SandboxLandlock && abi == 0:
		return plan, errors.New("landlock sandbox unavailable: the kernel or seccomp profile does not allow Landlock")
	}
	return plan, nil
}

func runProbe(self string, namespaces bool, abi int) error {
	args := []string{self, SandboxArg, "--probe", "--landlock=" + strconv.Itoa(abi), "--rw=" + os.TempDir(), "--bin=" + self, "--"}
	cmd := &exec.Cmd{Path: self, Args: args, Env: []string{}, SysProcAttr: &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}}
	if namespaces {
		namespaceAttrs(cmd.SysProcAttr)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	defer r.Close()
	cmd.ExtraFiles = []*os.File{w}
	err = cmd.Start()
	w.Close()
	if err != nil {
		return err
	}
	status, rerr := readStatus(r, 5*time.Second)
	werr := cmd.Wait()
	if rerr != nil {
		return rerr
	}
	if !strings.HasPrefix(status, "ok") {
		return errors.New(status)
	}
	return werr
}

// readStatus reads the helper's one-line report from its status pipe.
func readStatus(r *os.File, timeout time.Duration) (string, error) {
	_ = r.SetReadDeadline(time.Now().Add(timeout))
	line, err := bufio.NewReader(r).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" && err != nil {
		return "", fmt.Errorf("sandbox helper did not report: %w", err)
	}
	return line, nil
}

// helperArgs are the helper's fixed arguments for one session.
type helperArgs struct {
	Landlock  int
	MemBytes  uint64
	CPUSec    uint64
	NoFile    uint64
	FSize     uint64
	Nice      int
	RW        string // the session's private directory
	Binary    string
	TuiSense  []string // argv of tuisense (argv[0] included)
	ProbeOnly bool
}

func (h helperArgs) argv(self string) []string {
	a := []string{self, SandboxArg,
		"--landlock=" + strconv.Itoa(h.Landlock),
		"--mem=" + strconv.FormatUint(h.MemBytes, 10),
		"--cpu=" + strconv.FormatUint(h.CPUSec, 10),
		"--nofile=" + strconv.FormatUint(h.NoFile, 10),
		"--fsize=" + strconv.FormatUint(h.FSize, 10),
		"--nice=" + strconv.Itoa(h.Nice),
		"--rw=" + h.RW,
		"--bin=" + h.Binary,
		"--"}
	return append(a, h.TuiSense...)
}

func parseHelperArgs(args []string) (helperArgs, error) {
	var h helperArgs
	for i, a := range args {
		if a == "--" {
			h.TuiSense = args[i+1:]
			return h, nil
		}
		k, v, _ := strings.Cut(a, "=")
		var err error
		switch k {
		case "--probe":
			h.ProbeOnly = true
		case "--landlock":
			h.Landlock, err = strconv.Atoi(v)
		case "--mem":
			h.MemBytes, err = strconv.ParseUint(v, 10, 64)
		case "--cpu":
			h.CPUSec, err = strconv.ParseUint(v, 10, 64)
		case "--nofile":
			h.NoFile, err = strconv.ParseUint(v, 10, 64)
		case "--fsize":
			h.FSize, err = strconv.ParseUint(v, 10, 64)
		case "--nice":
			h.Nice, err = strconv.Atoi(v)
		case "--rw":
			h.RW = v
		case "--bin":
			h.Binary = v
		default:
			err = fmt.Errorf("unknown argument %q", a)
		}
		if err != nil {
			return h, err
		}
	}
	return h, errors.New("missing --")
}

// statusFD is the helper's report pipe (cmd.ExtraFiles[0]).
const statusFD = 3

// SandboxMain is the helper's entry point (argv[1] == SandboxArg). It
// confines the current process and then executes tuisense in place, or
// exits with status 1 after reporting why it could not.
func SandboxMain(args []string) int {
	status := os.NewFile(statusFD, "status")
	fail := func(err error) int {
		if status != nil {
			fmt.Fprintf(status, "error: %v\n", err)
		}
		return 1
	}
	h, err := parseHelperArgs(args)
	if err != nil {
		return fail(err)
	}
	// Every step that is per-thread (no_new_privs, Landlock) must happen on
	// the thread that finally calls execve.
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fail(fmt.Errorf("no_new_privs: %w", err))
	}
	dropCapabilities()
	if !h.ProbeOnly {
		if err := applyRlimits(h); err != nil {
			return fail(err)
		}
		if h.Nice > 0 {
			_ = unix.Setpriority(unix.PRIO_PROCESS, 0, h.Nice)
		}
	}
	if h.Landlock > 0 {
		if err := applyLandlock(h); err != nil {
			return fail(fmt.Errorf("landlock: %w", err))
		}
	}
	if h.ProbeOnly {
		if status != nil {
			fmt.Fprintf(status, "ok probe pid=%d\n", os.Getpid())
		}
		return 0
	}
	if len(h.TuiSense) == 0 || h.TuiSense[0] == "" {
		return fail(errors.New("no program"))
	}
	if status != nil {
		fmt.Fprintf(status, "ok pid=%d landlock=%d\n", os.Getpid(), h.Landlock)
		status.Close()
	}
	// The environment was prepared by the server; nothing is inherited.
	err = unix.Exec(h.Binary, h.TuiSense, os.Environ())
	return fail(fmt.Errorf("exec %s: %w", h.Binary, err))
}

// dropCapabilities empties the bounding, ambient and current capability
// sets, so tuisense runs without any capability even when the helper
// holds some (inside its user namespace, or a misconfigured root run).
// Steps the kernel refuses for lack of privilege leave nothing to drop.
func dropCapabilities() {
	_ = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0)
	for c := 0; c <= unix.CAP_LAST_CAP; c++ {
		_ = unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(c), 0, 0, 0)
	}
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	_ = unix.Capset(&hdr, &data[0])
}

func applyRlimits(h helperArgs) error {
	set := func(res int, v uint64, name string) error {
		if v == 0 {
			return nil
		}
		if err := unix.Setrlimit(res, &unix.Rlimit{Cur: v, Max: v}); err != nil {
			return fmt.Errorf("setrlimit %s: %w", name, err)
		}
		return nil
	}
	// No core dumps: a crash must never write session memory to disk.
	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{}); err != nil {
		return fmt.Errorf("setrlimit core: %w", err)
	}
	for _, l := range []struct {
		res  int
		v    uint64
		name string
	}{
		{unix.RLIMIT_AS, h.MemBytes, "as"},
		{unix.RLIMIT_CPU, h.CPUSec, "cpu"},
		{unix.RLIMIT_NOFILE, h.NoFile, "nofile"},
		{unix.RLIMIT_FSIZE, h.FSize, "fsize"},
	} {
		if err := set(l.res, l.v, l.name); err != nil {
			return err
		}
	}
	return nil
}

// Landlock access rights by ABI version.
const (
	llFSv1 = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM
	llFileRights = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
)

func handledFS(abi int) uint64 {
	h := uint64(llFSv1)
	if abi >= 2 {
		h |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		h |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 5 {
		h |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return h
}

// applyLandlock confines the process to its own session directory
// (read-write, never executable), the tuisense binary (read and execute),
// and read-only system files a Go program may need. With ABI 4+ it denies
// every TCP bind and connect; with ABI 6+ it cannot signal processes or
// reach abstract UNIX sockets outside its own domain (other sessions, the
// server).
func applyLandlock(h helperArgs) error {
	abi := h.Landlock
	attr := unix.LandlockRulesetAttr{Access_fs: handledFS(abi)}
	size := unsafe.Sizeof(attr.Access_fs)
	if abi >= 4 {
		attr.Access_net = unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP
		size += unsafe.Sizeof(attr.Access_net)
	}
	if abi >= 6 {
		attr.Scoped = unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | unix.LANDLOCK_SCOPE_SIGNAL
		size += unsafe.Sizeof(attr.Scoped)
	}
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), size, 0)
	if errno != 0 {
		return fmt.Errorf("create ruleset: %w", errno)
	}
	ruleset := int(fd)
	defer unix.Close(ruleset)

	read := uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR)
	// The ELF loader and C libraries of a dynamically linked build are
	// mapped executable, which Landlock counts as execution.
	libs := read | unix.LANDLOCK_ACCESS_FS_EXECUTE
	rules := []struct {
		path     string
		access   uint64
		required bool
	}{
		{h.RW, attr.Access_fs &^ unix.LANDLOCK_ACCESS_FS_EXECUTE, true},
		{h.Binary, unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE, true},
		{"/dev/null", unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE, false},
		// Time zone data and, for dynamically linked builds, the loader
		// and C libraries. Read-only.
		{"/etc/localtime", read, false},
		{"/usr/share/zoneinfo", read, false},
		{"/etc/ld.so.cache", read, false},
		{"/lib", libs, false},
		{"/lib64", libs, false},
		{"/usr/lib", libs, false},
		{"/usr/lib64", libs, false},
	}
	for _, r := range rules {
		if err := addPathRule(ruleset, r.path, r.access&attr.Access_fs); err != nil {
			if r.required {
				return fmt.Errorf("%s: %w", r.path, err)
			}
		}
	}
	_, _, errno = unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(ruleset), 0, 0)
	if errno != 0 {
		return fmt.Errorf("restrict self: %w", errno)
	}
	return nil
}

func addPathRule(ruleset int, path string, access uint64) error {
	path = filepath.Clean(path)
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		access &= llFileRights // a file rule may only carry file rights
	}
	rule := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
	_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(ruleset), unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
