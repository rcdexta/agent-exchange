package ax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const resourcePeriod = 2 * time.Second
const resourceWindow = 10 * time.Second
const resourceCooldown = time.Minute

type cpuBucket struct {
	Second int64 `json:"second"`
	CPU    int64 `json:"cpu_ns"`
}

// Only AX helper processes report their own CPU. No native child is sampled,
// signalled, or charged to this budget, and no PID registry can target a reused PID.
type resourceBudget struct {
	Started     int64         `json:"started"`
	Updated     int64         `json:"updated"`
	PausedUntil int64         `json:"paused_until"`
	Buckets     [10]cpuBucket `json:"buckets"`
}

type resourcePaused struct{ Until time.Time }

func (e *resourcePaused) Error() string {
	return "AX messaging paused until " + e.Until.Format(time.RFC3339) + " (shared CPU budget); coding sessions remain running"
}

func readBudget(dir string) (resourceBudget, error) {
	var s resourceBudget
	f, err := privateFile(filepath.Join(dir, "resource-budget.json"), syscall.O_RDONLY)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	err = json.NewDecoder(io.LimitReader(f, 4096)).Decode(&s)
	return s, err
}

func resourcePause(dir string, now time.Time) error {
	s, err := readBudget(dir)
	if err != nil {
		return fmt.Errorf("AX resource protection unavailable: %w", err)
	}
	if s.PausedUntil > now.UnixMilli() {
		return &resourcePaused{Until: time.UnixMilli(s.PausedUntil)}
	}
	return nil
}

func (s *resourceBudget) add(now time.Time, cpu time.Duration) bool {
	second := now.Unix()
	if s.Started == 0 || second < s.Updated || second-s.Updated > 10 {
		*s = resourceBudget{Started: second, PausedUntil: s.PausedUntil}
	}
	s.Updated = second
	if now.UnixMilli() < s.PausedUntil {
		return true
	}
	b := &s.Buckets[second%int64(len(s.Buckets))]
	if b.Second != second {
		*b = cpuBucket{Second: second}
	}
	b.CPU += int64(max(cpu, 0))
	var total int64
	for _, bucket := range s.Buckets {
		if bucket.Second > second-10 && bucket.Second <= second {
			total += bucket.CPU
		}
	}
	if second-s.Started >= 10 && total >= int64(resourceWindow/2) {
		s.PausedUntil = now.Add(resourceCooldown).UnixMilli()
		s.Buckets = [10]cpuBucket{}
		s.Started = second + int64(resourceCooldown/time.Second)
		return true
	}
	return false
}

func reportCPU(dir string, now time.Time, cpu time.Duration) (bool, error) {
	f, err := privateFile(filepath.Join(dir, "resource-budget.lock"), syscall.O_CREAT|syscall.O_RDWR)
	if err != nil {
		return false, err
	}
	defer f.Close()
	// Never wait behind a stalled helper. The caller keeps unsent CPU for its
	// next sample, instead of blocking messaging or silently losing the sample.
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return false, err
	}
	s, err := readBudget(dir)
	if err != nil {
		return false, err
	}
	if s.PausedUntil > now.UnixMilli() {
		return true, nil
	}
	paused := s.add(now, cpu)
	path := filepath.Join(dir, "resource-budget.json")
	tmp := path + "." + randomID("")
	out, err := privateFile(tmp, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY)
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp)
	err = json.NewEncoder(out).Encode(s)
	if err == nil && paused {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	return paused, err
}

func processCPU() (time.Duration, error) {
	var r syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r); err != nil {
		return 0, err
	}
	return time.Duration(r.Utime.Nano() + r.Stime.Nano()), nil
}

// pause only touches messaging. hardStop is supplied only by standalone serve
// and bridge commands, never a launcher that owns a native harness/backend.
func watchResources(ctx context.Context, dir string, pause func(), hardStop func()) func() {
	ctx, stop := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(resourcePeriod)
		defer t.Stop()
		watchResourceSamples(ctx, dir, pause, hardStop, t.C, processCPU)
	}()
	return stop
}

func watchResourceSamples(ctx context.Context, dir string, pause func(), hardStop func(), ticks <-chan time.Time, readCPU func() (time.Duration, error)) {
	var reported, previous time.Duration
	wasPaused, warned := false, false
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticks:
			cpu, err := readCPU()
			if err == nil {
				var paused bool
				paused, err = reportCPU(dir, now, cpu-reported)
				if err == nil {
					reported = cpu
					if paused {
						pause()
						if !warned {
							fmt.Fprintln(os.Stderr, resourcePause(dir, now))
							warned = true
						}
						// A helper still burning CPU after its messaging was paused
						// is stopped locally. Never signal a parent or process group.
						if wasPaused && cpu-previous > resourcePeriod/10 && hardStop != nil {
							hardStop()
						}
					} else {
						warned = false
					}
					wasPaused = paused
				}
			}
			previous = cpu
			if err != nil && !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
				pause()
				if !warned {
					fmt.Fprintln(os.Stderr, "AX resource protection unavailable:", err)
					warned = true
				}
			}
		}
	}
}

func resourceStatus(dir string) string {
	if err := resourcePause(dir, time.Now()); err != nil {
		return err.Error()
	}
	return "CPU protection: shared helper budget, 50% of one CPU over 10s; 60s cooldown. OS CPU quota: not configured."
}
