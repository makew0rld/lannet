package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/schollz/peerdiscovery"
)

// peers maps IP:port to names
var peers map[string]*peer
var peersMu = &sync.RWMutex{}

type peer struct {
	ip       string
	Name     string
	lastSeen time.Time
}

// startPeerServices starts the peer discovery service in a goroutine.
// It needs the port number of the webserver.
func startPeerServices(serverPort string) {
	// Discovery service
	go peerdiscovery.Discover(
		peerdiscovery.Settings{
			Port:      discoveryPort,
			Payload:   []byte(serverPort), // Payload holds the webserver port
			TimeLimit: -1,                 // Never stop finding peers
			Delay:     pingInterval,
			Notify: func(d peerdiscovery.Discovered) {
				go func() {
					peersMu.Lock()
					defer peersMu.Unlock()

					key := d.Address + ":" + string(d.Payload)
					_, ok := peers[key]
					if !ok {
						// This peer isn't added yet
						// Default first name is just IP address
						peers[key] = &peer{
							ip:   d.Address,
							Name: d.Address,
						}
						// But get their real name
						go updatePeerName(key, peers[key])
					}
					// Update lastSeen every time
					peers[key].lastSeen = time.Now()
				}()
			},
		},
	)
	// Timeout service - remove peers after they haven't been seen for 5 times
	// the amount of time they're supposed to ping
	go func() {
		for {
			// Runs every 5 seconds
			time.Sleep(5 * time.Second)

			peersMu.Lock()
			for addr, p := range peers {
				if p.lastSeen.Add(5 * pingInterval).Before(time.Now()) {
					delete(peers, addr)
				}
			}
			peersMu.Unlock()
		}
	}()
	// Name service - updates the names of peers
	go func() {
		for {
			// Runs every 5 seconds
			time.Sleep(5 * time.Second)

			// Get peer names and update them, concurrently

			c := make(chan struct{}, 8) // Limit workers; semaphore
			for addr, p := range peers {
				c <- struct{}{}
				go func(addr2 string, p2 *peer) {
					defer func() { <-c }()
					updatePeerName(addr2, p2)
				}(addr, p)
			}
		}
	}()
}

func updatePeerName(addr string, p *peer) {
	resp, err := http.Get("http://" + addr + "/.api/getName")
	// Ignore errors, leaving the name as what it was before
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Reset peer name to default of IP address
		peersMu.Lock()
		p.Name = p.ip
		peersMu.Unlock()
		return
	}
	// Only read name up to 64 bytes
	var newName strings.Builder
	_, err = io.CopyN(&newName, resp.Body, 65)
	if !errors.Is(err, io.EOF) {
		// Too long or some other error
		return
	}
	// Change name
	peersMu.Lock()
	p.Name = strings.ToValidUTF8(newName.String(), "\uFFFD")
	peersMu.Unlock()
}
