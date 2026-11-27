package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

const teacher = "teacher"

var students = map[string]string{"anna": "Анна", "ivan": "Иван"}

type task struct {
	ID      int    `json:"id"`
	Part    int    `json:"part"`
	Prompt  string `json:"prompt"`
	BoardID string `json:"board_id"`
}

type attempt struct {
	ID        string `json:"id"`
	Student   string `json:"student"`
	Number    int    `json:"number"`
	Submitted bool   `json:"submitted"`
	Tasks     []task `json:"tasks"`
}

func demoAttempt(student string, number int, submitted bool) attempt {
	id := student + "-" + strconv.Itoa(number)
	return attempt{id, students[student], number, submitted, []task{
		{1, 1, "Вычислите: 18 × 7 − 26.", id + "/test"},
		{2, 1, "Найдите x: 3x + 7 = 22.", id + "/test"},
		{3, 2, "Постройте равнобедренный треугольник. Докажите, что медиана к основанию является высотой.", id + "/3"},
	}}
}

type scene struct {
	Elements []json.RawMessage `json:"elements"`
	Revision int               `json:"revision"`
}

type savedState struct {
	Submitted map[string]bool  `json:"submitted"`
	Retakes   map[string]bool  `json:"retakes"`
	Boards    map[string]scene `json:"boards"`
}

type roomMessage struct {
	scene
	CanEdit bool   `json:"can_edit"`
	Ack     int    `json:"ack,omitempty"`
	Error   string `json:"error,omitempty"`
}

type change struct {
	ID       int               `json:"id"`
	Elements []json.RawMessage `json:"elements"`
}

type peer struct {
	conn *websocket.Conn
	out  chan roomMessage
}

type demo struct {
	mu    sync.Mutex
	file  string
	state savedState
	rooms map[string]map[*peer]struct{}
}

func newDemo(file string) (*demo, error) {
	d := &demo{
		file: file,
		state: savedState{
			Submitted: make(map[string]bool),
			Retakes:   make(map[string]bool),
			Boards:    make(map[string]scene),
		},
		rooms: make(map[string]map[*peer]struct{}),
	}
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &d.state); err != nil {
		return nil, err
	}
	if d.state.Submitted == nil || d.state.Retakes == nil || d.state.Boards == nil {
		return nil, errors.New("invalid demo state")
	}
	return d, nil
}

