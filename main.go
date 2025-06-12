package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"log"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// session represents a single SSE connection for a user
type session struct {
	stateChannel chan interface{}
	userID       string
}

// sessionsLock stores and manages all active sessions
type sessionsLock struct {
	MU       sync.Mutex
	sessions []*session
}

func (sl *sessionsLock) addSession(s *session) {
	sl.MU.Lock()
	defer sl.MU.Unlock()
	sl.sessions = append(sl.sessions, s)
}

func (sl *sessionsLock) removeSession(s *session) {
	sl.MU.Lock()
	defer sl.MU.Unlock()
	idx := slices.Index(sl.sessions, s)
	if idx != -1 {
		if sl.sessions[idx].stateChannel != nil {
			close(sl.sessions[idx].stateChannel)
		}
		sl.sessions[idx] = nil
		sl.sessions = slices.Delete(sl.sessions, idx, idx+1)
	}
}

func (sl *sessionsLock) closeAllSessions() {
	sl.MU.Lock()
	defer sl.MU.Unlock()
	for _, s := range sl.sessions {
		if s != nil && s.stateChannel != nil {
			close(s.stateChannel)
		}
	}
	sl.sessions = nil
}

var currentSessions sessionsLock

func main() {
	app := fiber.New()
	app.Use(recover.New())
	app.Use(cors.New())

	// Health check
	app.Get("/health", func(c fiber.Ctx) error {
		return c.Send(nil)
	})

	// Returns open sessions and connection count
	app.Get("/connections", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"open-connections": app.Server().GetOpenConnectionsCount(),
			"sessions":         len(currentSessions.sessions),
		})
	})

	// System metrics endpoint
	app.Get("/metrics/system", func(c fiber.Ctx) error {
		// Go memory stats
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		// OS memory stats
		vmStat, _ := mem.VirtualMemory()

		// CPU usage (averaged over 1 second)
		cpuPercent, _ := cpu.Percent(time.Second, false)

		return c.JSON(fiber.Map{
			"timestamp": time.Now().Format(time.RFC3339),
			"system_memory": fiber.Map{
				"total_mb":     bToMb(vmStat.Total),
				"used_mb":      bToMb(vmStat.Used),
				"used_percent": fmt.Sprintf("%.2f", vmStat.UsedPercent),
			},
			"cpu": fiber.Map{
				"usage_percent": fmt.Sprintf("%.2f", cpuPercent[0]),
			},
			"go_memory": fiber.Map{
				"alloc_mb":       fmt.Sprintf("%.2f", bToMbFloat(memStats.Alloc)),
				"total_alloc_mb": fmt.Sprintf("%.2f", bToMbFloat(memStats.TotalAlloc)),
				"heap_alloc_mb":  fmt.Sprintf("%.2f", bToMbFloat(memStats.HeapAlloc)),
				"heap_sys_mb":    fmt.Sprintf("%.2f", bToMbFloat(memStats.HeapSys)),
				"gc_cycles":      memStats.NumGC,
			},
			"goroutines": runtime.NumGoroutine(),
		})
	})

	// SSE connection
	app.Get("/sse", func(c fiber.Ctx) error {
		userID := c.Query("userID")
		if userID == "" {
			return c.Status(400).SendString("userID is required")
		}

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		stateChan := make(chan interface{})
		s := &session{stateChannel: stateChan, userID: userID}
		currentSessions.addSession(s)

		err := c.SendStreamWriter(func(w *bufio.Writer) {
			keepAlive := time.NewTicker(15 * time.Second)
			defer keepAlive.Stop()
			// Remove session when client disconnects
			defer func() {
				currentSessions.removeSession(s)
				log.Printf("SSE disconnected: userID=%s", userID)
			}()

			for {
				select {
				case ev, ok := <-stateChan:
					if !ok {
						// Channel closed gracefully
						return
					}

					sseMessage, err := buildSSEPayload("current-value", ev)
					if err != nil {
						log.Printf("SSE format error: %v", err)
						continue
					}

					if _, err := fmt.Fprint(w, sseMessage); err != nil {
						log.Printf("SSE write error: %v", err)
						return
					}
					if err := w.Flush(); err != nil {
						log.Printf("SSE flush error: %v", err)
						return
					}
				case <-keepAlive.C:
					// Optional: Send heartbeat if desired
					// _, _ = fmt.Fprint(w, ":keepalive\n")
					// _ = w.Flush()
				}
			}
		})

		return err
	})

	// Broadcast to all sessions of a user
	app.Post("/send-to-user", func(c fiber.Ctx) error {
		type reqBody struct {
			UserID string      `json:"userID"`
			Value  interface{} `json:"value"`
		}
		var body reqBody
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
		}
		if body.UserID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "userID is required"})
		}

		sent := 0
		currentSessions.MU.Lock()
		for _, s := range currentSessions.sessions {
			if s != nil && s.userID == body.UserID {
				select {
				case s.stateChannel <- body.Value:
					sent++
				default:
					// Drop if blocked
				}
			}
		}
		currentSessions.MU.Unlock()

		return c.JSON(fiber.Map{"sent": sent})
	})

	// Start server in goroutine
	go func() {
		if err := app.Listen(":8080"); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Graceful shutdown listener
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Gracefully shutting down the server...")

	// Close all SSE connections before shutdown
	currentSessions.closeAllSessions()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server shutdown complete.")
}

func buildSSEPayload(eventType string, data any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	// Create JSON-serializable structure
	payload := map[string]any{"data": data}

	// Encode the payload into JSON and write it into a buffer
	if err := enc.Encode(payload); err != nil {
		return "", err
	}

	// Initialize a string builder for efficient string concatenation
	var sb strings.Builder

	// Add SSE event type
	sb.WriteString(fmt.Sprintf("event: %s\n", eventType))

	// Add retry interval (client will retry connection after 15s if disconnected)
	sb.WriteString("retry: 15000\n")

	// Add actual data as a single line (escaped JSON)
	sb.WriteString(fmt.Sprintf("data: %s\n\n", strings.TrimSpace(buf.String())))

	// Return the final SSE block
	return sb.String(), nil
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

func bToMbFloat(b uint64) float64 {
	return float64(b) / 1024 / 1024
}
