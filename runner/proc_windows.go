package runner

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

// channelVariable gives the job's process the numbers of its two handles
// of the channel: Windows hands a process no files by number.
const channelVariable = "CORBIE_JOB_CHANNEL"

// stillActive is the exit code of a process that has not ended.
const stillActive = 259

// group is the job object the job's process made of itself.
var group windows.Handle

// ownGroup does nothing: the job's process makes its group itself, before
// it starts anything.
func ownGroup(cmd *exec.Cmd) {}

// startGroup puts the job's process in a job object of its own, which
// every program it starts joins. Only the process holds the object's
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

// endGroup stops everything in the job's group, the job's process
// included.
func endGroup() {
	if group != 0 {
		windows.TerminateJobObject(group, 3)
	}
}

// gone says whether no process has this number any more.
func gone(pid int) bool {
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

// killGroup stops a job's process, and so everything it started: its
// group's handle closes with it.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}

func (j *supervised) watchMemory(pid int, done <-chan struct{}) { <-done }

// giveChannel has the job's process inherit its two ends of the channel,
// and tells it their numbers.
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

// takeChannel gives the job's process its ends of the channel, kept from
// every program it starts.
func takeChannel() (fromRunner, toRunner *os.File) {
	from, to, _ := strings.Cut(os.Getenv(channelVariable), ",")
	os.Unsetenv(channelVariable)
	return handleFile(from, "from the runner"), handleFile(to, "to the runner")
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
