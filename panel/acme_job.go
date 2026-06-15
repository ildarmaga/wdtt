package panel

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type acmeJob struct {
	mu      sync.Mutex
	running bool
	log     strings.Builder
	result  map[string]interface{}
	err     error
}

var activeAcmeJob acmeJob

func acmeJobTryStart(title string) bool {
	activeAcmeJob.mu.Lock()
	defer activeAcmeJob.mu.Unlock()
	if activeAcmeJob.running {
		return false
	}
	activeAcmeJob.running = true
	activeAcmeJob.log.Reset()
	activeAcmeJob.result = nil
	activeAcmeJob.err = nil
	activeAcmeJob.log.WriteString(title + "\n")
	activeAcmeJob.log.WriteString(strings.Repeat("-", 48) + "\n")
	return true
}

func acmeLogWrite(s string) {
	activeAcmeJob.mu.Lock()
	defer activeAcmeJob.mu.Unlock()
	if !activeAcmeJob.running {
		return
	}
	activeAcmeJob.log.WriteString(s)
	if !strings.HasSuffix(s, "\n") {
		activeAcmeJob.log.WriteString("\n")
	}
}

func acmeJobDone(result map[string]interface{}) {
	activeAcmeJob.mu.Lock()
	defer activeAcmeJob.mu.Unlock()
	activeAcmeJob.running = false
	activeAcmeJob.result = result
	if result != nil {
		if msg, ok := result["message"].(string); ok {
			activeAcmeJob.log.WriteString("\n✓ " + msg + "\n")
		}
	}
}

func acmeJobFail(err error) {
	activeAcmeJob.mu.Lock()
	defer activeAcmeJob.mu.Unlock()
	activeAcmeJob.running = false
	activeAcmeJob.err = err
	if err != nil {
		activeAcmeJob.log.WriteString("\n✗ " + err.Error() + "\n")
	}
}

func acmeJobPollJSON() map[string]interface{} {
	activeAcmeJob.mu.Lock()
	defer activeAcmeJob.mu.Unlock()
	out := map[string]interface{}{
		"running": activeAcmeJob.running,
		"log":     activeAcmeJob.log.String(),
	}
	if activeAcmeJob.running {
		return out
	}
	if activeAcmeJob.err != nil {
		out["success"] = false
		out["error"] = activeAcmeJob.err.Error()
		out["message"] = activeAcmeJob.err.Error()
		return out
	}
	out["success"] = true
	if activeAcmeJob.result != nil {
		for k, v := range activeAcmeJob.result {
			out[k] = v
		}
	}
	return out
}

func acmeJobActive() bool {
	activeAcmeJob.mu.Lock()
	defer activeAcmeJob.mu.Unlock()
	return activeAcmeJob.running
}

func runAcmeShStream(timeout time.Duration, args ...string) (string, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, acmeShBin(), args...)
	cmd.Env = append(os.Environ(), "HOME=/root")

	acmeLogWrite("$ acme.sh " + strings.Join(args, " "))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var buf strings.Builder
	var wg sync.WaitGroup
	pump := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			buf.WriteString(line)
			buf.WriteByte('\n')
			acmeLogWrite(line)
		}
	}
	wg.Add(2)
	go pump(stdout)
	go pump(stderr)
	wg.Wait()
	err = cmd.Wait()
	return strings.TrimSpace(buf.String()), err
}
