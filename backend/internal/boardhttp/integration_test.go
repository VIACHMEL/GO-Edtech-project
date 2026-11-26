package boardhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/websocket"
)

func TestBoardCollaborationPostgres(t *testing.T) {
	dsn := os.Getenv("EDTECH_TEST_DSN")
	if dsn == "" {
		t.Skip("set EDTECH_TEST_DSN to run PostgreSQL integration tests")
	}
	config, err := pgxpool.ParseConfig(dsn)
	checkBoardError(t, err)
	name := config.ConnConfig.Database
	if name != "edtech_test_db" && !strings.HasSuffix(name, "_test") {
		t.Fatalf("refusing to write to non-test database %q", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	store, err := db.New(ctx, dsn)
	checkBoardError(t, err)
	t.Cleanup(store.Close)
	fixture := newBoardFixture(t, ctx, store)
	boardID := fixture.board.ID
	var revoked sync.Map
	actor := func(r *http.Request) (int64, bool) {
		id, err := strconv.ParseInt(r.Header.Get("X-Board-Test-Actor"), 10, 64)
		_, disabled := revoked.Load(id)
		return id, err == nil && !disabled
	}
	start := func(store *db.DB) *httptest.Server {
		mux := http.NewServeMux()
		server := httptest.NewUnstartedServer(mux)
		Register(mux, store, actor, "http://"+server.Listener.Addr().String())
		server.Start()
		t.Cleanup(server.Close)
		return server
	}
	server := start(store)
	conns := make([]*websocket.Conn, 10)
	for i := range conns {
		actorID := fixture.students[0]
		if i%2 == 0 {
			actorID = fixture.teacher
		}
		conns[i] = dialBoard(t, server.URL, boardID, actorID)
		initial := receiveBoard(t, conns[i])
		if initial.Type != "snapshot" || initial.Seq != 0 || !initial.CanEdit || initial.DraftOwner != actorID {
			t.Fatalf("initial snapshot %d: %+v", i, initial)
		}
	}

	var group sync.WaitGroup
	errors := make(chan error, len(conns))
	for i, conn := range conns {
		group.Go(func() {
			errors <- websocket.JSON.Send(conn, boardTestOp(i+1, 0, fmt.Sprintf("shape%d", i), i))
		})
	}
	group.Wait()
	close(errors)
	for err := range errors {
		checkBoardError(t, err)
	}
	for i, conn := range conns {
		shapes := map[string]bool{}
		ack := false
		for !ack || len(shapes) < len(conns) {
			message := receiveBoard(t, conn)
			if message.Error != "" {
				t.Fatalf("distinct edit rejected on client %d: %+v", i, message)
			}
			if message.Type == "snapshot" {
				clear(shapes)
			}
			for _, element := range message.Elements {
				var value struct{ ID string }
				checkBoardError(t, json.Unmarshal(element, &value))
				shapes[value.ID] = true
			}
			ack = ack || message.Ack == boardTestID(i+1)
		}
	}

	sameObject := boardTestOp(12, 10, "shape0", 20)
	stale := boardTestOp(13, 10, "shape0", 30)
	checkBoardError(t, websocket.JSON.Send(conns[0], sameObject))
	accepted := boardAck(t, conns[0], sameObject.ID)
	if accepted.Type != "delta" || accepted.Seq != 11 || len(accepted.Elements) != 1 || accepted.Error != "" {
		t.Fatalf("expected delta ACK: %+v", accepted)
	}
	checkBoardError(t, websocket.JSON.Send(conns[1], stale))
	rejected := boardAck(t, conns[1], stale.ID)
	if rejected.Error != "element_conflict" || len(rejected.Conflicts) != 1 || rejected.Conflicts[0] != "shape0" || rejected.Seq != 11 {
		t.Fatalf("expected object conflict: %+v", rejected)
	}

	filePath := fmt.Sprintf("/api/v1/boards/%d/files/test_png", boardID)
	var raster bytes.Buffer
	checkBoardError(t, png.Encode(&raster, image.NewRGBA(image.Rect(0, 0, 4, 4))))
	boardRequest(t, server.URL, http.MethodPut, filePath, fixture.students[0], "https://different.invalid", "image/png", raster.Bytes(), 403)
	boardRequest(t, server.URL, http.MethodPut, filePath, fixture.students[1], server.URL, "image/png", raster.Bytes(), 404)
	boardRequest(t, server.URL, http.MethodPut, filePath, fixture.students[0], server.URL, "image/svg+xml", []byte("<svg/>"), 415)
	boardRequest(t, server.URL, http.MethodPut, filePath, fixture.students[0], server.URL, "image/png", []byte("<svg/>"), 400)
	boardRequest(t, server.URL, http.MethodPut, filePath, fixture.students[0], server.URL, "image/png", make([]byte, db.BoardFileLimit+1), 413)
	boardRequest(t, server.URL, http.MethodPut, filePath, fixture.students[0], server.URL, "image/png", raster.Bytes(), 204)
	imageOperation := boardTestOp(14, 11, "image1", 0)
	imageOperation.Payload.Elements = []json.RawMessage{json.RawMessage(`{"id":"image1","type":"image","fileId":"test_png","status":"saved","x":0,"y":0,"width":4,"height":4,"version":1,"versionNonce":1}`)}
	checkBoardError(t, websocket.JSON.Send(conns[1], imageOperation))
	if reply := boardAck(t, conns[1], imageOperation.ID); reply.Seq != 12 || reply.Error != "" {
		t.Fatalf("image sync: %+v", reply)
	}
	download := boardRequest(t, server.URL, http.MethodGet, filePath, fixture.teacher, "", "", nil, 200)
	if !bytes.Equal(download.Body.Bytes(), raster.Bytes()) || download.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("image was not served with its original bytes and safe headers")
	}
	boardRequest(t, server.URL, http.MethodGet, filePath, fixture.students[1], "", "", nil, 404)
	wsPath := fmt.Sprintf("/ws/boards/%d", boardID)
	boardRequest(t, server.URL, http.MethodGet, wsPath, fixture.students[1], server.URL, "", nil, 404)
	boardRequest(t, server.URL, http.MethodGet, wsPath, fixture.students[0], "https://different.invalid", "", nil, 403)
	boardRequest(t, server.URL, http.MethodGet, wsPath, 0, server.URL, "", nil, 401)

	revoked.Store(fixture.teacher, true)
	for i := 0; i < 10; i++ {
		if receiveBoard(t, conns[0]).Type == "access_revoked" {
			break
		}
		if i == 9 {
			t.Fatal("established socket retained a revoked session")
		}
	}
	revoked.Delete(fixture.teacher)
	for _, conn := range conns {
		_ = conn.Close()
	}
	server.Close()

	// A fresh HTTP handler and pool have no room or scene cache.
	restarted, err := db.New(ctx, dsn)
	checkBoardError(t, err)
	t.Cleanup(restarted.Close)
	server = start(restarted)
	conn := dialBoard(t, server.URL, boardID, fixture.students[0])
	initial := receiveBoard(t, conn)
	if initial.Seq != 12 || len(initial.Elements) != 11 {
		t.Fatalf("snapshot after server restart: %+v", initial)
	}
	checkBoardError(t, websocket.JSON.Send(conn, imageOperation))
	if duplicate := boardAck(t, conn, imageOperation.ID); duplicate.Type != "snapshot" || duplicate.Seq != 12 || duplicate.Error != "" {
		t.Fatalf("exact retry after restart: %+v", duplicate)
	}
	boardRequest(t, server.URL, http.MethodGet, filePath, fixture.students[0], "", "", nil, 200)

	_, err = store.SubmitAttempt(ctx, fixture.students[0], fixture.attempt.ID)
	checkBoardError(t, err)
	checkBoardError(t, websocket.JSON.Send(conn, boardTestOp(15, 12, "readonly", 0)))
	if reply := boardAck(t, conn, boardTestID(15)); reply.CanEdit || reply.Seq != 12 || reply.Error == "" {
		t.Fatalf("submitted board still editable: %+v", reply)
	}
	boardRequest(t, server.URL, http.MethodPut, filePath+"_new", fixture.students[0], server.URL, "image/png", raster.Bytes(), 409)

	second, err := store.StartOrResumeAttempt(ctx, fixture.students[0], fixture.attempt.AssignmentID)
	checkBoardError(t, err)
	next, err := store.GetSolveAttempt(ctx, fixture.students[0], second.ID)
	checkBoardError(t, err)
	if next.Tasks[0].BoardID == boardID || next.Tasks[0].BoardID == 0 {
		t.Fatal("second attempt did not get its own board")
	}
	fresh, err := store.GetBoard(ctx, fixture.students[0], next.Tasks[0].BoardID)
	checkBoardError(t, err)
	if fresh.Seq != 0 || len(fresh.Elements) != 0 {
		t.Fatal("second attempt inherited previous drawings")
	}
}

type boardFixture struct {
	teacher  int64
	students []int64
	attempt  *db.Attempt
	board    *boardRef
}

func newBoardFixture(t *testing.T, ctx context.Context, store *db.DB) boardFixture {
	t.Helper()
	stamp := time.Now().UnixNano()
	var users, tasks []int64
	var groupID, worksheetID int64
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, query := range []struct {
			sql string
			arg any
		}{
			{"DELETE FROM boards WHERE owner_id=ANY($1)", &users},
			{"DELETE FROM attempt_answers WHERE worksheet_id=$1", &worksheetID},
			{"DELETE FROM attempts WHERE worksheet_id=$1", &worksheetID},
			{"DELETE FROM assignments WHERE worksheet_id=$1", &worksheetID},
			{"DELETE FROM worksheet_items WHERE worksheet_id=$1", &worksheetID},
			{"DELETE FROM worksheets WHERE id=$1", &worksheetID},
			{"DELETE FROM group_members WHERE group_id=$1", &groupID},
			{"DELETE FROM groups WHERE id=$1", &groupID},
			{"DELETE FROM teacher_students WHERE teacher_id=ANY($1)", &users},
			{"DELETE FROM users WHERE id=ANY($1)", &users},
			{"DELETE FROM tasks WHERE id=ANY($1)", &tasks},
		} {
			if _, err := store.Pool.Exec(cleanup, query.sql, query.arg); err != nil {
				t.Errorf("clean fixture: %v", err)
			}
		}
	})
	createUser := func(role string, index int) int64 {
		user, err := store.CreateUser(ctx, db.NewUser{
			Email: fmt.Sprintf("board-%d-%d@example.invalid", stamp, index),
			Name:  "Board test user", Role: role, PasswordHash: "not-a-real-password",
		})
		checkBoardError(t, err)
		users = append(users, user.ID)
		return user.ID
	}
	teacher := createUser("teacher", 0)
	group, err := store.CreateGroup(ctx, db.NewGroup{TeacherID: teacher, Name: "Board test group"})
	checkBoardError(t, err)
	groupID = group.ID
	fixture := boardFixture{teacher: teacher}
	for i := 1; i <= 10; i++ {
		student := createUser("student", i)
		_, err := store.Pool.Exec(ctx, "INSERT INTO teacher_students(teacher_id,student_id) VALUES($1,$2)", teacher, student)
		checkBoardError(t, err)
		checkBoardError(t, store.AddGroupMember(ctx, group.ID, student))
		fixture.students = append(fixture.students, student)
	}
	work, err := store.CreateWorksheet(ctx, teacher, "Board test worksheet", "math", 2)
	checkBoardError(t, err)
	worksheetID = work.ID
	for i, input := range []struct {
		exam         string
		number, part int
	}{
		{"oge", 19, 0}, {"ege", 12, 1}, {"oge", 20, 0}, {"ege", 13, 2}, {"ege", 14, 0},
	} {
		metadata := map[string]any{}
		if input.part != 0 {
			metadata["exam_part"] = input.part
		}
		task, err := store.AddTask(ctx, &db.Task{
			SourceURL:  fmt.Sprintf("https://example.invalid/board-%d-%d", stamp, i),
			SourceSite: "example.invalid", Subject: "math", ExamType: input.exam,
			TaskNumber: input.number, TaskType: "short_answer", ContentText: "Test problem", Answer: "1", Metadata: metadata,
		})
		checkBoardError(t, err)
		tasks = append(tasks, task.TaskID)
		_, err = store.AddWorksheetItem(ctx, teacher, work.ID, db.WorksheetItem{TaskVersionID: task.VersionID, Position: i + 1, MaxScore: "1"})
		checkBoardError(t, err)
	}
	checkBoardError(t, store.PublishWorksheet(ctx, teacher, work.ID))
	distribution := fmt.Sprintf("a7b1aa5a-0000-4000-8000-%012x", stamp&0xffffffffffff)
	assigned, err := store.AssignWorksheetToGroup(ctx, teacher, work.ID, group.ID, distribution, nil)
	checkBoardError(t, err)
	if len(assigned) != 10 {
		t.Fatalf("group distribution: wanted 10 assignments; got %d", len(assigned))
	}
	seen := map[int64]bool{}
	for _, assignment := range assigned {
		attempt, err := store.StartOrResumeAttempt(ctx, assignment.StudentID, assignment.ID)
		checkBoardError(t, err)
		solve, err := store.GetSolveAttempt(ctx, assignment.StudentID, attempt.ID)
		checkBoardError(t, err)
		if len(solve.Tasks) != 5 {
			t.Fatalf("expected five tasks in solve, got %d", len(solve.Tasks))
		}
		a, b, c, d, e := solve.Tasks[0].BoardID, solve.Tasks[1].BoardID, solve.Tasks[2].BoardID, solve.Tasks[3].BoardID, solve.Tasks[4].BoardID
		if a <= 0 || a != b || c <= 0 || d <= 0 || a == c || a == d || c == d || e != 0 {
			t.Fatalf("board ownership by exam part: %v", solve.Tasks)
		}
		for _, id := range []int64{a, c, d} {
			if seen[id] {
				t.Fatalf("group students share board %d", id)
			}
			seen[id] = true
		}
		if assignment.StudentID == fixture.students[0] {
			fixture.attempt = attempt
			fixture.board = &boardRef{ID: a}
		}
	}
	return fixture
}

