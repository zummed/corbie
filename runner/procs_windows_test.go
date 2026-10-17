package runner_test

import "golang.org/x/sys/windows"

// standInHere plays no process of its own: the processes only tests on
// unix need are not played on Windows.
func standInHere(as string, args []string) bool { return false }

// kill stops a process, if it is still there.
func kill(pid int) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err == nil {
		windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
}

// ended says whether a process has ended: nothing has its number, or it
// has an exit code.
func ended(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err == windows.ERROR_INVALID_PARAMETER {
		return true
	}
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	const stillActive = 259
	return windows.GetExitCodeProcess(h, &code) == nil && code != stillActive
}
