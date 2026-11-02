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

	"github.com/VIACHMEL/GO-Edtech-project/backend/pkg/db"
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
				// Временная ошибка. Повторим запрос.

			default:
				return KompegeTask{}, false, err
			}
		}

		if attempt == 3 {
			return KompegeTask{}, false, fmt.Errorf(
				"три неудачные попытки для ID %d: %w", id, err,
			)
		}

		time.Sleep(time.Duration(attempt*10) * time.Second)
	}

	return KompegeTask{}, false, errors.New("неожиданное завершение")
}

func saveProgress(filename string, nextID int) error {
	temporary := filename + ".tmp"

	data := []byte(strconv.Itoa(nextID))

	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}

	return os.Rename(temporary, filename)
}

func importRange(
	ctx context.Context,
	store *db.DB,
	client *http.Client,
	start, end int,
) error {
	if start < 1 || end < start {
		return fmt.Errorf("неверный диапазон: %d-%d", start, end)
	}

	progressFile := fmt.Sprintf(
		"kompege-progress-%d-%d.txt", start, end,
	)

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

	savedCount := 0
	missingCount := 0
	skippedCount := 0

	for id := nextID; id <= end; id++ {
		time.Sleep(2 * time.Second)

		task, missing, err := fetchWithRetry(client, id)
		if err != nil {
			return fmt.Errorf("импорт остановлен на ID %d: %w", id, err)
		}

		if missing {
			missingCount++
		} else if task.Number < 1 || task.Number > 27 ||
			strings.TrimSpace(task.Text) == "" {
			skippedCount++
		} else {
			dbTask, err := prepareDBTask(id, task)
			if err != nil {
				return fmt.Errorf("подготовка ID %d: %w", id, err)
			}

			_, err = store.AddTask(ctx, dbTask)
			if err != nil {
				return fmt.Errorf("сохранение ID %d: %w", id, err)
			}

			savedCount++
		}

		if err := saveProgress(progressFile, id+1); err != nil {
			return fmt.Errorf("сохранение прогресса: %w", err)
		}

		if id%100 == 0 || id == end {
			fmt.Printf(
				"ID %d/%d | сохранено: %d | отсутствуют: %d | пропущено: %d\n",
				id, end, savedCount, missingCount, skippedCount,
			)
		}
	}

	fmt.Println("Обход диапазона завершён")
	return nil
}
