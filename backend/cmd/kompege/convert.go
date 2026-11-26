package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

var demoYearPattern = regexp.MustCompile(`(?i)демоверсия\s*(20\d{2})`)

func sourceYear(comment string) int {
	match := demoYearPattern.FindStringSubmatch(comment)
	if len(match) < 2 {
		return 0
	}
	year, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return year
}

func prepareExamTask(source KompegeTask, number int) (*db.Task, error) {
	suffix := ""
	if number != source.Number {
		if source.Number != 19 || (number != 20 && number != 21) {
			return nil, fmt.Errorf("номер %d отсутствует у исходного задания", number)
		}
		for _, sub := range source.SubTasks {
			if sub.Number != number {
				continue
			}
			if strings.TrimSpace(sub.Text) == "" || strings.TrimSpace(sub.Key) == "" {
				return nil, fmt.Errorf("неполное подзадание №%d", number)
			}
			// Сохраняем общие правила игры из №19 и отделяем вопрос подзадания.
			source.Text = "<p>Контекст блока №19–21</p>" + source.Text +
				fmt.Sprintf("<p>Вопрос №%d</p>", number) + sub.Text
			source.Number, source.Key, source.SolveText, source.SubTasks = number, sub.Key, "", nil
			if len(sub.Table) != 0 && string(sub.Table) != "null" {
				source.Table = sub.Table
			}
			suffix = fmt.Sprintf("#%d", number)
			break
		}
		if suffix == "" {
			return nil, fmt.Errorf("номер %d отсутствует у исходного задания", number)
		}
	}
	task, err := prepareDBTask(source.ID, source)
	if err != nil {
		return nil, err
	}
	if reason := taskQualityProblem(source, task); reason != "" {
		return nil, fmt.Errorf("%s", reason)
	}
	task.SourceURL += suffix
	return task, nil
}

func prepareDBTask(sourceID int, kt KompegeTask) (*db.Task, error) {
	contentHTML, images, err := prepareHTML(kt.Text)
	if err != nil {
		return nil, fmt.Errorf("подготовка HTML: %w", err)
	}
	tables, err := extractTables(contentHTML)
	if err != nil {
		return nil, fmt.Errorf("извлечение таблиц: %w", err)
	}
	files, err := prepareFiles(kt.Files)
	if err != nil {
		return nil, err
	}

	metadata := map[string]any{
		"difficulty":   difficultyName(kt.Difficulty),
		"content_html": contentHTML,
	}
	if kt.Number >= 1 && kt.Number <= 27 {
		part := 1
		if kt.Number >= 24 {
			part = 2
		}
		metadata["exam_part"] = part
	}
	if len(tables) > 0 {
		metadata["tables"] = tables
	}
	if len(images) > 0 {
		metadata["images"] = images
	}
	if len(files) > 0 {
		metadata["files"] = files
	}
	if comment := strings.TrimSpace(kt.Comment); comment != "" {
		metadata["source_comment"] = comment
	}
	if uploader := strings.TrimSpace(kt.UserID); uploader != "" {
		metadata["source_uploader_id"] = uploader
	}

	if len(kt.Table) > 0 && string(kt.Table) != "null" && string(kt.Table) != "{}" {
		var originalTable any
		if err := json.Unmarshal(kt.Table, &originalTable); err == nil {
			metadata["source_table"] = originalTable
		}
	}
	if solutionHTML := cleanHTML(kt.SolveText); solutionHTML != "" {
		metadata["solution_html"] = solutionHTML
	}
	if len(kt.SubTasks) > 0 {
		subtasks, err := prepareSubtasks(kt.SubTasks)
		if err != nil {
			return nil, fmt.Errorf("подзадания: %w", err)
		}
		metadata["subtasks"] = subtasks
	}

	return &db.Task{
		SourceURL:   fmt.Sprintf("https://kompege.ru/api/v1/task/%d", sourceID),
		SourceSite:  "kompege.ru",
		Subject:     "informatics",
		ExamType:    "ege",
		TaskType:    "short_answer",
		TaskNumber:  kt.Number,
		ContentText: htmlToText(contentHTML),
		Answer:      strings.TrimSpace(kt.Key),
		Solution:    htmlToText(cleanHTML(kt.SolveText)),
		Year:        sourceYear(kt.Comment),
		Metadata:    metadata,
	}, nil
}