func checkBoardError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func dialBoard(t *testing.T, addr string, boardID, actor int64) *websocket.Conn {
	t.Helper()
	url := fmt.Sprintf("ws%s/ws/boards/%d", strings.TrimPrefix(addr, "http"), boardID)
	config, err := websocket.NewConfig(url, addr)
	checkBoardError(t, err)
	config.Header.Set("X-Board-Test-Actor", strconv.FormatInt(actor, 10))
	conn, err := websocket.DialConfig(config)
	checkBoardError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func receiveBoard(t *testing.T, conn *websocket.Conn) frame {
	t.Helper()
	checkBoardError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	var message frame
	checkBoardError(t, websocket.JSON.Receive(conn, &message))
	return message
}

func boardAck(t *testing.T, conn *websocket.Conn, op string) frame {
	t.Helper()
	for range 25 {
		message := receiveBoard(t, conn)
		if message.Ack == op {
			return message
		}
	}
	t.Fatalf("no ACK for %s", op)
	return frame{}
}

func boardTestID(id int) string {
	return fmt.Sprintf("9a24058a-0000-4000-8000-%012x", id)
}

func boardTestOp(id int, base int64, elementID string, x int) db.BoardOperation {
	element := fmt.Sprintf(`{"id":%q,"type":"rectangle","x":%d,"y":0,"width":10,"height":10,"version":1,"versionNonce":1}`, elementID, x)
	return db.BoardOperation{ID: boardTestID(id), BaseSeq: base, Payload: db.BoardPayload{Elements: []json.RawMessage{json.RawMessage(element)}}}
}

func boardRequest(t *testing.T, addr, method, path string, actor int64, origin, mime string, data []byte, status int) *httptest.ResponseRecorder {
	t.Helper()
	request, err := http.NewRequest(method, addr+path, bytes.NewReader(data))
	checkBoardError(t, err)
	request.Header.Set("X-Board-Test-Actor", strconv.FormatInt(actor, 10))
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if mime != "" {
		request.Header.Set("Content-Type", mime)
	}
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	checkBoardError(t, err)
	defer response.Body.Close()
	recorder := httptest.NewRecorder()
	recorder.HeaderMap = response.Header
	recorder.Code = response.StatusCode
	_, err = recorder.Body.ReadFrom(response.Body)
	checkBoardError(t, err)
	if response.StatusCode != status {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.StatusCode, status, recorder.Body)
	}
	return recorder
}
