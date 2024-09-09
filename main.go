package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	linux = "linux"
)

var (
	errTargetNotSupported = errors.New("target not supported")
)

type entry struct {
	k string
	v string
}

func main() {
	target := runtime.GOOS
	entries, err := collect(target)
	if err != nil {
		slog.Error("failed to collect", "error", err)
	}

	fmt.Println(entriesToString(entries))
}

func collect(target string) ([]entry, error) {
	var entries []entry
	osName, err := osName(target)
	if err != nil {
		return nil, fmt.Errorf("unable to get os: %w", err)
	}
	entries = append(entries, entry{k: "OS", v: osName})

	kernel, err := kernel(target)
	if err != nil {
		return nil, fmt.Errorf("unable to get kernel: %w", err)
	}
	entries = append(entries, entry{k: "Kernel", v: kernel})

	uptime, err := uptime(target)
	if err != nil {
		return nil, fmt.Errorf("unable to get uptime: %w", err)
	}
	entries = append(entries, entry{k: "Uptime", v: uptime})

	cpu, err := cpu(target)
	if err != nil {
		return nil, fmt.Errorf("unable to get cpu: %w", err)
	}
	entries = append(entries, entry{k: "CPU", v: cpu})

	memory, err := memory(target)
	if err != nil {
		return nil, fmt.Errorf("unable to get memory: %w", err)
	}
	entries = append(entries, entry{k: "Memory", v: memory})

	return entries, nil
}

func entriesToString(entries []entry) string {
	var n int
	for _, entry := range entries {
		l := len(entry.k)
		if n < len(entry.k) {
			n = l
		}
	}

	var str string
	for _, entry := range entries {
		k := entry.k + strings.Repeat(" ", n-len(entry.k))
		str = fmt.Sprintf("%v%v %v\n", str, k, entry.v)
	}
	str = strings.TrimSuffix(str, "\n")
	return str
}

func osName(target string) (string, error) {
	switch target {
	case linux:
		fname := "/etc/os-release"
		f, err := os.Open(fname)
		if err != nil {
			return "", fmt.Errorf("failed to open %v", fname)
		}
		defer f.Close()

		var name string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) == 0 || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			k := parts[0]
			v := strings.Trim(parts[1], `"`)

			if k == "PRETTY_NAME" {
				name = v
				break
			}
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("failed to scan %v", fname)
		}

		if name == "" {
			return "", errors.New("no value for \"PRETTY_NAME\"")
		}
		return name, nil
	}
	return "", errTargetNotSupported
}

func kernel(target string) (string, error) {
	switch target {
	case linux:
		var uname syscall.Utsname
		if err := syscall.Uname(&uname); err != nil {
			return "", errors.New("failed syscall utsname")
		}
		return int8ToString(uname.Release[:]), nil
	}
	return "", errTargetNotSupported
}

func uptime(target string) (string, error) {
	switch target {
	case linux:
		var sysinfo syscall.Sysinfo_t
		if err := syscall.Sysinfo(&sysinfo); err != nil {
			return "", errors.New("failed syscall sysinfo")
		}

		duration := time.Duration(sysinfo.Uptime) * time.Second

		h := int(duration.Hours())
		m := int(duration.Minutes()) % 60

		if h > 0 && m > 0 {
			return fmt.Sprintf("%vh %vm", h, m), nil
		} else if h > 0 {
			return fmt.Sprintf("%vh", h), nil
		}
		return fmt.Sprintf("%vm", m), nil
	}
	return "", errTargetNotSupported
}

func cpu(target string) (string, error) {
	switch target {
	case linux:
		fname := "/proc/cpuinfo"
		f, err := os.Open(fname)
		if err != nil {
			return "", fmt.Errorf("failed to open %v", fname)
		}
		defer f.Close()

		var name string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) == 0 {
				continue
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}

			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])

			if k == "model name" {
				name = v
				break
			}
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("failed to scan %v", fname)
		}

		if name == "" {
			return "", errors.New("no value for \"model name\"")
		}
		return name, nil
	}
	return "", errTargetNotSupported
}

func memory(target string) (string, error) {
	switch target {
	case linux:
		fname := "/proc/meminfo"
		f, err := os.Open(fname)
		if err != nil {
			return "", fmt.Errorf("failed to open %v", fname)
		}
		defer f.Close()

		memInfo := make(map[string]uint64)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) == 0 {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) <= 2 {
				continue
			}

			k := strings.TrimSuffix(fields[0], ":")
			v, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return "", fmt.Errorf("cannot convert %v uint64", fields[1])
			}
			memInfo[k] = v
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("failed to scan %v", fname)
		}

		totalMB := memInfo["MemTotal"] / 1024
		freeMB := memInfo["MemFree"] / 1024
		buffersMB := memInfo["Buffers"] / 1024
		cachedMB := memInfo["Cached"] / 1024

		usedMB := totalMB - (freeMB + buffersMB + cachedMB)

		return fmt.Sprintf("%vM / %vM", usedMB, totalMB), nil
	}
	return "", errTargetNotSupported
}

func int8ToString(arr []int8) string {
	b := make([]byte, len(arr))
	for _, v := range arr {
		if v == 0x00 {
			break
		}
		b = append(b, byte(v))
	}
	return string(b)
}
