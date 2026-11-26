package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

type KompegeTask struct {
	ID         int               `json:"taskId"`
	Number     int               `json:"number"`
	Hidden     bool              `json:"hide"`
	Text       string            `json:"text"`
	Key        string            `json:"key"`
	Difficulty int               `json:"difficulty"`
	SolveText  string            `json:"solve_text"`
	SubTasks   []SubTask         `json:"subTask"`
	Files      []json.RawMessage `json:"files"`
	Comment    string            `json:"comment"`
	UserID     string            `json:"user_id"`
	Table      json.RawMessage   `json:"table"`
}

type SubTask struct {
	Number     int             `json:"number"`
	Text       string          `json:"text"`
	Key        string          `json:"key"`
	Difficulty *int            `json:"difficulty"`
	Table      json.RawMessage `json:"table"`
}

type taskHTTPError struct{ Code int }

func (e *taskHTTPError) Error() string { return fmt.Sprintf("HTTP %d", e.Code) }

type taskDecodeError struct{ Err error }

func (e *taskDecodeError) Error() string {
	return "неверный JSON задания: " + e.Err.Error()
}
func (e *taskDecodeError) Unwrap() error { return e.Err }

func difficultyName(level int) string {
	switch level {
	case 0:
		return "base"
	case 1:
		return "advanced"
	case 2:
		return "high"
	default:
		return "unknown"
	}
}

func getTask(client *http.Client, id int) (KompegeTask, error) {
	var task KompegeTask
	err := getSourceJSON(client, fmt.Sprintf("task/%d", id), &task)
	task.ID = id
	return task, err
}

func getSourceJSON(client *http.Client, endpoint string, value any) error {
	url := "https://kompege.ru/api/v1/" + endpoint
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &taskHTTPError{Code: resp.StatusCode}
	}

	const maxResponseBytes = 16 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxResponseBytes {
		return &taskDecodeError{Err: fmt.Errorf("ответ превышает %d байт", maxResponseBytes)}
	}
	if err := json.Unmarshal(body, value); err != nil {
		return &taskDecodeError{Err: err}
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !allowedMaterialURL(req.URL) {
				return fmt.Errorf("недопустимое перенаправление КЕГЭ")
			}
			return nil
		},
	}
	if len(args) == 0 {
		return fmt.Errorf("укажи ID, --range START END, --select CANDIDATES или --select-variants KIMS")
	}
	ctx := context.Background()
	root := os.Getenv("DATA_ROOT")
	if args[0] == "--select" || args[0] == "--select-variants" {
		if len(args) != 2 {
			return fmt.Errorf("использование: %s СПИСОК (ID или КИМ, по одному на строку)", args[0])
		}
		return selectBank(ctx, client, root, args[1], args[0] == "--select-variants")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("не задан DATABASE_URL")
	}

	store, err := db.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("ошибка подключения к БД: проверьте DATABASE_URL и доступность PostgreSQL")
	}
	defer store.Close()

	if args[0] == "--range" {
		if len(args) != 3 {
			return fmt.Errorf("использование: --range START END")
		}
		start, err1 := strconv.Atoi(args[1])
		end, err2 := strconv.Atoi(args[2])
		if err1 != nil || err2 != nil {
			return fmt.Errorf("START и END должны быть числами")
		}
		return importRange(ctx, store, client, root, start, end)
	}

	type taskRef struct{ id, part int }
	refs := make([]taskRef, 0, len(args))
	for _, arg := range args {
		rawID, rawPart, hasPart := strings.Cut(arg, ":")
		id, err := strconv.Atoi(rawID)
		if err != nil || id < 1 {
			return fmt.Errorf("некорректный ID: %q", arg)
		}
		part := 0
		if hasPart {
			part, err = strconv.Atoi(rawPart)
			if err != nil || (part != 20 && part != 21) {
				return fmt.Errorf("некорректная ссылка на подзадание: %q", arg)
			}
		}
		refs = append(refs, taskRef{id, part})
	}
	rejected := 0
	for i, ref := range refs {
		id := ref.id
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		task, missing, err := fetchWithRetry(client, id)
		if err != nil {
			return fmt.Errorf("импорт остановлен на ID %d: %w", id, err)
		}
		if missing || task.Number < 1 || task.Number > 27 {
			fmt.Printf("ID %d: отсутствует или вне ЕГЭ №1–27\n", id)
			rejected++
			continue
		}
		part := ref.part
		if part == 0 {
			part = task.Number
		}
		prepared, err := prepareExamTask(task, part)
		if err != nil {
			fmt.Printf("ID %d: ошибка подготовки: %v\n", id, err)
			rejected++
			continue
		}
		if err := importTask(ctx, store, client, root, id, prepared); err != nil {
			return fmt.Errorf("импорт остановлен на ID %d: %w", id, err)
		}
	}
	if rejected > 0 {
		return fmt.Errorf("не принято заданий: %d из %d", rejected, len(refs))
	}
	return nil
}
