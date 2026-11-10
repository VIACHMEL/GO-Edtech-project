package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

type KompegeTask struct {
	Number     int               `json:"number"`
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
	Number     int    `json:"number"`
	Text       string `json:"text"`
	Key        string `json:"key"`
	Difficulty *int   `json:"difficulty"`
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
	url := fmt.Sprintf("https://kompege.ru/api/v1/task/%d", id)
	resp, err := client.Get(url)
	if err != nil {
		return KompegeTask{}, fmt.Errorf("запрос задачи %d: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return KompegeTask{}, &taskHTTPError{Code: resp.StatusCode}
	}

	var task KompegeTask
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return KompegeTask{}, &taskDecodeError{Err: err}
	}
	return task, nil
}

func main() {
	client := &http.Client{Timeout: 10 * time.Second}
	if len(os.Args) < 2 {
		fmt.Println("Укажи ID или --range START END")
		return
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Println("Не задан DATABASE_URL")
		return
	}

	ctx := context.Background()
	store, err := db.New(ctx, dsn)
	if err != nil {
		fmt.Println("Ошибка подключения к БД:", err)
		return
	}
	defer store.Close()

	if os.Args[1] == "--range" {
		if len(os.Args) != 4 {
			fmt.Println("Использование: --range START END")
			return
		}
		start, err1 := strconv.Atoi(os.Args[2])
		end, err2 := strconv.Atoi(os.Args[3])
		if err1 != nil || err2 != nil {
			fmt.Println("START и END должны быть числами")
			return
		}
		if err := importRange(ctx, store, client, start, end); err != nil {
			fmt.Println("Ошибка импорта:", err)
		}
		return
	}

	for _, arg := range os.Args[1:] {
		id, err := strconv.Atoi(arg)
		if err != nil {
			fmt.Println("Некорректный ID:", arg)
			continue
		}
		task, err := getTask(client, id)
		if err != nil {
			fmt.Println("Ошибка получения:", err)
			continue
		}
		if task.Number < 1 || task.Number > 27 {
			fmt.Println("Не ЕГЭ №1–27:", task.Number)
			continue
		}
		dbTask, err := prepareDBTask(id, task)
		if err != nil {
			fmt.Println("Ошибка подготовки:", err)
			continue
		}
		if reason := taskQualityProblem(task, dbTask); reason != "" {
			fmt.Printf("ID %d: не сохранено — %s\n", id, reason)
			continue
		}
		savedID, err := store.AddTask(ctx, dbTask)
		if err != nil {
			fmt.Println("Ошибка сохранения:", err)
			continue
		}
		fmt.Printf("Сохранено: КЕГЭ %d -> БД %d, №%d, ответ %q\n", id, savedID, dbTask.TaskNumber, dbTask.Answer)
		fmt.Printf("HTML: %d байт, изображений: %d, таблиц: %d, файлов: %d\n",
			len(dbTask.Metadata["content_html"].(string)),
			metadataLength(dbTask.Metadata["images"]),
			metadataLength(dbTask.Metadata["tables"]),
			metadataLength(dbTask.Metadata["files"]),
		)
	}
}

func metadataLength(value any) int {
	switch v := value.(type) {
	case []string:
		return len(v)
	case []TableData:
		return len(v)
	case []map[string]string:
		return len(v)
	default:
		return 0
	}
}
