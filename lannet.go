package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/makeworld-the-better-one/lannet/webbrowser"
)

const (
	discoveryPort = "9998" // UDP port for finding peers
	pingInterval  = time.Second
)

var (
	tmpDir      = filepath.Join(os.TempDir(), "lannet")
	pidFile     = filepath.Join(tmpDir, "pid")
	portFile    = filepath.Join(tmpDir, "port")
	defaultRoot = "" // Defaults to $HOME/lannet
)
var name string // Name shown to other peers

// Set by Makefile
var (
	version string
	commit  string
	builtBy string
)

func main() {
	peers = make(map[string]*peer)

	home, err := os.UserHomeDir()
	if err != nil {
		defaultRoot = "."
	}
	defaultRoot = filepath.Join(home, "lannet")

	if len(os.Args) == 1 {
		// No commands, so start daemon if not running and open homepage
		_, err := os.ReadFile(pidFile)
		if err == nil {
			// Daemon is running
			openHomepage()
			return
		}
		// Daemon isn't running
		// Start daemon separately, this process isn't it

		execPath, err := os.Executable()
		if err != nil {
			// Hope lannet is on PATH
			execPath = "lannet"
		}
		err = exec.Command(execPath, "daemon").Start()
		if err != nil {
			fmt.Println("Failed to start daemon: " + err.Error())
			os.Exit(1)
		}

		// Give daemon some time to start up
		time.Sleep(time.Millisecond * 200)

		ok := openHomepage()
		if !ok {
			fmt.Println("daemon failed to start up properly")
		}
		return
	}

	// Check subcommand
	switch os.Args[1] {
	case "help":
		fmt.Printf(
			`lannet - a little web on the LAN

Commands:

lannet
    Start the daemon if needed, and open the homepage.

lannet version
    View the version information of lannet.

lannet stop
    Stop the daemon if running

lannet root my/path
    Change the webserver root from ~/lannet to my/path

lannet name [new-name]
    View the current name, or set it to a new one
`,
		)

	case "version":
		fmt.Println("lannet", version)
		fmt.Println("Commit:", commit)
		fmt.Println("Built by:", builtBy)

	case "daemon":
		// This process is the daemon, start up

		os.MkdirAll(tmpDir, 0755)

		// First write the PID file to record that it's the daemon and it's running
		err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
		if err != nil {
			// Nothing really works if it can't write the PID file, just stop
			os.Exit(1)
		}

		os.MkdirAll(defaultRoot, 0755)

		// Setup name
		u, err := user.Current()
		if err == nil {
			if u.Name == "" {
				name = u.Username
			} else {
				name = u.Name
			}
		}
		hostname, _ := os.Hostname()
		if name == "" {
			name = hostname
		} else {
			name += "@" + hostname
		}

		// Start services and record webserver port
		port := startWebserver()
		startPeerServices(port)
		err = os.WriteFile(portFile, []byte(port), 0644)
		daemonDie(err)

		// Now just let the other goroutines run
		select {}

	case "stop":
		// Stop the daemon if it's running

		b, err := os.ReadFile(pidFile)
		if err != nil {
			fmt.Println("daemon not running")
			return
		}
		daemonPid, _ := strconv.Atoi(string(b))
		daemonProc, err := os.FindProcess(daemonPid)
		if err != nil {
			fmt.Println("daemon not running")
			return
		}
		err = daemonProc.Kill()
		if err != nil {
			fmt.Println("error killing daemon: " + err.Error())
			os.Exit(1)
		}
		os.Remove(pidFile)
		os.Remove(portFile)

	case "root":
		// Error handling
		port, err := os.ReadFile(portFile)
		if err != nil {
			fmt.Println("daemon not running, run 'lannet' to start it")
			os.Exit(1)
		}
		if len(os.Args) != 3 {
			fmt.Println("No root path specified")
			os.Exit(1)
		}
		fi, err := os.Stat(os.Args[2])
		if err != nil {
			fmt.Println("error using that path: " + err.Error())
			os.Exit(1)
		}
		if !fi.IsDir() {
			fmt.Println("not a directory")
			os.Exit(1)
		}

		newRoot, _ := filepath.Abs(os.Args[2])

		// Make API request now that it's all good
		resp, err := http.Post("http://localhost:"+string(port)+"/.api/setRoot", "text/plain", strings.NewReader(newRoot))
		if err != nil {
			fmt.Println("API request failed: " + err.Error())
			os.Exit(1)
		}
		if resp.StatusCode != http.StatusOK {
			// Print server response
			fmt.Printf("API request failed!\n\n")
			io.Copy(os.Stdout, resp.Body)
			os.Exit(1)
		}

	case "name":
		// Error handling
		port, err := os.ReadFile(portFile)
		if err != nil {
			fmt.Println("daemon not running, run 'lannet' to start it")
			os.Exit(1)
		}
		if len(os.Args) > 3 {
			fmt.Println("Too many arguments, quote your name if it has spaces")
			os.Exit(1)
		}

		var resp *http.Response

		if len(os.Args) == 2 {
			// Get name
			resp, err = http.Get("http://localhost:" + string(port) + "/.api/getName")
		} else {
			// Set name
			resp, err = http.Post(
				"http://localhost:"+string(port)+"/.api/setName", "text/plain", strings.NewReader(os.Args[2]),
			)
		}
		// Common error handling
		if err != nil {
			fmt.Println("API request failed: " + err.Error())
			os.Exit(1)
		}
		if resp.StatusCode != http.StatusOK {
			// Print server response
			fmt.Printf("API request failed!\n\n")
			io.Copy(os.Stdout, resp.Body)
			os.Exit(1)
		}

		if len(os.Args) == 2 {
			// Get name - print it
			io.Copy(os.Stdout, resp.Body)
			fmt.Println()
		}

	default:
		fmt.Println("unknown command " + "'" + os.Args[1] + "'")
	}
}

// openHomepage returns false if the port file could not be read/found.
func openHomepage() bool {
	port, err := os.ReadFile(portFile)
	if err != nil {
		return false
	}
	u := "http://localhost:" + string(port) + "/.homepage"
	_, err = webbrowser.Open(u)
	if err != nil {
		fmt.Println("Error opening webbrowser: " + err.Error())
	}
	fmt.Println(u)
	return true
}

// daemonDie cleans up files if some unrecoverable error happens
func daemonDie(err error) {
	if err == nil {
		return
	}
	os.Remove(pidFile)
	os.Remove(portFile)
	os.Exit(1)
}
