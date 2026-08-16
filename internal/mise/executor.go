package mise

import (
	"os"
	"os/exec"
)

// OsExecutor runs commands via os/exec with a custom environment,
// returning combined output. It is the real-world Executor.
type OsExecutor struct{}

// Run implements Executor.
func (OsExecutor) Run(env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	return string(out), err
}
