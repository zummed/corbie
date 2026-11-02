package role

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// channelVariable gives a process the numbers of its two handles of the
// channel: Windows hands a process no files by number.
const channelVariable = "CORBIE_JOB_CHANNEL"

// stillActive is the exit code of a process that has not ended.
const stillActive = 259

// group is the job object the process made of itself.
var group windows.Handle

// ownGroup does nothing: the process makes its group itself, before it
// starts anything.
func ownGroup(cmd *exec.Cmd) {}

// startGroup puts the process in a job object of its own, which every
// program it starts joins. Only the process holds the object's
// handle, and the system ends everything in the object when that handle
// closes: when the process ends, however it ends.
func startGroup() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE}}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return err
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		windows.CloseHandle(job)
		return err
	}
	group = job
	return nil
}

// LimitMemory has the system refuse this process more memory than a limit,
// through the job object it made of itself, so that a request for more ends
// it at once and fills nothing.
func LimitMemory(limit uint64) {
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY},
		ProcessMemoryLimit: uintptr(limit)}
	windows.SetInformationJobObject(group, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
}

// endGroup stops everything in the process's group, the process included.
func endGroup() {
	if group != 0 {
		windows.TerminateJobObject(group, 3)
	}
}

// Gone says whether no process has this number any more.
func Gone(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err == windows.ERROR_INVALID_PARAMETER {
		return true
	}
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code != stillActive
}

// killGroup stops a process, and so everything it started: its group's
// handle closes with it.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}

// Watch does nothing here: what a process holds is watched from inside.
func Watch(pid int, limit int64, done <-chan struct{}, over func()) { <-done }

// giveChannel has a process inherit its two ends of the channel, and tells
// it their numbers.
func giveChannel(cmd *exec.Cmd, toJob, fromJob *os.File) {
	var handles []syscall.Handle
	for _, f := range []*os.File{toJob, fromJob} {
		h := syscall.Handle(f.Fd())
		windows.SetHandleInformation(windows.Handle(h), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)
		handles = append(handles, h)
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, handles...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%d,%d", channelVariable, handles[0], handles[1]))
}

// takeChannel gives a process its ends of the channel, kept from every
// program it starts.
func takeChannel() (from, to *os.File) {
	first, second, _ := strings.Cut(os.Getenv(channelVariable), ",")
	os.Unsetenv(channelVariable)
	return handleFile(first, "from its starter"), handleFile(second, "to its starter")
}

// handleFile opens an inherited handle by its number, and keeps it from
// the programs the process starts. A number that is not one gives no file,
// which every read and write fails on.
func handleFile(number, name string) *os.File {
	h, err := strconv.ParseUint(number, 10, 64)
	if err != nil {
		return nil
	}
	windows.SetHandleInformation(windows.Handle(h), windows.HANDLE_FLAG_INHERIT, 0)
	return os.NewFile(uintptr(h), name)
}
