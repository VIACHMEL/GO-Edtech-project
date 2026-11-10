package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

func fetchWithRetry(client *http.Client, id int) (KompegeTask, bool, error) {
	for attempt := 1; attempt <= 3; attempt++ {
		task, err := getTask(client, id)
		if err == nil {
			return task, false, nil
		}

		var httpErr *taskHTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.Code {
			case 404:
				return KompegeTask{}, true, nil
			case 403, 429:
				return KompegeTask{}, false, err
			case 408, 500, 502, 503, 504:
				// Временная ошибка.
			default:
				return KompegeTask{}, false, err
			}
		}
		var decodeErr *taskDecodeError
		if errors.As(err, &decodeErr) {
			// 200 OK, но тело ответа не JSON нужной структуры.
			return KompegeTask{}, false, err
		}
		if attempt == 3 {
			return KompegeTask{}, false, fmt.Errorf("три попытки для ID %d: %w", id, err)
		}
		time.Sleep(time.Duration(attempt*10) * time.Second)
	}
	return KompegeTask{}, false, errors.New("неожиданное завершение")
}

func saveProgress(filename string, nextID int) error {
	temporary := filename + ".tmp"
	if err := os.WriteFile(temporary, []byte(strconv.Itoa(nextID)), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, filename)
}

func loadProblemIDs(filename string) (map[int]bool, error) {
	result := make(map[int]bool)
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		id, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("неверный ID в %s: %q", filename, line)
		}
		result[id] = true
	}
	return result, nil
}

func saveProblemID(filename string, id int) error {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := fmt.Fprintln(file, id); err != nil {
		return err
	}
	return file.Sync()
}

func isImageOnly(text string) bool {
	withoutImages := strings.ReplaceAll(text, "[изображение]", "")
	return strings.Trim(withoutImages, "| \n\r\t") == "" && strings.Contains(text, "[изображение]")
}

func taskQualityProblem(task KompegeTask, prepared *db.Task) string {
	if strings.TrimSpace(task.Key) == "" {
		return "отсутствует ответ в API"
	}
	if strings.TrimSpace(task.Text) == "" {
		return "пустое исходное условие"
	}
	if strings.TrimSpace(prepared.ContentText) == "" {
		return "условие исчезло после очистки"
	}
	if isImageOnly(prepared.ContentText) {
		return "условие только изображением"
	}
	for _, sub := range task.SubTasks {
		if strings.TrimSpace(sub.Key) == "" {
			return fmt.Sprintf("нет ответа подзадания %d", sub.Number)
		}
		if strings.TrimSpace(sub.Text) == "" {
			return fmt.Sprintf("пустое условие подзадания %d", sub.Number)
		}
	}
	return ""
}

func importRange(ctx context.Context, store *db.DB, client *http.Client, start, end int) error {
	if start < 1 || end < start {
		return fmt.Errorf("неверный диапазон %d-%d", start, end)
	}

	progressFile := fmt.Sprintf("kompege-progress-%d-%d.txt", start, end)
	problemFile := fmt.Sprintf("kompege-problem-%d-%d.txt", start, end)
	loggedProblems, err := loadProblemIDs(problemFile)
	if err != nil {
		return err
	}

	nextID := start
	if data, err := os.ReadFile(progressFile); err == nil {
		saved, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr != nil || saved < start || saved > end+1 {
			return fmt.Errorf("повреждён файл прогресса %s", progressFile)
		}
		nextID = saved
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	savedCount, missingCount, skippedCount, problemCount := 0, 0, 0, 0
	fmt.Printf("Диапазон %d-%d | продолжаем с %d\n", start, end, nextID)

	recordProblem := func(id int, reason string) error {
		if !loggedProblems[id] {
			if err := saveProblemID(problemFile, id); err != nil {
				return err
			}
			loggedProblems[id] = true
		}
		problemCount++
		fmt.Printf("ПРОБЛЕМНЫЙ ID %d: %s\n", id, reason)
		return nil
	}

	for id := nextID; id <= end; id++ {
		time.Sleep(2 * time.Second)
		task, missing, err := fetchWithRetry(client, id)

		var decodeErr *taskDecodeError
		if errors.As(err, &decodeErr) {
			if logErr := recordProblem(id, "повреждён JSON ответа API"); logErr != nil {
				return logErr
			}
		} else if err != nil {
			// Ограничение API / сбой сервера / сети останавливают обход.
			return fmt.Errorf("импорт остановлен на ID %d: %w", id, err)
		} else if missing || (task.Number == 0 && task.Text == "" && task.Key == "") {
			missingCount++
		} else if task.Number < 1 || task.Number > 27 {
			skippedCount++
		} else if strings.TrimSpace(task.Key) == "" || strings.TrimSpace(task.Text) == "" {
			reason := "пустое условие"
			if strings.TrimSpace(task.Key) == "" {
				reason = "нет ответа"
			}
			if err := recordProblem(id, reason); err != nil {
				return err
			}
		} else {
			prepared, err := prepareDBTask(id, task)
			if err != nil {
				// Проблема данных одного задания не останавливает весь импорт.
				if logErr := recordProblem(id, "подготовка: "+err.Error()); logErr != nil {
					return logErr
				}
			} else if reason := taskQualityProblem(task, prepared); reason != "" {
				if err := recordProblem(id, reason); err != nil {
					return err
				}
			} else {
				if _, err := store.AddTask(ctx, prepared); err != nil {
					// Ошибка БД — критическая, прогресс не продвигаем.
					return fmt.Errorf("сохранение ID %d: %w", id, err)
				}
				savedCount++
			}
		}

		// Позицию продвигаем только после сохранения или записи ID в журнал.
		if err := saveProgress(progressFile, id+1); err != nil {
			return fmt.Errorf("сохранение прогресса: %w", err)
		}
		if id%100 == 0 || id == end {
			fmt.Printf("ID %d/%d | сохранено %d | отсутствуют %d | вне ЕГЭ %d | проблемные %d\n",
				id, end, savedCount, missingCount, skippedCount, problemCount)
		}
	}
	fmt.Println("Обход диапазона завершён")
	return nil
}
