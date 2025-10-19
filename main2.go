package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

var (
	tasks        = []Task{}
	nextID       = 1
	dataDir      = "data"
	dataFile     = dataDir + "/tasks.json"
	templateDir  = "template"
	templateFile = templateDir + "/index.html"
	mu           sync.RWMutex
)

func main() {
	initializeStorage()

	if err := loadTasks(); err != nil {
		log.Printf("⚠️ Could not load tasks: %v", err)
	}

	fs := http.FileServer(http.Dir("template/"))
	http.Handle("/template/", http.StripPrefix("/template/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Запрос: %s", r.URL.Path)

		if r.URL.Path == "/" || r.URL.Path == "/template/index.html" {
			http.ServeFile(w, r, "template/index.html")
			log.Printf("index.html opened")
			return
		}
		http.ServeFile(w, r, "template/index.html")
		log.Println("Served index.html")
	})

	http.Handle("/locales/", http.StripPrefix("/locales/", http.FileServer(http.Dir("template/locales/"))))
	http.Handle("/photo.jpg", http.FileServer(http.Dir("template/")))

	http.HandleFunc("/api/command", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		response := handleCommand(req.Command)

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(response))
	})

	log.Println("📁 Template: ", templateFile)
	log.Println("💾 Data file: ", dataFile)
	log.Println("🚀 Server running on http://localhost:1112")
	log.Fatal(http.ListenAndServe(":1112", nil))
}

func initializeStorage() {
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.Mkdir(dataDir, 0755); err != nil {
			log.Fatal("❌ Cannot create data directory:", err)
		}
		log.Println("📁 Created data directory:", dataDir)
	}

	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		empty := []Task{}
		data, _ := json.MarshalIndent(empty, "", "  ")
		if err := os.WriteFile(dataFile, data, 0644); err != nil {
			log.Fatal("❌ Cannot create data file:", err)
		}
		log.Println("📄 Created initial task file:", dataFile)
	}
}

func loadTasks() error {
	mu.Lock()
	defer mu.Unlock()

	file, err := os.Open(dataFile)
	if err != nil {
		return err
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	if len(bytes) == 0 {
		tasks = []Task{}
		return nil
	}

	if err := json.Unmarshal(bytes, &tasks); err != nil {
		return err
	}

	nextID = 1
	for _, t := range tasks {
		if t.ID >= nextID {
			nextID = t.ID + 1
		}
	}

	log.Printf("✅ Loaded %d tasks from disk", len(tasks))
	return nil
}

func saveTasks() error {
	mu.RLock()
	defer mu.RUnlock()

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(dataFile, data, 0644)
}

func safelySave() {
	if err := saveTasks(); err != nil {
		log.Printf("❌ Failed to save tasks: %v", err)
	}
}

func handleCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "❌ Empty command."
	}

	parts := strings.Fields(cmd)
	base := strings.ToLower(parts[0])

	switch base {
	case "help":
		return `
Available commands:
  add <task>     - Add a new task
  list           - List all tasks
  done <id>      - Mark task as done (e.g. 'done 1')
  pending        - Show only pending tasks
  clear          - Clear screen
  delete <id>    - Delete a task by ID
`

	case "add":
		if len(parts) < 2 {
			return "❌ Usage: add <task>"
		}
		title := strings.Join(parts[1:], " ")

		mu.Lock()
		tasks = append(tasks, Task{ID: nextID, Title: title, Done: false})
		newID := nextID
		nextID++
		mu.Unlock()

		safelySave()
		return fmt.Sprintf("✓ Added: \"%s\" (ID: %d)", title, newID)

	case "list":
		mu.RLock()
		defer mu.RUnlock()

		if len(tasks) == 0 {
			return "📭 No tasks yet."
		}
		var out strings.Builder
		out.WriteString("\n📋 All Tasks:\n")
		for _, t := range tasks {
			status := "[ ]"
			if t.Done {
				status = "[x]"
			}
			out.WriteString(fmt.Sprintf("  %d. %s %s\n", t.ID, status, t.Title))
		}
		return out.String()

	case "pending":
		mu.RLock()
		defer mu.RUnlock()

		var pending []Task
		for _, t := range tasks {
			if !t.Done {
				pending = append(pending, t)
			}
		}
		if len(pending) == 0 {
			return "✅ All tasks completed!"
		}
		var out strings.Builder
		out.WriteString("\n⏳ Pending Tasks:\n")
		for _, t := range pending {
			out.WriteString(fmt.Sprintf("  %d. %s\n", t.ID, t.Title))
		}
		return out.String()

	case "done":
		if len(parts) != 2 {
			return "❌ Usage: done <id>"
		}
		var id int
		n, _ := fmt.Sscanf(parts[1], "%d", &id)
		if n != 1 {
			return "❌ Invalid ID"
		}

		mu.Lock()
		for i := range tasks {
			if tasks[i].ID == id {
				tasks[i].Done = true
				mu.Unlock()
				safelySave()
				return fmt.Sprintf("✅ Task %d marked as done.", id)
			}
		}
		mu.Unlock()
		return "🔍 Task not found."

	case "delete":
		if len(parts) != 2 {
			return "❌ Usage: delete <id>"
		}
		var id int
		n, _ := fmt.Sscanf(parts[1], "%d", &id)
		if n != 1 {
			return "❌ Invalid ID"
		}

		mu.Lock()
		for i, t := range tasks {
			if t.ID == id {
				deletedTitle := t.Title
				tasks = append(tasks[:i], tasks[i+1:]...)
				mu.Unlock()
				safelySave()
				return fmt.Sprintf("🗑️ Deleted task %d: \"%s\"", id, deletedTitle)
			}
		}
		mu.Unlock()
		return "🔍 Task not found."

	case "clear":
		return "\n\x1b[2J\x1b[H"

	default:
		return fmt.Sprintf("❌ Unknown command: %s\nType 'help' for available commands.", cmd)
	}
}
