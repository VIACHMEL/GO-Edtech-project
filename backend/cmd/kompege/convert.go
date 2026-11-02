package main

import (
	"fmt"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/pkg/db"
)

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
	if len(tables) > 0 {
		metadata["tables"] = tables
	}

	if len(images) > 0 {
		metadata["images"] = images
	}

	if len(files) > 0 {
		metadata["files"] = files
	}

	if len(kt.SubTasks) > 0 {
		subtasks, err := prepareSubtasks(kt.SubTasks)
		if err != nil {
			return nil, fmt.Errorf("обработка подзаданий: %w", err)
		}

		metadata["subtasks"] = subtasks
	}

	return &db.Task{
		SourceURL:   fmt.Sprintf("https://kompege.ru/api/v1/task/%d", sourceID),
		SourceSite:  "kompege.ru",
		Subject:     "informatics",
		ExamType:    "ege",
		TaskNumber:  kt.Number,
		ContentText: htmlToText(contentHTML),
		Answer:      strings.TrimSpace(kt.Key),
		Solution:    htmlToText(cleanHTML(kt.SolveText)),
		Metadata:    metadata,
		TaskType:    "short_answer",
	}, nil
}
