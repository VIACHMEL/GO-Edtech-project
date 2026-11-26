package main

import (
	"context"
	"crypto/sha256"
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
	if task.Hidden {
		return "задание скрыто источником"
	}
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

func selectBank(ctx context.Context, client *http.Client, root, filename string, variants bool) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var ids []int
	seenIDs := make(map[int]bool)
	for _, field := range strings.Fields(strings.TrimPrefix(string(data), "\ufeff")) {
		id, err := strconv.Atoi(field)
		if err != nil || id < 1 {
			return fmt.Errorf("неверный ID в списке кандидатов: %q", field)
		}
		if !seenIDs[id] {
			ids = append(ids, id)
			seenIDs[id] = true
		}
	}

	const target = 10
	var counts [28]int
	seenContent := make(map[[32]byte]bool)
	seenTasks := make(map[string]bool)
	accepted, rejected := 0, 0
	defer func() {
		for number := 1; number < len(counts); number++ {
			fmt.Printf("COVERAGE\t%d\t%d\t%d\n", number, counts[number], target-counts[number])
		}
		fmt.Printf("Принято %d, отклонено %d\n", accepted, rejected)
	}()
	accept := func(source KompegeTask, number int) error {
		id := source.ID
		if id <= 0 || number < 1 || number >= len(counts) {
			rejected++
			fmt.Printf("Отклонён ID %d: отсутствует или вне №1–27\n", id)
			return nil
		}
		ref := strconv.Itoa(id)
		if number != source.Number {
			ref += ":" + strconv.Itoa(number)
		}
		if counts[number] >= target || seenTasks[ref] {
			return nil
		}
		seenTasks[ref] = true
		task, err := prepareExamTask(source, number)
		if err != nil {
			rejected++
			fmt.Printf("Отклонён ID %s (№%d): %v\n", ref, number, err)
			return nil
		}
		materials, err := saveTaskMaterials(ctx, client, root, id, task)
		if err != nil {
			var httpErr *taskHTTPError
			if errors.As(err, &httpErr) && (httpErr.Code == 403 || httpErr.Code == 429) {
				return fmt.Errorf("отбор остановлен на материале ID %s: %w", ref, err)
			}
			rejected++
			fmt.Printf("Отклонён ID %s (№%d): материал %v\n", ref, number, err)
			return nil
		}
		// Одинаковые формулировки с разными входными данными — разные задачи.
		content := strings.ToLower(strings.Join(strings.Fields(task.ContentText), " "))
		hash := sha256.New()
		fmt.Fprintf(hash, "%d:%s", number, content)
		for _, m := range materials {
			fmt.Fprintf(hash, "\n%s:%s", m.key, m.checksum)
		}
		var fingerprint [32]byte
		hash.Sum(fingerprint[:0])
		if seenContent[fingerprint] {
			rejected++
			fmt.Printf("Отклонён ID %s (№%d): дубликат условия и материалов\n", ref, number)
			return nil
		}
		seenContent[fingerprint] = true
		counts[number]++
		accepted++
		fmt.Printf("SELECTED\t%d\t%s\t%d\n", number, ref, len(materials))
		return nil
	}
	for i, id := range ids {
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		var candidates []KompegeTask
		if variants {
			var variant struct {
				AuthRequired bool          `json:"authRequired"`
				RealMode     bool          `json:"realMode"`
				Tasks        []KompegeTask `json:"tasks"`
			}
			if err := getSourceJSON(client, fmt.Sprintf("variant/kim/%d", id), &variant); err != nil {
				var httpErr *taskHTTPError
				var malformed *taskDecodeError
				if (errors.As(err, &httpErr) && httpErr.Code == 404) || errors.As(err, &malformed) {
					fmt.Printf("Пропущен КИМ %d: %v\n", id, err)
					continue
				}
				return fmt.Errorf("отбор остановлен на КИМ %d: %w", id, err)
			}
			if variant.AuthRequired || variant.RealMode {
				fmt.Printf("Пропущен КИМ %d: закрытый вариант\n", id)
				continue
			}
			candidates = variant.Tasks
		} else {
			source, missing, err := fetchWithRetry(client, id)
			if err != nil || missing {
				var malformed *taskDecodeError
				if missing || errors.As(err, &malformed) {
					rejected++
					fmt.Printf("Отклонён ID %d: отсутствует или ответ API повреждён\n", id)
					continue
				}
				return fmt.Errorf("отбор остановлен на ID %d: %w", id, err)
			}
			candidates = []KompegeTask{source}
		}
		for _, candidate := range candidates {
			if err := accept(candidate, candidate.Number); err != nil {
				return err
			}
			if candidate.Number == 19 && !candidate.Hidden {
				for _, sub := range candidate.SubTasks {
					if sub.Number != 20 && sub.Number != 21 {
						continue
					}
					if err := accept(candidate, sub.Number); err != nil {
						return err
					}
				}
			}
			if accepted == target*(len(counts)-1) {
				return nil
			}
		}
	}
	return fmt.Errorf("кандидаты закончились до заполнения всех квот")
}

func importTask(ctx context.Context, store *db.DB, client *http.Client, root string, id int, task *db.Task) error {
	materials, err := saveTaskMaterials(ctx, client, root, id, task)
	if err != nil {
		return err
	}
	saved, err := store.AddTask(ctx, task)
	if err != nil {
		return err
	}
	if err := storeTaskMaterials(ctx, store, saved.VersionID, materials); err != nil {
		return fmt.Errorf("версия %d записана, повторите ID %d для сохранения артефактов: %w", saved.VersionID, id, err)
	}
	fmt.Printf("Сохранено: КЕГЭ %d -> БД %d, №%d, версия %d, материалов %d\n",
		id, saved.TaskID, task.TaskNumber, saved.VersionID, len(materials))
	return nil
}

func importRange(ctx context.Context, store *db.DB, client *http.Client, root string, start, end int) error {
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
				if err := importTask(ctx, store, client, root, id, prepared); err != nil {
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
