// heavily inspired by rider2emacs by elizagamedev -> https://github.com/elizagamedev/rider2emacs
// for now very windows focused (there's a few good linux first solutions out there already)
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type FileTarget struct {
	Line     *int
	Column   *int
	Filename string
}

func parseArgs() ([]FileTarget, error) {
	var fileTargets []FileTarget
	var line *int
	var column *int

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--line", "-l":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("no integer argument passed to %s", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return nil, fmt.Errorf("invalid line number: %s", args[i])
			}
			if n > 0 {
				line = &n
			}
		case "--column", "-c":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("no integer argument passed to %s", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return nil, fmt.Errorf("invalid column number: %s", args[i])
			}
			if n > 0 {
				column = &n
			}
		case "nosplash", "dontReopenProjects", "disableNonBundledPlugins", "--wait":
			continue
		default:
			if strings.HasSuffix(strings.ToLower(arg), ".sln") {
				line, column = nil, nil
				continue
			}
			fileTargets = append(fileTargets, FileTarget{
				Line:     line,
				Column:   column,
				Filename: arg,
			})
			line, column = nil, nil
		}
	}
	return fileTargets, nil
}

func getServerFile() string {
	return filepath.Join(os.TempDir(), "nvim-unity-server.txt")
}

func pipeAddr() string {
	return `\\.\pipe\nvim-unity-` + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func readServerAddr() (string, bool) {
	data, err := os.ReadFile(getServerFile())
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "", false
	}
	return s, true
}

func writeServerAddr(addr string) error {
	f, err := os.OpenFile(getServerFile(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(addr)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

func isServerRunning() (bool, string) {
	addr, ok := readServerAddr()
	if !ok {
		return false, ""
	}
	cmd := exec.Command("nvim", "--server", addr, "--remote-expr", "1")
	var sink bytes.Buffer
	cmd.Stdout = &sink
	cmd.Stderr = &sink
	if cmd.Run() == nil {
		return true, addr
	}
	_ = os.Remove(getServerFile())
	return false, ""
}

func sendToExistingServer(fileTargets []FileTarget, addr string) {
	var args []string
	args = append(args, "--server", addr)
	for _, ft := range fileTargets {
		if ft.Line != nil {
			if ft.Column != nil {
				args = append(args, "--remote", fmt.Sprintf("+%d:%d", *ft.Line, *ft.Column), ft.Filename)
			} else {
				args = append(args, "--remote", fmt.Sprintf("+%d", *ft.Line), ft.Filename)
			}
		} else {
			args = append(args, "--remote", ft.Filename)
		}
	}
	_ = exec.Command("nvim", args...).Run()
}

func startHeadlessServer(addr string) error {
	args := []string{"--listen", addr, "--headless"}
	cmd := exec.Command("nvim", args...)
	return cmd.Start()
}

func main() {
	fileTargets, err := parseArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		time.Sleep(2 * time.Second)
		os.Exit(1)
	}
	if len(fileTargets) == 0 {
		fmt.Fprintf(os.Stderr, "error: no file arguments provided\n")
		time.Sleep(2 * time.Second)
		os.Exit(1)
	}

	if ok, addr := isServerRunning(); ok {
		sendToExistingServer(fileTargets, addr)
		fmt.Println("File sent to Neovim server.")
		time.Sleep(1 * time.Second)
		return
	}

	addr := pipeAddr()
	if err := writeServerAddr(addr); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot write server file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Starting headless Neovim server...")
	fmt.Printf("Server: %s\n", addr)

	if err := startHeadlessServer(addr); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to start Neovim: %v\n", err)
		_ = os.Remove(getServerFile())
		os.Exit(1)
	}

	time.Sleep(800 * time.Millisecond)

	sendToExistingServer(fileTargets, addr)

	fmt.Println()
	fmt.Printf("To attach Neovide:\n  neovide --server %s\n", addr)
	fmt.Printf("To attach terminal Neovim:\n  nvim --server %s --remote-ui\n", addr)
	fmt.Println()
	fmt.Println("Waiting for further file requests...")

	for {
		time.Sleep(2 * time.Second)
		if ok, _ := isServerRunning(); !ok {
			fmt.Printf("Server stopped at %s\n", time.Now().Format("15:04:05"))
			break
		}
	}

	_ = os.Remove(getServerFile())
}
