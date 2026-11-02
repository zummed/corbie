package secret_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/zummed/corbie/secret"
)

// A job's list is as long as its definition: the controller reads it to find
// what to send, and the time that takes grows with its length.
func Test_sec_named_takes_time_in_proportion_to_the_list(t *testing.T) {
	allow := make([]string, 500_000)
	for i := range allow {
		allow[i] = fmt.Sprintf("secret:s%d@a", i)
	}
	began := time.Now()
	names := secret.Named(append(allow, allow...))
	if took := time.Since(began); len(names) != len(allow) || took > 5*time.Second {
		t.Fatalf("%d names in %v", len(names), took)
	}
}
