package role_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/zummed/corbie/role"
)

// holdsPast is the argument that makes the test binary a process that asks
// for more memory than its limit.
const holdsPast = "holds-past-its-limit"

func init() {
	if len(os.Args) > 1 && os.Args[1] == holdsPast {
		role.LimitMemory(64 << 20)
		held := make([]byte, 256<<20)
		held[len(held)-1] = 1
		os.Exit(0)
	}
}

// A request past the limit ends the process at once, and fills nothing.
func Test_role_system_refuses_memory_past_the_limit(t *testing.T) {
	said, err := exec.Command(os.Args[0], holdsPast).CombinedOutput()
	if err == nil || !strings.Contains(string(said), "out of memory") && !strings.Contains(string(said), "cannot allocate") {
		t.Fatalf("it ended %v: %.300s", err, said)
	}
}