// save is called under mu. Publish a change only after committing it to disk.
func (d *demo) save() error {
	data, err := json.Marshal(d.state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(d.file), ".board-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), d.file)
}

func validActor(actor string) bool {
	return actor == teacher || students[actor] != ""
}

func (d *demo) attempts(actor string) []attempt {
	var list []attempt
	for _, student := range []string{"anna", "ivan"} {
		if actor != teacher && actor != student {
			continue
		}
		first := demoAttempt(student, 1, d.state.Submitted[student+"-1"])
		list = append(list, first)
		if d.state.Retakes[student] {
			list = append(list, demoAttempt(student, 2, d.state.Submitted[student+"-2"]))
		}
	}
	return list
}

func (d *demo) updateAttempt(w http.ResponseWriter, r *http.Request) {
	actor, id, action := r.PathValue("actor"), r.PathValue("id"), r.PathValue("action")
	if students[actor] == "" {
		http.NotFound(w, r)
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	list := d.attempts(actor)
	index := slices.IndexFunc(list, func(a attempt) bool { return a.ID == id })
	if index < 0 {
		http.NotFound(w, r)
		return
	}
	a := list[index]
	switch action {
	case "submit":
		if !a.Submitted {
			d.state.Submitted[id] = true
			if err := d.save(); err != nil {
				delete(d.state.Submitted, id)
				http.Error(w, "Не удалось сохранить сдачу", http.StatusInternalServerError)
				return
			}
			// The set of rooms can include both board types for this attempt.
			for boardID := range d.rooms {
				if strings.HasPrefix(boardID, id+"/") {
					d.broadcast(boardID, false, nil, 0)
				}
			}
		}
	case "retry":
		if a.Number != 1 || !a.Submitted {
			http.Error(w, "Попытка недоступна", http.StatusConflict)
			return
		}
		if !d.state.Retakes[actor] {
			d.state.Retakes[actor] = true
			if err := d.save(); err != nil {
				delete(d.state.Retakes, actor)
				http.Error(w, "Не удалось открыть попытку", http.StatusInternalServerError)
				return
			}
		}
	default:
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func elementID(raw json.RawMessage) string {
	var element struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &element) != nil || len(element.ID) > 128 {
		return ""
	}
	return element.ID
}

func mergeScene(previous scene, updates []json.RawMessage) (scene, error) {
	elements := slices.Clone(previous.Elements)
	positions := make(map[string]int, len(elements))
	for i, element := range elements {
		positions[elementID(element)] = i
	}
	for _, element := range updates {
		id := elementID(element)
		if id == "" || len(element) > 64<<10 {
			return scene{}, errors.New("Некорректный элемент доски")
		}
		if i, ok := positions[id]; ok {
			elements[i] = element
		} else {
			positions[id] = len(elements)
			elements = append(elements, element)
		}
	}
	var size int
	for _, element := range elements {
		size += len(element)
	}
	if len(elements) > 1000 || size > 512<<10 {
		return scene{}, errors.New("Достигнут лимит размера доски")
	}
	return scene{Elements: elements, Revision: previous.Revision + 1}, nil
}

func (d *demo) send(p *peer, message roomMessage) {
	select {
	case p.out <- message:
	default:
		// A stalled connection must reload the latest committed scene.
		_ = p.conn.SetDeadline(time.Now())
	}
}

func (d *demo) broadcast(id string, editable bool, sender *peer, ack int) {
	for p := range d.rooms[id] {
		message := roomMessage{scene: d.state.Boards[id], CanEdit: editable}
		if p == sender {
			message.Ack = ack
		}
		d.send(p, message)
	}
}

func (d *demo) apply(id, attemptID string, p *peer, update change) {
	d.mu.Lock()
	defer d.mu.Unlock()
	old := d.state.Boards[id]
	editable := !d.state.Submitted[attemptID]
	var err error
	switch {
	case !editable:
		err = errors.New("Попытка сдана")
	case update.ID <= 0 || len(update.Elements) == 0:
		err = errors.New("Некорректное изменение")
	default:
		var next scene
		next, err = mergeScene(old, update.Elements)
		if err == nil {
			d.state.Boards[id] = next
			if err = d.save(); err != nil {
				d.state.Boards[id] = old
				log.Printf("save board: %v", err)
				err = errors.New("Не удалось сохранить доску")
			}
		}
	}
	if err != nil {
		d.send(p, roomMessage{scene: old, CanEdit: editable, Ack: update.ID, Error: err.Error()})
		return
	}
	d.broadcast(id, editable, p, update.ID)
}

func (d *demo) socket(conn *websocket.Conn, boardID, attemptID string) {
	conn.MaxPayloadBytes = 640 << 10
	p := &peer{conn: conn, out: make(chan roomMessage, 8)}
	d.mu.Lock()
	if d.rooms[boardID] == nil {
		d.rooms[boardID] = make(map[*peer]struct{})
	}
	d.rooms[boardID][p] = struct{}{}
	d.send(p, roomMessage{scene: d.state.Boards[boardID], CanEdit: !d.state.Submitted[attemptID]})
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.rooms[boardID], p)
		close(p.out)
		d.mu.Unlock()
		_ = conn.Close()
	}()
	go func() {
		for message := range p.out {
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if websocket.JSON.Send(conn, message) != nil {
				_ = conn.Close()
				return
			}
		}
	}()
	for {
		var update change
		if err := websocket.JSON.Receive(conn, &update); err != nil {
			return
		}
		d.apply(boardID, attemptID, p, update)
	}
}

func (d *demo) handler(host, staticDir string) http.Handler {
	origin := "http://" + host
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/{actor}/attempts", func(w http.ResponseWriter, r *http.Request) {
		actor := r.PathValue("actor")
		if !validActor(actor) {
			http.NotFound(w, r)
			return
		}
		d.mu.Lock()
		list := d.attempts(actor)
		d.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})
	mux.HandleFunc("POST /api/{actor}/attempts/{id}/{action}", d.updateAttempt)
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		boardID, actor := r.URL.Query().Get("board"), r.URL.Query().Get("actor")
		attemptID := ""
		if validActor(actor) {
			d.mu.Lock()
			for _, a := range d.attempts(actor) {
				if slices.ContainsFunc(a.Tasks, func(t task) bool { return t.BoardID == boardID }) {
					attemptID = a.ID
					break
				}
			}
			d.mu.Unlock()
		}
		if attemptID == "" {
			http.NotFound(w, r)
			return
		}
		websocket.Handler(func(conn *websocket.Conn) { d.socket(conn, boardID, attemptID) }).ServeHTTP(w, r)
	})
	mux.Handle("GET /", http.FileServer(http.Dir(staticDir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		needsOrigin := r.Method == http.MethodPost || r.URL.Path == "/ws"
		if r.Host != host || (needsOrigin && r.Header.Get("Origin") != origin) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func main() {
	port := flag.Int("port", 8092, "loopback HTTP port")
	file := flag.String("state", ".board-demo.json", "local demo state")
	staticDir := flag.String("static", "../frontend/board-demo/dist", "built frontend directory")
	flag.Parse()
	if _, err := os.Stat(filepath.Join(*staticDir, "index.html")); err != nil {
		log.Fatalf("Build the frontend first: %v", err)
	}
	d, err := newDemo(*file)
	if err != nil {
		log.Fatal(err)
	}
	host := fmt.Sprintf("127.0.0.1:%d", *port)
	server := &http.Server{
		Addr:              host,
		Handler:           d.handler(host, *staticDir),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Board demo: http://%s/", host)
	log.Fatal(server.ListenAndServe())
}
