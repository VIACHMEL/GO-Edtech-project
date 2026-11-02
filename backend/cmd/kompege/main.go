package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/VIACHMEL/GO-Edtech-project/backend/pkg/db"
	"net/http"
	"os"
	"strconv"
	"time"
)

type KompegeTask struct {
	Number     int               `json:"number"`
	Text       string            `json:"text"`
	Key        string            `json:"key"`
	Difficulty int               `json:"difficulty"`
	SolveText  string            `json:"solve_text"`
	SubTasks   []SubTask         `json:"subTask"`
	Files      []json.RawMessage `json:"files"`
}
type SubTask struct {
	Number     int    `json:"number"`
	Text       string `json:"text"`
	Key        string `json:"key"`
	Difficulty *int   `json:"difficulty"`
}
type taskHTTPError struct {
	Code int
}

func (e *taskHTTPError) Error() string {
	return fmt.Sprintf("HTTP %d", e.Code)
}
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
		return KompegeTask{}, fmt.Errorf("Запрос задачи %d %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return KompegeTask{}, &taskHTTPError{
			Code: resp.StatusCode,
		}
	}

	var kt KompegeTask

	err = json.NewDecoder(resp.Body).Decode(&kt)
	if err != nil {
		return KompegeTask{}, fmt.Errorf("декодирование задачи %d: %w", id, err)
	}
	return kt, nil
}
func main() {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	if len(os.Args) < 2 {
		fmt.Println("Укажи ID задания")
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
			fmt.Println("Некоректный ID:", arg)
			continue
		}
		task, err := getTask(client, id)
		if err != nil {
			fmt.Print("Ошибка:", err)
			continue
		}
		if task.Number < 1 || task.Number > 27 {
			fmt.Println("Пропущен номер:", task.Number)
			continue
		}
		dbTask, err := prepareDBTask(id, task)
		if err != nil {
			fmt.Println("Ошибка подготовки задания:", id, err)
			continue
		}

		savedID, err := store.AddTask(ctx, dbTask)
		if err != nil {
			fmt.Println("Ошибка сохранения:", err)
			continue
		}

		fmt.Println("Задание сохранено. ID в БД:", savedID)
		savedTask, err := store.GetTask(ctx, savedID)

		if err != nil {
			fmt.Println("Ошибка чтения из БД:", err)
			continue
		}

		fmt.Println("Прочитано из PostgreSQL:")
		fmt.Println("Файлы из БД:", savedTask.Metadata["files"])
		fmt.Println("Номер:", savedTask.TaskNumber)
		fmt.Println("Ответ:", savedTask.Answer)
		fmt.Println("Суб задания", savedTask.Metadata["subtasks"])
		fmt.Println("картинки", savedTask.Metadata["images"])

		fmt.Println("Таблицы:", savedTask.Metadata["tables"])
		fmt.Println("Подзадания:", savedTask.Metadata["subtasks"])
	}

}
